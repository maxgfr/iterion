package tracker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ForgejoOptions configures the Forgejo (Gitea-compatible) adapter.
type ForgejoOptions struct {
	Host  string // base URL, e.g. "https://codeberg.org"
	Repo  string // "owner/repo"
	Token string // FORGEJO_TOKEN — sent as Authorization header

	IncludeLabels []string
	ExcludeLabels []string
	StateMapping  map[string]LabelSelector
	ClaimedLabel  string

	// HTTPClient overrides the default net/http client. Useful for
	// tests; production leaves it nil.
	HTTPClient *http.Client
}

// ForgejoAdapter implements Tracker against the Forgejo/Gitea API. It
// uses direct REST calls because there is no widely-installed CLI
// equivalent of `gh` for Forgejo.
type ForgejoAdapter struct {
	opts ForgejoOptions
	hc   *http.Client
}

// NewForgejo returns a configured adapter.
func NewForgejo(opts ForgejoOptions) (*ForgejoAdapter, error) {
	if opts.Host == "" {
		return nil, errors.New("forgejo tracker: host is required")
	}
	if opts.Repo == "" {
		return nil, errors.New("forgejo tracker: repo is required (owner/repo)")
	}
	if opts.ClaimedLabel == "" {
		opts.ClaimedLabel = "iterion-claimed"
	}
	hc := opts.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	opts.Host = strings.TrimRight(opts.Host, "/")
	return &ForgejoAdapter{opts: opts, hc: hc}, nil
}

// Name implements Tracker.
func (a *ForgejoAdapter) Name() string { return "forgejo" }

// ListCandidates returns open issues matching the include/exclude
// label filters (filter executed locally — Forgejo doesn't have
// negative-label search in the same syntax as GitHub).
func (a *ForgejoAdapter) ListCandidates(ctx context.Context) ([]Issue, error) {
	q := url.Values{}
	q.Set("state", "open")
	q.Set("type", "issues")
	q.Set("limit", "50")
	if len(a.opts.IncludeLabels) > 0 {
		q.Set("labels", strings.Join(a.opts.IncludeLabels, ","))
	}
	endpoint := fmt.Sprintf("/repos/%s/issues?%s", a.opts.Repo, q.Encode())

	var raw []forgejoIssue
	if err := a.do(ctx, http.MethodGet, endpoint, nil, &raw); err != nil {
		return nil, err
	}
	out := make([]Issue, 0, len(raw))
	for _, r := range raw {
		labels := labelNames(r.Labels)
		if anyOfString(labels, a.opts.ExcludeLabels) {
			continue
		}
		if containsString(labels, a.opts.ClaimedLabel) {
			continue
		}
		iss := a.toIssue(r)
		if iss.WorkflowState == "" {
			continue
		}
		out = append(out, iss)
	}
	return out, nil
}

// RefreshStates fetches each issue and re-derives its state from
// current labels.
func (a *ForgejoAdapter) RefreshStates(ctx context.Context, ids []string) (map[string]string, error) {
	out := make(map[string]string, len(ids))
	for _, id := range ids {
		num, ok := parseForgejoID(a.opts.Host, a.opts.Repo, id)
		if !ok {
			continue
		}
		var r forgejoIssue
		if err := a.do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/issues/%d", a.opts.Repo, num), nil, &r); err != nil {
			continue
		}
		iss := a.toIssue(r)
		if iss.WorkflowState != "" {
			out[id] = iss.WorkflowState
		}
	}
	return out, nil
}

// UpdateState replaces an issue's labels with the include set from the
// matching state mapping. Returns ErrTransitionRejected if newState
// has no mapping.
func (a *ForgejoAdapter) UpdateState(ctx context.Context, id, newState string) error {
	sel, ok := a.opts.StateMapping[newState]
	if !ok {
		return fmt.Errorf("%w: no label mapping for state %q", ErrTransitionRejected, newState)
	}
	num, ok := parseForgejoID(a.opts.Host, a.opts.Repo, id)
	if !ok {
		return ErrNotFound
	}
	// Read current labels, apply the diff: drop excludes from sel,
	// add includes from sel. Keeps unrelated labels untouched.
	var current forgejoIssue
	if err := a.do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/issues/%d", a.opts.Repo, num), nil, &current); err != nil {
		return err
	}
	have := labelNames(current.Labels)
	for _, l := range sel.LabelsExclude {
		have = filterOutString(have, l)
	}
	for _, l := range sel.LabelsInclude {
		if !containsString(have, l) {
			have = append(have, l)
		}
	}
	return a.replaceLabels(ctx, num, have)
}

