package gitlab

import "testing"

const mrOpenPayload = `{
  "object_kind": "merge_request",
  "event_type": "merge_request",
  "project": {
    "id": 42,
    "path_with_namespace": "acme/widgets",
    "web_url": "https://gitlab.com/acme/widgets",
    "git_http_url": "https://gitlab.com/acme/widgets.git"
  },
  "object_attributes": {
    "iid": 7,
    "action": "open",
    "source_branch": "feature/x",
    "target_branch": "main",
    "title": "Add X",
    "description": "Implements X",
    "url": "https://gitlab.com/acme/widgets/-/merge_requests/7",
    "last_commit": { "id": "abc123" }
  },
  "labels": [{ "title": "review" }, { "title": "" }]
}`

func TestParseMergeRequest(t *testing.T) {
	p, err := ParseMergeRequest([]byte(mrOpenPayload))
	if err != nil {
		t.Fatal(err)
	}
	if p.ProjectID != 42 || p.ProjectPath != "acme/widgets" || p.CloneURL != "https://gitlab.com/acme/widgets.git" {
		t.Fatalf("project: %+v", p)
	}
	if p.MRIID != 7 || p.SourceBranch != "feature/x" || p.TargetBranch != "main" || p.HeadSHA != "abc123" {
		t.Fatalf("mr: %+v", p)
	}
	if p.MRURL != "https://gitlab.com/acme/widgets/-/merge_requests/7" || p.SubjectID() != "mr:7" {
		t.Fatalf("url/subject: %+v", p)
	}
	if len(p.Labels) != 1 || p.Labels[0] != "review" {
		t.Fatalf("labels (empty filtered): %v", p.Labels)
	}
	if !p.IsReviewable() {
		t.Fatal("open MR should be reviewable")
	}
}

func TestParseMergeRequest_RejectsNonMR(t *testing.T) {
	if _, err := ParseMergeRequest([]byte(`{"object_kind":"push"}`)); err == nil {
		t.Fatal("non-merge_request should error")
	}
	if _, err := ParseMergeRequest([]byte(`{bad`)); err == nil {
		t.Fatal("malformed json should error")
	}
}

func TestIsReviewable(t *testing.T) {
	cases := []struct {
		action, oldrev, head string
		want                 bool
	}{
		{"open", "", "h", true},
		{"reopen", "", "h", true},
		{"update", "old", "h", false}, // push no longer auto-triggers (re-review is on-demand via /revi)
		{"update", "h", "h", false},   // metadata edit
		{"update", "", "h", false},    // label-only
		{"close", "", "h", false},
		{"approved", "", "h", false},
	}
	for _, c := range cases {
		p := Parsed{Action: c.action, OldRev: c.oldrev, HeadSHA: c.head}
		if p.IsReviewable() != c.want {
			t.Errorf("action=%q oldrev=%q head=%q => %v want %v", c.action, c.oldrev, c.head, p.IsReviewable(), c.want)
		}
	}
}

func TestMatchEvent(t *testing.T) {
	if !MatchEvent(nil, "merge_request") || MatchEvent(nil, "push") {
		t.Fatal("default allowlist should be merge_request only")
	}
	if !MatchEvent([]string{"*"}, "anything") {
		t.Fatal("wildcard event")
	}
	if !MatchEvent([]string{"push", "merge_request"}, "push") {
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
	if !MatchProject([]string{"acme/*"}, "acme/anything") || !MatchProject([]string{"acme/*"}, "acme/sub/repo") {
		t.Fatal("prefix wildcard")
	}
	if MatchProject([]string{"acme/*"}, "other/repo") {
		t.Fatal("prefix wildcard should not cross group")
	}
	if !MatchProject([]string{"*"}, "any/thing") {
		t.Fatal("bare wildcard")
	}
}
