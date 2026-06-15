package native

import (
	"os"
	"path/filepath"
	"testing"
)

// A torn final line (crash mid-append) must not swallow the next event:
// writeEventLineLocked separates the torn bytes with a newline so the
// following record stays its own line. Before the fix (O_WRONLY, no
// separator) the next append concatenated onto the torn bytes and a
// tailer skipping the corrupt line lost BOTH events.
func TestWriteEventLineSeparatesTornTail(t *testing.T) {
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	p := filepath.Join(s.root, eventsFile)

	writeEvt := func(typ string) {
		t.Helper()
		s.mu.Lock()
		defer s.mu.Unlock()
		if err := s.writeEventLineLocked(Event{Type: EventType(typ)}); err != nil {
			t.Fatalf("writeEventLineLocked(%s): %v", typ, err)
		}
	}

	writeEvt("created")

	// Simulate a crash mid-append: a partial JSON record, no newline.
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatalf("open for corruption: %v", err)
	}
	if _, err := f.WriteString(`{"type":"torn`); err != nil {
		t.Fatalf("write torn bytes: %v", err)
	}
	_ = f.Close()

	writeEvt("moved")

	var got []string
	if err := s.ScanEvents(func(e *Event) bool {
		got = append(got, string(e.Type))
		return true
	}); err != nil {
		t.Fatalf("ScanEvents: %v", err)
	}

	// The torn line is skipped, but both valid events survive.
	has := func(want string) bool {
		for _, g := range got {
			if g == want {
				return true
			}
		}
		return false
	}
	if !has("created") || !has("moved") {
		t.Fatalf("events after torn write = %v; want both 'created' and 'moved' present", got)
	}
}
