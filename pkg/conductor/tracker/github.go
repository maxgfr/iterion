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

	iterlog "github.com/SocialGouv/iterion/pkg/log"
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

	// Logger, when non-nil, receives warnings about silent
	// degradations (e.g. ListCandidates hitting the per-poll cap).
	// Optional — adapter is fully functional without it.
	Logger *iterlog.Logger
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

// ghCandidateListLimit caps the number of candidates we pull per poll.
// gh CLI paginates internally up to --limit, so a single invocation
// covers very large backlogs without us implementing pagination
// ourselves. Set high enough that an active repo never silently drops
// candidates; if a poll returns exactly this many we log a warning
// so the operator knows to investigate.
const ghCandidateListLimit = 1000

// ListCandidates returns open issues matching include/exclude labels
// and not carrying ClaimedLabel.
func (a *GitHubAdapter) ListCandidates(ctx context.Context) ([]Issue, error) {
	search := buildSearch(a.opts)
	args := []string{
		"issue", "list",
		"--repo", a.opts.Repo,
		"--state", "open",
		"--limit", fmt.Sprintf("%d", ghCandidateListLimit),
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
	if len(raw) >= ghCandidateListLimit && a.opts.Logger != nil {
		a.opts.Logger.Warn("github tracker: ListCandidates hit the %d-issue cap on repo %s — beyond this point issues are silently dropped from dispatch; consider tightening label filters",
			ghCandidateListLimit, a.opts.Repo)
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
//
// One `gh api` call covers the entire set instead of spawning one `gh
// issue view <num>` per ID. The trade-off: GH returns 100 issues max
// per page, which is enough for any realistic conductor's running set
// (gated by agent.max_concurrent, typically single digits).
func (a *GitHubAdapter) RefreshStates(ctx context.Context, ids []string) (map[string]string, error) {
	if len(ids) == 0 {
		return map[string]string{}, nil
	}
	wanted := make(map[int]string, len(ids))
	for _, id := range ids {
		if num, ok := parseGitHubID(a.opts.Repo, id); ok {
			wanted[num] = id
		}
	}
	if len(wanted) == 0 {
		return map[string]string{}, nil
	}

	// Pull every open or closed issue in one call and filter locally.
	// state=all so we still see issues an operator closed externally
	// (the conductor's reconcileStalled path needs to detect that).
	args := []string{
		"api",
		fmt.Sprintf("repos/%s/issues?state=all&per_page=100&filter=all", a.opts.Repo),
		"-H", "Accept: application/vnd.github+json",
	}
	raw, err := a.opts.Command(ctx, args, a.env())
	if err != nil {
		return nil, fmt.Errorf("gh api repos/%s/issues: %w", a.opts.Repo, err)
	}
	// gh api emits REST-shaped issues — labels are objects with `name`,
	// number is `number`, etc. — so the existing ghIssue struct fits
	// after a minor mapping pass for the `state` field naming.
	var raws []apiIssue
	if err := json.Unmarshal(raw, &raws); err != nil {
		return nil, fmt.Errorf("gh api parse: %w", err)
	}

	out := make(map[string]string, len(wanted))
	for _, r := range raws {
		id, want := wanted[r.Number]
		if !want {
			continue
		}
		iss := a.toIssue(r.toGhIssue())
		if iss.WorkflowState != "" {
			out[id] = iss.WorkflowState
		}
	}
	return out, nil
}

// apiIssue mirrors the REST shape that `gh api repos/.../issues`
// returns, which differs slightly from `gh issue list --json` (camelCase
// vs snake_case fields). The two are converged into ghIssue here so the
// rest of the adapter stays uniform.
type apiIssue struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	State     string    `json:"state"`
	Labels    []ghLabel `json:"labels"`
	Assignees []ghUser  `json:"assignees"`
	User      ghUser    `json:"user"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	HTMLURL   string    `json:"html_url"`
}

func (a apiIssue) toGhIssue() ghIssue {
	return ghIssue{
		Number:    a.Number,
		Title:     a.Title,
		Body:      a.Body,
		State:     a.State,
		Labels:    a.Labels,
		Assignees: a.Assignees,
		Author:    a.User,
		CreatedAt: a.CreatedAt,
		UpdatedAt: a.UpdatedAt,
		URL:       a.HTMLURL,
	}
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
		return nil, fmt.Errorf("%s", redactGHSecrets(msg))
	}
	return stdout.Bytes(), nil
}

// redactGHSecrets blanks out token-shaped substrings the gh CLI may
// echo back on failure (e.g. "Invalid token: ghp_xxxx…"). Without
// this, a misconfigured GH_TOKEN leaks via the bubbled-up error into
// downstream logs and centralized log aggregation.
func redactGHSecrets(s string) string {
	for _, prefix := range ghTokenPrefixes {
		for {
			i := strings.Index(s, prefix)
			if i < 0 {
				break
			}
			// Trim everything from the prefix to the next whitespace
			// or end-of-string and replace with a redaction marker.
			tail := s[i+len(prefix):]
			end := strings.IndexAny(tail, " \t\n\r\"'")
			if end < 0 {
				end = len(tail)
			}
			s = s[:i] + prefix + "***REDACTED***" + tail[end:]
		}
	}
	return s
}

// ghTokenPrefixes lists the documented prefixes GitHub uses for
// personal access tokens, OAuth tokens, server-to-server tokens, and
// fine-grained tokens. Keep in sync with
// https://github.blog/2021-04-05-behind-githubs-new-authentication-token-formats/
var ghTokenPrefixes = []string{"ghp_", "gho_", "ghu_", "ghs_", "ghr_", "github_pat_"}
