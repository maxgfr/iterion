package forgejo

import "testing"

const prOpenPayload = `{
  "action": "opened",
  "number": 7,
  "pull_request": {
    "number": 7,
    "title": "Add X",
    "body": "Implements X",
    "html_url": "https://codeberg.org/acme/widgets/pulls/7",
    "state": "open",
    "head": {"ref": "feature/x", "sha": "abc123"},
    "base": {"ref": "main"}
  },
  "repository": {
    "id": 42,
    "full_name": "acme/widgets",
    "clone_url": "https://codeberg.org/acme/widgets.git"
  },
  "sender": {"login": "alice"}
}`

func TestParsePullRequest(t *testing.T) {
	p, err := ParsePullRequest([]byte(prOpenPayload))
	if err != nil {
		t.Fatal(err)
	}
	if p.RepoID != 42 || p.ProjectPath != "acme/widgets" || p.CloneURL != "https://codeberg.org/acme/widgets.git" {
		t.Fatalf("repo: %+v", p)
	}
	if p.PRNumber != 7 || p.SourceBranch != "feature/x" || p.TargetBranch != "main" || p.HeadSHA != "abc123" {
		t.Fatalf("pr: %+v", p)
	}
	if p.SubjectID() != "pr:7" || p.SenderLogin != "alice" {
		t.Fatalf("subject/sender: %+v", p)
	}
	if !p.IsReviewable() {
		t.Fatal("opened should be reviewable")
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
		// Gitea spells the push action "synchronized" (vs GitHub's
		// "synchronize"); both must filter, since re-review is on-demand.
		{"synchronized", false},
		{"synchronize", false},
		{"edited", false},
		{"closed", false},
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
}

func TestMatchProject(t *testing.T) {
	if !MatchProject(nil, "any/thing") {
		t.Fatal("empty allowlist allows all")
	}
	if !MatchProject([]string{"acme/*"}, "acme/x") {
		t.Fatal("prefix wildcard")
	}
}
