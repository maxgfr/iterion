// Package github decodes GitHub webhook payloads into the same narrow
// normalized shape iterion's inbound handler consumes for GitLab. We
// model only the pull_request fields the review-PR flow needs — never
// the whole event — so the handler can persist selected fields + a
// payload hash without retaining the raw body.
package github

// EventHeaderPullRequest is the X-GitHub-Event value for a PR event.
// GitHub also sends events like "ping", "push", "issue_comment" etc.;
// they all share one route and we filter on the header.
const EventHeaderPullRequest = "pull_request"

// PullRequestEvent is the subset of GitHub's pull_request webhook we
// decode. Field names follow the GitHub camelCase-on-the-wire pattern.
type PullRequestEvent struct {
	Action      string      `json:"action"`
	Number      int64       `json:"number"`
	Repository  Repository  `json:"repository"`
	PullRequest PullRequest `json:"pull_request"`
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
