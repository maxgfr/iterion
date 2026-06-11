package github

import "testing"

const prOpenPayload = `{
  "action": "opened",
  "number": 7,
  "repository": {
    "id": 42,
    "full_name": "acme/widgets",
    "clone_url": "https://github.com/acme/widgets.git",
    "html_url": "https://github.com/acme/widgets"
  },
  "pull_request": {
    "number": 7,
    "title": "Add X",
    "body": "Implements X",
    "html_url": "https://github.com/acme/widgets/pull/7",
    "state": "open",
    "head": {"ref": "feature/x", "sha": "abc123"},
    "base": {"ref": "main"}
  },
  "sender": {"login": "alice"}
}`

func TestParsePullRequest(t *testing.T) {
	p, err := ParsePullRequest([]byte(prOpenPayload))
	if err != nil {
		t.Fatal(err)
	}
	if p.RepoID != 42 || p.ProjectPath != "acme/widgets" || p.CloneURL != "https://github.com/acme/widgets.git" {
		t.Fatalf("repo: %+v", p)
	}
	if p.PRNumber != 7 || p.SourceBranch != "feature/x" || p.TargetBranch != "main" || p.HeadSHA != "abc123" {
		t.Fatalf("pr: %+v", p)
	}
	if p.PRURL != "https://github.com/acme/widgets/pull/7" || p.SubjectID() != "pr:7" {
		t.Fatalf("url/subject: %+v", p)
	}
	if p.SenderLogin != "alice" {
		t.Fatalf("sender: %+v", p)
	}
	if !p.IsReviewable() {
		t.Fatal("opened PR should be reviewable")
	}
}

func TestParsePullRequest_MalformedFails(t *testing.T) {
	if _, err := ParsePullRequest([]byte(`{bad`)); err == nil {
		t.Fatal("malformed json should error")
	}
}

func TestIsReviewable(t *testing.T) {
	cases := []struct {
		action string
		want   bool
	}{
		{"opened", true},
		{"reopened", true},
		{"synchronize", false}, // push: re-review is on-demand, not on every push
		{"edited", false},
		{"labeled", false},
		{"closed", false},
		{"review_requested", false},
	}
	for _, c := range cases {
		p := Parsed{Action: c.action}
		if got := p.IsReviewable(); got != c.want {
			t.Errorf("action=%q => %v want %v", c.action, got, c.want)
		}
	}
}

func TestMatchEvent(t *testing.T) {
	if !MatchEvent(nil, "pull_request") || MatchEvent(nil, "push") {
		t.Fatal("default allowlist should be pull_request only")
	}
	if !MatchEvent([]string{"*"}, "anything") {
		t.Fatal("wildcard event")
	}
	if !MatchEvent([]string{"push", "pull_request"}, "push") {
		t.Fatal("explicit allow")
	}
}

func TestMatchProject(t *testing.T) {
	if !MatchProject(nil, "acme/widgets") {
		t.Fatal("empty allowlist allows all")
	}
	if !MatchProject([]string{"acme/widgets"}, "acme/widgets") || MatchProject([]string{"acme/widgets"}, "acme/gadgets") {
		t.Fatal("exact match")
	}
	if !MatchProject([]string{"acme/*"}, "acme/anything") {
		t.Fatal("prefix wildcard")
	}
	if MatchProject([]string{"acme/*"}, "other/repo") {
		t.Fatal("prefix wildcard should not cross owner")
	}
	if !MatchProject([]string{"*"}, "any/thing") {
		t.Fatal("bare wildcard")
	}
}
