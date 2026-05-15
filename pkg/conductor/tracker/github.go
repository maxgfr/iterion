package tracker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// GitHubOptions configures the GitHub Issues adapter. The Token field
// is optional — when empty, the adapter relies on `gh auth status`
// having a valid login already.
type GitHubOptions struct {
	// Repo is "owner/repo".
	Repo string

	// Token, when non-empty, is exported as GH_TOKEN to the gh
	// subprocess so the adapter works in non-interactive contexts
	// (CI). Empty means rely on the existing gh login.
	Token string

	// IncludeLabels, ExcludeLabels narrow the candidate pool. All
	// IncludeLabels must be present; any ExcludeLabel disqualifies.
	IncludeLabels []string
	ExcludeLabels []string

	// StateMapping maps a workflow state name to a label predicate.
	// The first entry that matches in iteration order determines
	// the issue's WorkflowState. Map iteration order is unspecified
	// in Go, so callers should treat ordering as best-effort and
	// design label predicates so at most one matches per issue.
	StateMapping map[string]LabelSelector

	// ClaimedLabel is added by Claim and removed by Release. Issues
	// carrying this label are filtered out of ListCandidates.
	// Defaults to "iterion-claimed".
	ClaimedLabel string

	// Command, when non-nil, overrides the gh subprocess factory.
	// Used by tests to inject fake responses. Production leaves it
	// nil so the adapter shells out to the real `gh`.
	Command func(ctx context.Context, args []string, env []string) ([]byte, error)
}

// LabelSelector restricts a state mapping by label allowlist / blocklist.
type LabelSelector struct {
	LabelsInclude []string
	LabelsExclude []string
}

// GitHubAdapter implements Tracker over the GitHub Issues API by
// shelling out to the `gh` CLI. Auth, OAuth, rate-limit handling and
// pagination come for free from gh; iterion only deals with JSON.
type GitHubAdapter struct {
	opts GitHubOptions
}

// NewGitHub returns a configured adapter. Returns an error if the
// minimum config (repo) is missing.
func NewGitHub(opts GitHubOptions) (*GitHubAdapter, error) {
	if opts.Repo == "" {
		return nil, errors.New("github tracker: repo is required (owner/repo)")
	}
	if opts.ClaimedLabel == "" {
		opts.ClaimedLabel = "iterion-claimed"
	}
	if opts.Command == nil {
		opts.Command = runGH
	}
	return &GitHubAdapter{opts: opts}, nil
}

// Name implements Tracker.
func (a *GitHubAdapter) Name() string { return "github" }

// ListCandidates returns open issues matching include/exclude labels
// and not carrying ClaimedLabel.
func (a *GitHubAdapter) ListCandidates(ctx context.Context) ([]Issue, error) {
	search := buildSearch(a.opts)
	args := []string{
		"issue", "list",
		"--repo", a.opts.Repo,
		"--state", "open",
		"--limit", "100",
		"--json", "number,title,body,labels,state,assignees,author,createdAt,updatedAt,url",
	}
	if search != "" {
		args = append(args, "--search", search)
	}
	out, err := a.opts.Command(ctx, args, a.env())
	if err != nil {
		return nil, fmt.Errorf("gh issue list: %w", err)
	}
	var raw []ghIssue
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("gh issue list parse: %w", err)
	}
	out2 := make([]Issue, 0, len(raw))
	for _, r := range raw {
		iss := a.toIssue(r)
		if iss.WorkflowState == "" {
			continue // doesn't match any configured state
		}
		out2 = append(out2, iss)
	}
	return out2, nil
}

// RefreshStates returns the current state for each ID (which on the
// GitHub side means: read the current labels and re-derive the
// state_mapping result).
func (a *GitHubAdapter) RefreshStates(ctx context.Context, ids []string) (map[string]string, error) {
	out := make(map[string]string, len(ids))
	for _, id := range ids {
		num, ok := parseGitHubID(a.opts.Repo, id)
		if !ok {
			continue
		}
		raw, err := a.viewIssue(ctx, num)
		if err != nil {
			continue
		}
		iss := a.toIssue(raw)
		if iss.WorkflowState != "" {
			out[id] = iss.WorkflowState
		}
	}
	return out, nil
}

// UpdateState transitions an issue by adjusting labels per the
// matching state mapping. Best-effort: if newState has no label
// mapping configured, returns ErrTransitionRejected.
func (a *GitHubAdapter) UpdateState(ctx context.Context, id, newState string) error {
	sel, ok := a.opts.StateMapping[newState]
	if !ok {
		return fmt.Errorf("%w: no label mapping for state %q", ErrTransitionRejected, newState)
	}
	num, ok := parseGitHubID(a.opts.Repo, id)
	if !ok {
		return ErrNotFound
	}
	args := []string{"issue", "edit", fmt.Sprintf("%d", num), "--repo", a.opts.Repo}
	for _, l := range sel.LabelsInclude {
		args = append(args, "--add-label", l)
	}
	for _, l := range sel.LabelsExclude {
		args = append(args, "--remove-label", l)
	}
	if _, err := a.opts.Command(ctx, args, a.env()); err != nil {
		return fmt.Errorf("gh issue edit: %w", err)
	}
	return nil
}

// Comment appends a note on the issue.
func (a *GitHubAdapter) Comment(ctx context.Context, id, body string) error {
	num, ok := parseGitHubID(a.opts.Repo, id)
	if !ok {
		return ErrNotFound
	}
	args := []string{"issue", "comment", fmt.Sprintf("%d", num), "--repo", a.opts.Repo, "--body", body}
	if _, err := a.opts.Command(ctx, args, a.env()); err != nil {
		return fmt.Errorf("gh issue comment: %w", err)
	}
	return nil
}

