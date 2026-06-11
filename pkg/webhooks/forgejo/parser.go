package forgejo

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// Parsed is the normalized PR view the handler consumes. Mirrors the
// shape gitlab.Parsed / github.Parsed expose so the per-provider
// handlers all look the same.
type Parsed struct {
	RepoID       int64
	ProjectPath  string
	CloneURL     string
	PRNumber     int64
	Action       string
	SourceBranch string
	TargetBranch string
	Title        string
	Description  string
	PRURL        string
	HeadSHA      string
	State        string
	SenderLogin  string
}

// ParsePullRequest decodes a Forgejo/Gitea pull_request webhook body.
// Defensive: we tolerate int64 ids (Forgejo's wire schema is int64 but
// some old fixtures used strings — json.Number would explode), missing
// top-level Number (some events nest it only inside pull_request), and
// empty branches (filter step will catch them).
func ParsePullRequest(body []byte) (Parsed, error) {
	var e PullRequestEvent
	if err := json.Unmarshal(body, &e); err != nil {
		return Parsed{}, fmt.Errorf("forgejo: decode pull_request event: %w", err)
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

// IsReviewable matches gitlab/github contracts: only opened + reopened
// auto-trigger; subsequent pushes (action "synchronized" on Gitea,
// "synchronize" on GitHub-shaped Forgejo) do NOT.
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
