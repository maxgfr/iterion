package memory

import (
	"reflect"
	"testing"
)

func TestBuildIndex_EmptyScope(t *testing.T) {
	t.Setenv("ITERION_HOME", t.TempDir())
	s, _ := OpenScope("/tmp/wn", "session-continuity")
	got, err := s.BuildIndex()
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("empty scope: got %+v", got)
	}
}

func TestBuildIndex_FromFrontmatter(t *testing.T) {
	t.Setenv("ITERION_HOME", t.TempDir())
	s, _ := OpenScope("/tmp/wn", "whats-next")
	body := "---\n" +
		"title: \"Operator hard constraints\"\n" +
		"description: Running list of constraints captured from operator feedback.\n" +
		"tags: [constraint, operator, hard]\n" +
		"---\n\n" +
		"# Body heading should be ignored\n"
	_ = s.Write("constraints.md", []byte(body))

	got, err := s.BuildIndex()
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %+v", got)
	}
	e := got[0]
	if e.Path != "constraints.md" {
		t.Fatalf("path: %q", e.Path)
	}
	if e.Title != "Operator hard constraints" {
		t.Fatalf("title: %q", e.Title)
	}
	if e.Description != "Running list of constraints captured from operator feedback." {
		t.Fatalf("description: %q", e.Description)
	}
	if !reflect.DeepEqual(e.Tags, []string{"constraint", "operator", "hard"}) {
		t.Fatalf("tags: %v", e.Tags)
	}
}

func TestBuildIndex_FallsBackToFirstH1(t *testing.T) {
	t.Setenv("ITERION_HOME", t.TempDir())
	s, _ := OpenScope("/tmp/wn", "whats-next")
	_ = s.Write("brief.md", []byte("# Session brief\n\nbody body body\n"))
	_ = s.Write("notitle.md", []byte("plain text, no heading\n"))

	got, err := s.BuildIndex()
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %+v", got)
	}
	titles := map[string]string{got[0].Path: got[0].Title, got[1].Path: got[1].Title}
	if titles["brief.md"] != "Session brief" {
		t.Fatalf("brief title: %q", titles["brief.md"])
	}
	if titles["notitle.md"] != "" {
		t.Fatalf("notitle title should be empty, got %q", titles["notitle.md"])
	}
}

func TestBuildIndex_RecursiveLexicographic(t *testing.T) {
	t.Setenv("ITERION_HOME", t.TempDir())
	s, _ := OpenScope("/tmp/wn", "whats-next")
	_ = s.Write("zeta.md", []byte("# Z\n"))
	_ = s.Write("alpha.md", []byte("# A\n"))
	_ = s.Write("decisions/2026-05-18.md", []byte("# Dropped X\n"))
	_ = s.Write("learnings/style.md", []byte("# Style\n"))

	got, err := s.BuildIndex()
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}
	want := []string{"alpha.md", "decisions/2026-05-18.md", "learnings/style.md", "zeta.md"}
	gotPaths := make([]string, len(got))
	for i, e := range got {
		gotPaths[i] = e.Path
	}
	if !reflect.DeepEqual(gotPaths, want) {
		t.Fatalf("paths: got %v want %v", gotPaths, want)
	}
}

func TestBuildIndex_FrontmatterWithoutTitle_FallsBackToBodyH1(t *testing.T) {
	t.Setenv("ITERION_HOME", t.TempDir())
	s, _ := OpenScope("/tmp/wn", "whats-next")
	body := "---\ntags: [foo]\n---\n# Fallback heading\nbody\n"
	_ = s.Write("partial.md", []byte(body))

	got, _ := s.BuildIndex()
	if len(got) != 1 || got[0].Title != "Fallback heading" {
		t.Fatalf("got %+v", got)
	}
	if !reflect.DeepEqual(got[0].Tags, []string{"foo"}) {
		t.Fatalf("tags: %v", got[0].Tags)
	}
}
