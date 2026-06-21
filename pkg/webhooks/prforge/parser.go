package prforge

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// Parsed is the normalized PR view the inbound handler consumes. It
// mirrors gitlab.Parsed field-for-field so the per-provider handlers
// (GitHub + Forgejo/Gitea, both routed through this package) all look
// the same — SenderLogin is audit-only in V1 and not stored on the
// delivery row.
type Parsed struct {
	RepoID       int64
	ProjectPath  string // "owner/repo"
	CloneURL     string
	PRNumber     int64
	Action       string // "opened" | "reopened" | "synchronize" | "synchronized" | …
	SourceBranch string // head.ref
	TargetBranch string // base.ref
	Title        string
	Description  string
	PRURL        string
	HeadSHA      string
	State        string
	SenderLogin  string
}

// ParsePullRequest decodes a pull_request webhook body from GitHub or
// Forgejo/Gitea (one shared wire shape). We reject empty bodies / wrong
// shapes early so the handler can return a clean 400 instead of crashing
// on a nil deref. Defensive: tolerate missing top-level Number (some
// events nest it only inside pull_request).
func ParsePullRequest(body []byte) (Parsed, error) {
	var e PullRequestEvent
	if err := json.Unmarshal(body, &e); err != nil {
		return Parsed{}, fmt.Errorf("prforge: decode pull_request event: %w", err)
	}
	pr := e.PullRequest
	if pr.Number == 0 && e.Number != 0 {
		pr.Number = e.Number
	}
	return Parsed{
		RepoID:       e.Repository.ID,
		ProjectPath:  e.Repository.FullName,
		CloneURL:     e.Repository.CloneURL,
		PRNumber:     pr.Number,
		Action:       e.Action,
		SourceBranch: pr.Head.Ref,
		TargetBranch: pr.Base.Ref,
		Title:        pr.Title,
		Description:  pr.Body,
		PRURL:        pr.HTMLURL,
		HeadSHA:      pr.Head.SHA,
		State:        pr.State,
		SenderLogin:  e.Sender.Login,
	}, nil
}

// IsReviewable reports whether the PR action should AUTO-trigger a
// review. Same contract as gitlab.Parsed.IsReviewable — only opened /
// reopened. Subsequent push actions ("synchronize" on GitHub-shaped
// payloads, "synchronized" on Gitea-shaped payloads) deliberately do
// NOT re-trigger; re-review is on-demand.
func (p Parsed) IsReviewable() bool {
	switch p.Action {
	case "opened", "reopened":
		return true
	default:
		return false
	}
}

// SubjectID is the stable per-PR identifier used in delivery records.
func (p Parsed) SubjectID() string {
	return "pr:" + strconv.FormatInt(p.PRNumber, 10)
}
