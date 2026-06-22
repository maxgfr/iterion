// Package github is the GitHub implementation of forge.Admin: the
// outbound write-side client the orchestrator uses to list repos and
// create/update/delete the iterion webhook on them. Shared by the OAuth-App
// path (user token) and, later, the GitHub-App path (installation token) —
// they differ only in how the token is obtained, not in these REST calls.
package github

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/SocialGouv/iterion/pkg/forge"
)

// AdminClient talks to one GitHub instance (github.com or GHE) as one
// connection. Auth is a Bearer token (an OAuth user token, a PAT, or a
// GitHub-App installation token — GitHub accepts all three the same way).
type AdminClient struct {
	HTTP    *http.Client
	APIBase string // e.g. "https://api.github.com" or "https://ghe.example.com/api/v3"
	Token   string
}

// New builds an AdminClient. baseURL is the forge's WEB base
// ("https://github.com" or a GHE host); it is mapped to the matching REST
// API base. A nil httpClient falls back to http.DefaultClient.
func New(httpClient *http.Client, baseURL, token string) *AdminClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &AdminClient{HTTP: httpClient, APIBase: APIBaseFor(baseURL), Token: token}
}

// APIBaseFor maps a GitHub WEB base URL to its REST API base. github.com →
// api.github.com; a GitHub Enterprise host → <host>/api/v3.
func APIBaseFor(webBase string) string {
	b := strings.TrimRight(strings.TrimSpace(webBase), "/")
	switch b {
	case "", "https://github.com", "http://github.com":
		return "https://api.github.com"
	default:
		return b + "/api/v3"
	}
}

func (c *AdminClient) Provider() forge.Provider { return forge.ProviderGitHub }

func (c *AdminClient) do(ctx context.Context, method, path string, body any, out any) (int, error) {
	return forge.DoJSON(ctx, c.HTTP, method, c.APIBase+path, "github", func(req *http.Request) {
		req.Header.Set("Authorization", "Bearer "+c.Token)
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	}, body, out)
}

func statusErr(op string, code int) error {
	return forge.StatusErr("github", op, code)
}

func (c *AdminClient) WhoAmI(ctx context.Context) (forge.Identity, error) {
	var u struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
		Email string `json:"email"`
	}
	code, err := c.do(ctx, http.MethodGet, "/user", nil, &u)
	if err != nil {
		return forge.Identity{}, err
	}
	if code != http.StatusOK {
		return forge.Identity{}, statusErr("GET /user", code)
	}
	return forge.Identity{Login: u.Login, ID: strconv.FormatInt(u.ID, 10), Email: u.Email, Kind: "user", Namespace: u.Login}, nil
}

// OrgMembershipRole reports the caller's role ("admin" | "member") in org and
// whether the membership is active, via GET /user/memberships/orgs/{org}.
// A 404/403 (not a member, or no visibility) returns ("", false, nil) — the
// caller treats that as "no proof of control", not an error. Used to verify an
// iterion team controls (admins) a GitHub org before its teams may be
// allow-listed for SSO.
func (c *AdminClient) OrgMembershipRole(ctx context.Context, org string) (role string, active bool, err error) {
	var m struct {
		State string `json:"state"`
		Role  string `json:"role"`
	}
	code, err := c.do(ctx, http.MethodGet, "/user/memberships/orgs/"+url.PathEscape(org), nil, &m)
	if err != nil {
		return "", false, err
	}
	if code == http.StatusNotFound || code == http.StatusForbidden {
		return "", false, nil
	}
	if code != http.StatusOK {
		return "", false, statusErr("GET /user/memberships/orgs", code)
	}
	return m.Role, m.State == "active", nil
}

// ListRepos returns repos the token can admin (Permissions.Admin) — the
// floor for managing repo webhooks.
func (c *AdminClient) ListRepos(ctx context.Context, q forge.RepoQuery) ([]forge.RepoSummary, error) {
	perPage := q.PerPage
	if perPage <= 0 || perPage > 100 {
		perPage = 50
	}
	page := q.Page
	if page <= 0 {
		page = 1
	}
	vals := url.Values{}
	vals.Set("affiliation", "owner,collaborator,organization_member")
	vals.Set("per_page", strconv.Itoa(perPage))
	vals.Set("page", strconv.Itoa(page))
	vals.Set("sort", "pushed")
	var repos []struct {
		FullName      string `json:"full_name"`
		Description   string `json:"description"`
		Private       bool   `json:"private"`
		DefaultBranch string `json:"default_branch"`
		HTMLURL       string `json:"html_url"`
		Permissions   struct {
			Admin bool `json:"admin"`
		} `json:"permissions"`
	}
	code, err := c.do(ctx, http.MethodGet, "/user/repos?"+vals.Encode(), nil, &repos)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, statusErr("GET /user/repos", code)
	}
	// GitHub's repo search (q.Search) needs a separate endpoint; for now we
	// filter client-side so the picker's typeahead still narrows.
	needle := strings.ToLower(strings.TrimSpace(q.Search))
	out := make([]forge.RepoSummary, 0, len(repos))
	for _, r := range repos {
		if needle != "" && !strings.Contains(strings.ToLower(r.FullName), needle) {
			continue
		}
		out = append(out, forge.RepoSummary{
			FullName:      r.FullName,
			Description:   r.Description,
			Private:       r.Private,
			DefaultBranch: r.DefaultBranch,
			WebURL:        r.HTMLURL,
			CanAdmin:      r.Permissions.Admin,
		})
	}
	return out, nil
}

func (c *AdminClient) GetHook(ctx context.Context, repo, deliveryURL string) (*forge.HookHandle, error) {
	var hooks []githubHook
	code, err := c.do(ctx, http.MethodGet, "/repos/"+repo+"/hooks", nil, &hooks)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, statusErr("GET hooks", code)
	}
	for _, h := range hooks {
		if h.Config.URL == deliveryURL {
			hh := h.toHandle()
			return &hh, nil
		}
	}
	return nil, nil
}

func (c *AdminClient) CreateHook(ctx context.Context, repo string, spec forge.HookSpec) (forge.HookHandle, error) {
	var h githubHook
	code, err := c.do(ctx, http.MethodPost, "/repos/"+repo+"/hooks", hookBody(spec), &h)
	if err != nil {
		return forge.HookHandle{}, err
	}
	if code/100 != 2 {
		return forge.HookHandle{}, statusErr("create hook", code)
	}
	return h.toHandle(), nil
}

func (c *AdminClient) UpdateHook(ctx context.Context, repo, hookID string, spec forge.HookSpec) (forge.HookHandle, error) {
	var h githubHook
	code, err := c.do(ctx, http.MethodPatch, "/repos/"+repo+"/hooks/"+url.PathEscape(hookID), hookBody(spec), &h)
	if err != nil {
		return forge.HookHandle{}, err
	}
	if code/100 != 2 {
		return forge.HookHandle{}, statusErr("update hook", code)
	}
	return h.toHandle(), nil
}

func (c *AdminClient) DeleteHook(ctx context.Context, repo, hookID string) error {
	code, err := c.do(ctx, http.MethodDelete, "/repos/"+repo+"/hooks/"+url.PathEscape(hookID), nil, nil)
	if err != nil {
		return err
	}
	if code/100 != 2 {
		return statusErr("delete hook", code)
	}
	return nil
}
