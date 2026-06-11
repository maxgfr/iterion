// Package forgejo decodes Forgejo and Gitea webhook payloads (the
// projects share a wire format identical for PR events). We mirror the
// gitlab/github narrow-shape pattern: only the fields the review-PR
// handler reads, no whole-event retention.
package forgejo

// Forgejo and Gitea both send the same event header VALUE
// ("pull_request") on different header NAMES. The handler accepts
// either; this constant is the header value to check against.
const EventHeaderPullRequestValue = "pull_request"

// PullRequestEvent is the subset of Forgejo/Gitea's pull_request
// webhook we decode.
type PullRequestEvent struct {
	Action      string      `json:"action"`
	Number      int64       `json:"number"`
	PullRequest PullRequest `json:"pull_request"`
	Repository  Repository  `json:"repository"`
	Sender      Sender      `json:"sender"`
}

type Repository struct {
	ID       int64  `json:"id"`
	FullName string `json:"full_name"` // "owner/repo"
	CloneURL string `json:"clone_url"`
	HTMLURL  string `json:"html_url"`
}

type PullRequest struct {
	Number  int64  `json:"number"`
	Title   string `json:"title"`
	Body    string `json:"body"`
	HTMLURL string `json:"html_url"`
	State   string `json:"state"`
	Head    Ref    `json:"head"`
	Base    Ref    `json:"base"`
}

type Ref struct {
	Ref string `json:"ref"`
	SHA string `json:"sha"`
}

type Sender struct {
	Login string `json:"login"`
}
