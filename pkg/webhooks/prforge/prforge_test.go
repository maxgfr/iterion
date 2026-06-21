package prforge

import "testing"

// githubOpenPR is the wire shape GitHub sends — fields ordered with
// repository before pull_request, html_url at top level on repository.
const githubOpenPR = `{
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

// forgejoOpenPR is the wire shape Forgejo/Gitea sends — pull_request
// before repository (the only structural difference from GitHub), and
// the codeberg.org URL flavour.
const forgejoOpenPR = `{
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

func TestParsePullRequest_GitHub(t *testing.T) {
	p, err := ParsePullRequest([]byte(githubOpenPR))
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

func TestParsePullRequest_Forgejo(t *testing.T) {
	p, err := ParsePullRequest([]byte(forgejoOpenPR))
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
		// GitHub spells the push action "synchronize"; Gitea spells it
		// "synchronized". Both must filter, since re-review is on-demand.
		{"synchronize", false},
		{"synchronized", false},
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

// Allowlist matching tests live in pkg/webhooks/match_test.go (the
// canonical webhooks.MatchEvent + MatchProject are exercised there with
// every provider's default kinds).
