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

const noteRevi = `{
  "object_kind": "note",
  "project": {"id": 42, "path_with_namespace": "acme/widgets", "git_http_url": "https://gitlab.com/acme/widgets.git"},
  "user": {"username": "alice"},
  "object_attributes": {"id": 99, "note": "/revi", "noteable_type": "MergeRequest", "author_id": 1},
  "merge_request": {"iid": 7, "state": "opened", "source_branch": "feature/x", "target_branch": "main",
    "title": "Add X", "description": "desc", "url": "https://gitlab.com/acme/widgets/-/merge_requests/7",
    "last_commit": {"id": "headsha"}}
}`

// Parser-level note tests (TestParseNote, TestParseNote_NonMR,
// TestNoteCommand) live in note_test.go next to the parser. Here we
// cover the /revi specialization the re-review trigger consumes: MR
// state + command grammar through IsReviewCommand, against the same
// payload shape the handler sees.
func TestParseNote_ReviewCommandEndToEnd(t *testing.T) {
	p, err := ParseNote([]byte(noteRevi))
	if err != nil {
		t.Fatal(err)
	}
	if p.MRState != "opened" || p.AuthorUsername != "alice" {
		t.Fatalf("note: %+v", p)
	}
	if !p.IsReviewCommand() {
		t.Fatal("bare /revi on an open MR should be a review command")
	}
}

func TestIsReviewCommand(t *testing.T) {
	base := ParsedNote{MRIID: 7, MRState: "opened"}
	cases := []struct {
		note string
		want bool
	}{
		{"/revi", true},
		{"/revi focus=security", true},
		{"   /revi   ", true}, // surrounding whitespace tolerated
		{"please run /revi", false},
		{"/revia", false},               // longer token; must NOT match
		{"/REVI", true},                 // Command() is case-insensitive by design
		{"> /revi quoted\n/revi", true}, // quote-reply prefix skipped (Command grammar)
		{"> some quoted context\nhi", false},
		{"", false},
		{"hi", false},
	}
	for _, c := range cases {
		p := base
		p.NoteBody = c.note
		if got := p.IsReviewCommand(); got != c.want {
			t.Errorf("note=%q => %v want %v", c.note, got, c.want)
		}
	}
	// closed MR is filtered even with the exact command
	closed := ParsedNote{MRIID: 7, MRState: "closed", NoteBody: "/revi"}
	if closed.IsReviewCommand() {
		t.Fatal("closed MR must filter /revi")
	}
	// non-MR note (no MR attached — commit/issue/snippet) is filtered
	issue := ParsedNote{MRState: "opened", NoteBody: "/revi"}
	if issue.IsReviewCommand() {
		t.Fatal("non-MR note must filter")
	}
}

func TestMatchEvent(t *testing.T) {
	// empty allowlist: merge_request + note both allowed; everything
	// else (push/pipeline/…) denied. Lets a zero-config GitLab webhook
	// reach both the auto-review and the /revi paths.
	if !MatchEvent(nil, "merge_request") || !MatchEvent(nil, "note") || MatchEvent(nil, "push") {
		t.Fatal("default allowlist should be {merge_request, note}")
	}
	if !MatchEvent([]string{"*"}, "anything") {
		t.Fatal("wildcard event")
	}
	if !MatchEvent([]string{"push", "merge_request"}, "push") {
		t.Fatal("explicit allow")
	}
	// explicit allowlist excludes by omission (gates /revi off).
	if MatchEvent([]string{"merge_request"}, "note") {
		t.Fatal("explicit allowlist must exclude unlisted kinds")
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
