package gitlab

import "testing"

func TestParseNote(t *testing.T) {
	body := []byte(`{
		"object_kind":"note","event_type":"note",
		"user":{"id":42,"username":"alice","name":"Alice"},
		"project":{"id":194,"path_with_namespace":"devthejo/revi-playground","git_http_url":"https://gitlab.example/devthejo/revi-playground.git"},
		"object_attributes":{"id":900,"note":"/revi please re-review","noteable_type":"MergeRequest","discussion_id":"abc123","url":"https://gitlab.example/x/notes/900"},
		"merge_request":{"iid":7,"source_branch":"feat/x","target_branch":"main","title":"T","url":"https://gitlab.example/x/merge_requests/7","last_commit":{"id":"deadbeef"}}
	}`)
	p, err := ParseNote(body)
	if err != nil {
		t.Fatal(err)
	}
	if p.MRIID != 7 || p.DiscussionID != "abc123" || p.AuthorUsername != "alice" || p.AuthorID != 42 || p.NoteID != 900 {
		t.Fatalf("parsed: %+v", p)
	}
	if p.CloneURL == "" || p.HeadSHA != "deadbeef" {
		t.Fatalf("mr context not carried: %+v", p)
	}
	if !p.IsMergeRequestNote() {
		t.Fatal("should be an MR note")
	}
	if p.SubjectID() != "note:900" {
		t.Fatalf("subject: %s", p.SubjectID())
	}
	if cmd, args := p.Command(); cmd != "revi" || args != "please re-review" {
		t.Fatalf("cmd=%q args=%q", cmd, args)
	}
}

func TestParseNote_Issue(t *testing.T) {
	body := []byte(`{
		"object_kind":"note","event_type":"note",
		"user":{"id":42,"username":"alice"},
		"project":{"id":194,"path_with_namespace":"devthejo/revi-playground","git_http_url":"https://gitlab.example/devthejo/revi-playground.git","default_branch":"main"},
		"object_attributes":{"id":901,"note":"/featurly add an export endpoint","noteable_type":"Issue","url":"https://gitlab.example/x/notes/901"},
		"issue":{"iid":12,"title":"Add export","description":"please","url":"https://gitlab.example/devthejo/revi-playground/-/issues/12","state":"opened"}
	}`)
	p, err := ParseNote(body)
	if err != nil {
		t.Fatal(err)
	}
	if !p.IsIssueNote() || p.IsMergeRequestNote() {
		t.Fatalf("should be an issue note, not an MR note: %+v", p)
	}
	if p.IssueIID != 12 || p.IssueURL == "" || p.IssueState != "opened" || p.DefaultBranch != "main" {
		t.Fatalf("issue context not carried: %+v", p)
	}
	if cmd, args := p.Command(); cmd != "featurly" || args != "add an export endpoint" {
		t.Fatalf("cmd=%q args=%q", cmd, args)
	}
}

func TestParseNote_Unroutable(t *testing.T) {
	// A note on a Commit/Snippet is neither MR nor Issue → error (the handler
	// filters it 200). An Issue note, by contrast, now parses (see above).
	body := []byte(`{"object_kind":"note","object_attributes":{"id":1,"note":"hi","noteable_type":"Commit"}}`)
	if _, err := ParseNote(body); err == nil {
		t.Fatal("expected error for a commit note (unroutable noteable)")
	}
}

func TestNoteCommand(t *testing.T) {
	cases := []struct{ body, cmd, args string }{
		{"/revi", "revi", ""},
		{"/revi do it now", "revi", "do it now"},
		{"just a plain reply", "", ""},
		{"> quoted bot line\n/revi go", "revi", "go"},
		{"  /REVI  Mixed ", "revi", "Mixed"},
	}
	for _, c := range cases {
		cmd, args := ParsedNote{NoteBody: c.body}.Command()
		if cmd != c.cmd || args != c.args {
			t.Errorf("Command(%q) = (%q,%q); want (%q,%q)", c.body, cmd, args, c.cmd, c.args)
		}
	}
}