// Claim adds the ClaimedLabel and a marker comment (so multiple
// conductors against the same repo can observe each other's markers).
func (a *GitHubAdapter) Claim(ctx context.Context, id, marker string) error {
	num, ok := parseGitHubID(a.opts.Repo, id)
	if !ok {
		return ErrNotFound
	}
	args := []string{"issue", "edit", fmt.Sprintf("%d", num), "--repo", a.opts.Repo, "--add-label", a.opts.ClaimedLabel}
	if _, err := a.opts.Command(ctx, args, a.env()); err != nil {
		return fmt.Errorf("gh issue edit (claim): %w", err)
	}
	return nil
}

// Release removes the ClaimedLabel. Idempotent — gh ignores
// remove-label for a label that isn't present.
func (a *GitHubAdapter) Release(ctx context.Context, id, marker string) error {
	num, ok := parseGitHubID(a.opts.Repo, id)
	if !ok {
		return ErrNotFound
	}
	args := []string{"issue", "edit", fmt.Sprintf("%d", num), "--repo", a.opts.Repo, "--remove-label", a.opts.ClaimedLabel}
	if _, err := a.opts.Command(ctx, args, a.env()); err != nil {
		return fmt.Errorf("gh issue edit (release): %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// internals
// ---------------------------------------------------------------------------

// ghIssue is the JSON subset we ask gh to emit.
type ghIssue struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	State     string    `json:"state"`
	Labels    []ghLabel `json:"labels"`
	Assignees []ghUser  `json:"assignees"`
	Author    ghUser    `json:"author"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	URL       string    `json:"url"`
}

type ghLabel struct {
	Name string `json:"name"`
}

type ghUser struct {
	Login string `json:"login"`
}

func (a *GitHubAdapter) toIssue(g ghIssue) Issue {
	labels := make([]string, 0, len(g.Labels))
	for _, l := range g.Labels {
		labels = append(labels, l.Name)
	}
	// Exclude the claimed label from the surfaced labels so dispatch
	// templates render a stable view.
	labels = filterOutString(labels, a.opts.ClaimedLabel)

	id := fmt.Sprintf("github:%s#%d", a.opts.Repo, g.Number)
	assignee := ""
	if len(g.Assignees) > 0 {
		assignee = g.Assignees[0].Login
	}
	state := a.resolveState(labels)

	return Issue{
		ID:            id,
		Identifier:    fmt.Sprintf("%s#%d", a.opts.Repo, g.Number),
		Title:         g.Title,
		Body:          g.Body,
		WorkflowState: state,
		CreatedAt:     g.CreatedAt,
		UpdatedAt:     g.UpdatedAt,
		Labels:        labels,
		Assignee:      assignee,
		Metadata: map[string]string{
			"url":          g.URL,
			"github_state": g.State,
			"author":       g.Author.Login,
		},
	}
}

// resolveState delegates to the shared label helper.
func (a *GitHubAdapter) resolveState(labels []string) string {
	return resolveStateByLabels(labels, a.opts.StateMapping)
}

func (a *GitHubAdapter) env() []string {
	if a.opts.Token == "" {
		return nil
	}
	return []string{"GH_TOKEN=" + a.opts.Token, "GITHUB_TOKEN=" + a.opts.Token}
}

// buildSearch composes the --search query from include/exclude label
// hints. gh search supports `label:foo -label:bar` syntax.
func buildSearch(opts GitHubOptions) string {
	parts := make([]string, 0, len(opts.IncludeLabels)+len(opts.ExcludeLabels)+1)
	for _, l := range opts.IncludeLabels {
		parts = append(parts, "label:"+quoteLabel(l))
	}
	for _, l := range opts.ExcludeLabels {
		parts = append(parts, "-label:"+quoteLabel(l))
	}
	parts = append(parts, "-label:"+quoteLabel(opts.ClaimedLabel))
	return strings.Join(parts, " ")
}

func quoteLabel(l string) string {
	if strings.ContainsAny(l, " \t") {
		return `"` + l + `"`
	}
	return l
}

func (a *GitHubAdapter) viewIssue(ctx context.Context, num int) (ghIssue, error) {
	args := []string{
		"issue", "view", fmt.Sprintf("%d", num),
		"--repo", a.opts.Repo,
		"--json", "number,title,body,labels,state,assignees,author,createdAt,updatedAt,url",
	}
	out, err := a.opts.Command(ctx, args, a.env())
	if err != nil {
		return ghIssue{}, fmt.Errorf("gh issue view: %w", err)
	}
	var g ghIssue
	if err := json.Unmarshal(out, &g); err != nil {
		return ghIssue{}, fmt.Errorf("gh issue view parse: %w", err)
	}
	return g, nil
}

func parseGitHubID(repo, id string) (int, bool) {
	prefix := "github:" + repo + "#"
	if !strings.HasPrefix(id, prefix) {
		return 0, false
	}
	var n int
	if _, err := fmt.Sscanf(strings.TrimPrefix(id, prefix), "%d", &n); err != nil {
		return 0, false
	}
	return n, true
}

// runGH is the default Command — shells out to the user's `gh` install.
// stderr is bubbled up as part of the error so users see "gh: bad
// credentials" rather than an opaque exit code.
func runGH(ctx context.Context, args []string, env []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	if env != nil {
		cmd.Env = append(cmd.Env, env...)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("%s", msg)
	}
	return stdout.Bytes(), nil
}
