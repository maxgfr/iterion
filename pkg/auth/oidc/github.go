package oidc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// GitHubConnector implements Connector against GitHub's OAuth2
// (NOT OIDC) endpoints. We retrieve the user profile from the v3
// API and resolve the verified primary email separately.
type GitHubConnector struct {
	clientID     string
	clientSecret string
	display      string
	httpClient   *http.Client
}

func NewGitHubConnector(clientID, clientSecret, display string) *GitHubConnector {
	if display == "" {
		display = "GitHub"
	}
	return &GitHubConnector{
		clientID:     clientID,
		clientSecret: clientSecret,
		display:      display,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (g *GitHubConnector) Name() string       { return "github" }
func (g *GitHubConnector) Display() string    { return g.display }
func (g *GitHubConnector) SupportsPKCE() bool { return true }

const (
	githubAuthorizeURL = "https://github.com/login/oauth/authorize"
	githubTokenURL     = "https://github.com/login/oauth/access_token"
	githubUserURL      = "https://api.github.com/user"
	githubEmailsURL    = "https://api.github.com/user/emails"
	githubOrgsURL      = "https://api.github.com/user/orgs?per_page=100"
	githubTeamsURL     = "https://api.github.com/user/teams?per_page=100"
	// read:org lets iterion read the user's org + team memberships so the
	// per-org GitHub team-gating can decide access. It returns private
	// memberships too once the user authorizes (an org whose OAuth-app-access
	// policy blocks third-party apps will simply not appear).
	githubScopes = "read:user user:email read:org"
	// githubMaxGroups bounds how many org/team keys we collect from a single
	// login — a user in a pathological number of orgs/teams cannot blow up
	// memory. Truncation is logged-by-omission, never a login failure.
	githubMaxGroups = 1000
	// githubMaxPages caps Link-header pagination follow to avoid an unbounded
	// loop on a misbehaving upstream (100/page × 50 = 5000 rows ceiling).
	githubMaxPages = 50
)

func (g *GitHubConnector) AuthorizeURL(_ context.Context, redirectURI, state, codeVerifier string) (string, error) {
	if g.clientID == "" {
		return "", fmt.Errorf("oidc/github: missing client id")
	}
	q := url.Values{}
	q.Set("client_id", g.clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("scope", githubScopes)
	q.Set("state", state)
	q.Set("allow_signup", "true")
	if codeVerifier != "" {
		q.Set("code_challenge", deriveS256(codeVerifier))
		q.Set("code_challenge_method", "S256")
	}
	return githubAuthorizeURL + "?" + q.Encode(), nil
}

func (g *GitHubConnector) ExchangeCode(ctx context.Context, code, redirectURI, codeVerifier string) (ExternalUser, error) {
	form := url.Values{}
	form.Set("client_id", g.clientID)
	form.Set("client_secret", g.clientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	if codeVerifier != "" {
		form.Set("code_verifier", codeVerifier)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, githubTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return ExternalUser{}, fmt.Errorf("oidc/github: build token req: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return ExternalUser{}, fmt.Errorf("oidc/github: exchange: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return ExternalUser{}, fmt.Errorf("oidc/github: token endpoint %d", resp.StatusCode)
	}
	var tok struct {
		AccessToken string `json:"access_token"`
		Scope       string `json:"scope"`
		TokenType   string `json:"token_type"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return ExternalUser{}, fmt.Errorf("oidc/github: decode token: %w", err)
	}
	if tok.Error != "" {
		return ExternalUser{}, fmt.Errorf("oidc/github: %s: %s", tok.Error, tok.ErrorDesc)
	}
	if tok.AccessToken == "" {
		return ExternalUser{}, fmt.Errorf("oidc/github: empty access token")
	}

	user, err := g.fetchUser(ctx, tok.AccessToken)
	if err != nil {
		return ExternalUser{}, err
	}
	// Always route the email through /user/emails (which checks
	// e.Verified). /user.email is the publicly visible profile email
	// — a display setting the user can write to without verification,
	// so trusting it directly would let an attacker who sets their
	// public email to victim@example.com claim that iterion account.
	// Mirror the unverified-email gate that google.go and generic.go
	// enforce via ErrEmailNotVerified.
	email, err := g.fetchPrimaryEmail(ctx, tok.AccessToken)
	if err != nil {
		return ExternalUser{}, err
	}
	if email == "" {
		return ExternalUser{}, ErrEmailMissing
	}
	user.Email = email
	groups, err := g.fetchOrgsAndTeams(ctx, tok.AccessToken)
	if err != nil {
		return ExternalUser{}, err
	}
	user.Groups = groups
	return user, nil
}

// fetchOrgsAndTeams returns the user's GitHub org + team membership as
// lowercased keys: "<org>/*" per org and "<org>/<team-slug>" per team — the
// shape the per-org grant allow-list matches against. Deduped and capped.
func (g *GitHubConnector) fetchOrgsAndTeams(ctx context.Context, accessToken string) ([]string, error) {
	seen := make(map[string]struct{})
	out := make([]string, 0, 16)
	add := func(k string) {
		k = strings.ToLower(k)
		if _, dup := seen[k]; dup || len(out) >= githubMaxGroups {
			return
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	if err := g.pageGitHub(ctx, githubOrgsURL, accessToken, func(data []byte) error {
		var orgs []struct {
			Login string `json:"login"`
		}
		if err := json.Unmarshal(data, &orgs); err != nil {
			return fmt.Errorf("oidc/github: decode orgs: %w", err)
		}
		for _, o := range orgs {
			if o.Login != "" {
				add(o.Login + "/*")
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if err := g.pageGitHub(ctx, githubTeamsURL, accessToken, func(data []byte) error {
		var teams []struct {
			Slug         string `json:"slug"`
			Organization struct {
				Login string `json:"login"`
			} `json:"organization"`
		}
		if err := json.Unmarshal(data, &teams); err != nil {
			return fmt.Errorf("oidc/github: decode teams: %w", err)
		}
		for _, t := range teams {
			if t.Organization.Login != "" && t.Slug != "" {
				add(t.Organization.Login + "/" + t.Slug)
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return out, nil
}

// pageGitHub GETs nextURL and follows the Link-header rel="next" cursor,
// handing each page's raw JSON body to handle. Bounded by githubMaxPages.
func (g *GitHubConnector) pageGitHub(ctx context.Context, nextURL, accessToken string, handle func([]byte) error) error {
	for page := 0; nextURL != "" && page < githubMaxPages; page++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, nextURL, nil)
		if err != nil {
			return fmt.Errorf("oidc/github: build paged req: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		resp, err := g.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("oidc/github: paged fetch: %w", err)
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxOIDCBodyBytes))
		link := resp.Header.Get("Link")
		resp.Body.Close()
		if resp.StatusCode/100 != 2 {
			return fmt.Errorf("oidc/github: paged endpoint %d", resp.StatusCode)
		}
		if readErr != nil {
			return fmt.Errorf("oidc/github: read paged body: %w", readErr)
		}
		if err := handle(body); err != nil {
			return err
		}
		nextURL = nextGitHubLink(link)
	}
	return nil
}

// nextGitHubLink extracts the rel="next" URL from a GitHub Link header.
func nextGitHubLink(header string) string {
	for _, part := range strings.Split(header, ",") {
		seg := strings.SplitN(part, ";", 2)
		if len(seg) != 2 || !strings.Contains(seg[1], `rel="next"`) {
			continue
		}
		u := strings.TrimSpace(seg[0])
		u = strings.TrimPrefix(u, "<")
		u = strings.TrimSuffix(u, ">")
		return u
	}
	return ""
}

func (g *GitHubConnector) fetchUser(ctx context.Context, accessToken string) (ExternalUser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubUserURL, nil)
	if err != nil {
		return ExternalUser{}, fmt.Errorf("oidc/github: build user req: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return ExternalUser{}, fmt.Errorf("oidc/github: user: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return ExternalUser{}, fmt.Errorf("oidc/github: user endpoint %d", resp.StatusCode)
	}
	var u struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return ExternalUser{}, fmt.Errorf("oidc/github: decode user: %w", err)
	}
	subject := fmt.Sprintf("%d", u.ID)
	name := u.Name
	if name == "" {
		name = u.Login
	}
	return ExternalUser{
		Provider: g.Name(),
		Subject:  subject,
		Email:    u.Email,
		Name:     name,
	}, nil
}

func (g *GitHubConnector) fetchPrimaryEmail(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubEmailsURL, nil)
	if err != nil {
		return "", fmt.Errorf("oidc/github: build emails req: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("oidc/github: emails: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("oidc/github: emails endpoint %d", resp.StatusCode)
	}
	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&emails); err != nil {
		return "", fmt.Errorf("oidc/github: decode emails: %w", err)
	}
	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email, nil
		}
	}
	for _, e := range emails {
		if e.Verified {
			return e.Email, nil
		}
	}
	return "", ErrEmailMissing
}
