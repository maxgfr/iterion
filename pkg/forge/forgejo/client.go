// Package forgejo is the Forgejo/Gitea implementation of forge.Admin. Its
// REST shape mirrors GitHub (an `events` array + a nested `config`) but the
// API lives under /api/v1 and auth is the Gitea `token` scheme.
package forgejo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/SocialGouv/iterion/pkg/forge"
)

// AdminClient talks to one Forgejo/Gitea instance as one connection.
// BaseURL is the web base ("https://codeberg.org" or self-hosted). Auth is
// the Gitea token header; OAuth access tokens also authenticate this way on
// current Forgejo/Gitea.
type AdminClient struct {
	HTTP    *http.Client
	BaseURL string
	Token   string
}

func New(httpClient *http.Client, baseURL, token string) *AdminClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &AdminClient{HTTP: httpClient, BaseURL: strings.TrimRight(baseURL, "/"), Token: token}
}

func (c *AdminClient) Provider() forge.Provider { return forge.ProviderForgejo }

func (c *AdminClient) do(ctx context.Context, method, path string, body any, out any) (int, error) {
	var reqBody io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return 0, fmt.Errorf("forgejo: marshal body: %w", err)
		}
		reqBody = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+"/api/v1"+path, reqBody)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "token "+c.Token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if out != nil && resp.StatusCode/100 == 2 {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return resp.StatusCode, fmt.Errorf("forgejo: decode response: %w", err)
		}
	} else {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	}
	return resp.StatusCode, nil
}

func statusErr(op string, code int) error {
	switch code {
	case http.StatusUnauthorized:
		return forge.ErrUnauthorized
	case http.StatusForbidden:
		return forge.ErrForbidden
	case http.StatusNotFound:
		return forge.ErrHookNotFound
	default:
		return fmt.Errorf("forgejo: %s: HTTP %d", op, code)
	}
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

// ListRepos returns repos the token can admin. Gitea's /user/repos returns
// a `permissions.admin` flag the picker filters on.
func (c *AdminClient) ListRepos(ctx context.Context, q forge.RepoQuery) ([]forge.RepoSummary, error) {
	perPage := q.PerPage
	if perPage <= 0 || perPage > 50 {
		perPage = 50
	}
	page := q.Page
	if page <= 0 {
		page = 1
	}
	vals := url.Values{}
	vals.Set("limit", strconv.Itoa(perPage))
	vals.Set("page", strconv.Itoa(page))
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
	var hooks []forgejoHook
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
	var h forgejoHook
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
	var h forgejoHook
	code, err := c.do(ctx, http.MethodPatch, "/repos/"+repo+"/hooks/"+url.PathEscape(hookID), editBody(spec), &h)
	if err != nil {
		return forge.HookHandle{}, err
	}
	if code/100 != 2 {
		return forge.HookHandle{}, statusErr("update hook", code)
	}
	if h.ID == 0 { // some Gitea versions return 200 with empty body on edit
		hid, _ := strconv.ParseInt(hookID, 10, 64)
		h.ID = hid
		h.Config.URL = spec.URL
		h.Events = spec.Events
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
