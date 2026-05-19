package blob

import (
	"strings"
	"testing"
)

func TestArtifactKey(t *testing.T) {
	got, err := ArtifactKey("run_42", "agent_a", 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "artifacts/run_42/agent_a/3.json"
	if got != want {
		t.Errorf("ArtifactKey: got %q want %q", got, want)
	}
}

func TestArtifactKey_RejectsEmptyRunID(t *testing.T) {
	_, err := ArtifactKey("", "node", 1)
	if err == nil {
		t.Fatal("expected error on empty run_id")
	}
	if !strings.Contains(err.Error(), "run_id") {
		t.Errorf("error should mention run_id: %v", err)
	}
}

func TestArtifactKey_RejectsTraversal(t *testing.T) {
	_, err := ArtifactKey("run_1", "..", 1)
	if err == nil {
		t.Fatal("expected error on '..' in node_id")
	}
	if !strings.Contains(err.Error(), "node_id") {
		t.Errorf("error should mention node_id: %v", err)
	}
}

func TestArtifactKey_RejectsSeparator(t *testing.T) {
	_, err := ArtifactKey("run_1", "node/with-slash", 1)
	if err == nil {
		t.Fatal("expected error on '/' in component")
	}
}

func TestArtifactKey_RejectsNegativeVersion(t *testing.T) {
	_, err := ArtifactKey("run_1", "node", -1)
	if err == nil {
		t.Fatal("expected error on negative version")
	}
	if !strings.Contains(err.Error(), "negative") {
		t.Errorf("error should mention 'negative': %v", err)
	}
}

func TestAttachmentKey(t *testing.T) {
	got, err := AttachmentKey("run_42", "uploads", "doc.pdf")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "attachments/run_42/uploads/doc.pdf"
	if got != want {
		t.Errorf("AttachmentKey: got %q want %q", got, want)
	}
}

func TestAttachmentKey_RejectsSeparatorInFilename(t *testing.T) {
	_, err := AttachmentKey("run_1", "uploads", "../etc/passwd")
	if err == nil {
		t.Fatal("expected error on traversal in filename")
	}
}

func TestAttachmentRunPrefix(t *testing.T) {
	got, err := AttachmentRunPrefix("run_42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "attachments/run_42/"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestAttachmentRunPrefix_RejectsEmpty(t *testing.T) {
	_, err := AttachmentRunPrefix("")
	if err == nil {
		t.Fatal("expected error on empty run_id")
	}
}