// Comment adds a comment to the issue.
func (a *ForgejoAdapter) Comment(ctx context.Context, id, body string) error {
	num, ok := parseForgejoID(a.opts.Host, a.opts.Repo, id)
	if !ok {
		return ErrNotFound
	}
	in := map[string]string{"body": body}
	return a.do(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/issues/%d/comments", a.opts.Repo, num), in, nil)
}

// Claim adds ClaimedLabel.
func (a *ForgejoAdapter) Claim(ctx context.Context, id, marker string) error {
	num, ok := parseForgejoID(a.opts.Host, a.opts.Repo, id)
	if !ok {
		return ErrNotFound
	}
	var current forgejoIssue
	if err := a.do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/issues/%d", a.opts.Repo, num), nil, &current); err != nil {
		return err
	}
	have := labelNames(current.Labels)
	if !containsString(have, a.opts.ClaimedLabel) {
		have = append(have, a.opts.ClaimedLabel)
	}
	return a.replaceLabels(ctx, num, have)
}

// Release removes ClaimedLabel.
func (a *ForgejoAdapter) Release(ctx context.Context, id, marker string) error {
	num, ok := parseForgejoID(a.opts.Host, a.opts.Repo, id)
	if !ok {
		return ErrNotFound
	}
	var current forgejoIssue
	if err := a.do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/issues/%d", a.opts.Repo, num), nil, &current); err != nil {
		return err
	}
	have := filterOutString(labelNames(current.Labels), a.opts.ClaimedLabel)
	return a.replaceLabels(ctx, num, have)
}

// ---------------------------------------------------------------------------
// internals
// ---------------------------------------------------------------------------

type forgejoIssue struct {
	Number    int            `json:"number"`
	Title     string         `json:"title"`
	Body      string         `json:"body"`
	State     string         `json:"state"`
	Labels    []forgejoLabel `json:"labels"`
	Assignees []forgejoUser  `json:"assignees"`
	User      forgejoUser    `json:"user"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	URL       string         `json:"html_url"`
}

type forgejoLabel struct {
	Name string `json:"name"`
}

type forgejoUser struct {
	Login string `json:"login"`
}

func (a *ForgejoAdapter) toIssue(r forgejoIssue) Issue {
	labels := filterOutString(labelNames(r.Labels), a.opts.ClaimedLabel)
	id := fmt.Sprintf("forgejo:%s/%s#%d", trimHost(a.opts.Host), a.opts.Repo, r.Number)
	assignee := ""
	if len(r.Assignees) > 0 {
		assignee = r.Assignees[0].Login
	}
	return Issue{
		ID:            id,
		Identifier:    fmt.Sprintf("%s#%d", a.opts.Repo, r.Number),
		Title:         r.Title,
		Body:          r.Body,
		WorkflowState: a.resolveState(labels),
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     r.UpdatedAt,
		Labels:        labels,
		Assignee:      assignee,
		Metadata: map[string]string{
			"url":           r.URL,
			"forgejo_state": r.State,
			"author":        r.User.Login,
		},
	}
}

func (a *ForgejoAdapter) resolveState(labels []string) string {
	return resolveStateByLabels(labels, a.opts.StateMapping)
}

func (a *ForgejoAdapter) replaceLabels(ctx context.Context, num int, labels []string) error {
	in := struct {
		Labels []string `json:"labels"`
	}{Labels: labels}
	return a.do(ctx, http.MethodPut, fmt.Sprintf("/repos/%s/issues/%d/labels", a.opts.Repo, num), in, nil)
}

// do performs an authenticated request against the Forgejo API. The
// response body is decoded into out (when non-nil). 404 maps to
// ErrNotFound; other non-2xx statuses return a wrapped error with the
// body excerpt for diagnostics.
func (a *ForgejoAdapter) do(ctx context.Context, method, path string, in any, out any) error {
	var body io.Reader
	if in != nil {
		data, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("forgejo: marshal: %w", err)
		}
		body = bytes.NewReader(data)
	}
	endpoint := a.opts.Host + "/api/v1" + path
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return fmt.Errorf("forgejo: build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if a.opts.Token != "" {
		req.Header.Set("Authorization", "token "+a.opts.Token)
	}
	resp, err := a.hc.Do(req)
	if err != nil {
		return fmt.Errorf("forgejo: do: %w", err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		if out == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			return nil
		}
		return json.NewDecoder(resp.Body).Decode(out)
	default:
		buf, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("forgejo: %s %s: status %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(buf)))
	}
}

func labelNames(in []forgejoLabel) []string {
	out := make([]string, 0, len(in))
	for _, l := range in {
		out = append(out, l.Name)
	}
	return out
}

func trimHost(h string) string {
	h = strings.TrimPrefix(h, "https://")
	h = strings.TrimPrefix(h, "http://")
	return strings.TrimRight(h, "/")
}

// parseForgejoID expects "forgejo:<host>/<owner>/<repo>#<num>".
func parseForgejoID(host, repo, id string) (int, bool) {
	prefix := "forgejo:" + trimHost(host) + "/" + repo + "#"
	if !strings.HasPrefix(id, prefix) {
		return 0, false
	}
	var n int
	if _, err := fmt.Sscanf(strings.TrimPrefix(id, prefix), "%d", &n); err != nil {
		return 0, false
	}
	return n, true
}
