package gitlab

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDiscussionAndReplyClassification(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/projects/42/merge_requests/7/discussions/bot-thread":
			// Revi (id 500) opened this thread; a human (id 1) replied. A
			// system note ("changed this line…") is interleaved.
			w.Write([]byte(`{"id":"bot-thread","notes":[
				{"author":{"id":500,"username":"revi-bot"},"body":"### Review\nThe handler leaks the token at client.go:12.","system":false},
				{"author":{"id":0,"username":"ghost"},"body":"changed this line in version 2","system":true},
				{"author":{"id":1,"username":"alice"},"body":"why is that a leak?","system":false}
			]}`))
		case "/api/v4/projects/42/merge_requests/7/discussions/human-thread":
			w.Write([]byte(`{"id":"human-thread","notes":[{"author":{"id":1,"username":"alice"},"body":"lgtm"},{"author":{"id":2,"username":"bob"},"body":"+1"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	api := API{HTTP: srv.Client(), BaseURL: srv.URL, Token: "t"}
	const bot = int64(500)

	// (a) the bot authored a note in the thread → a reply is "talking to Revi".
	notes, err := api.Discussion(context.Background(), 42, 7, "bot-thread")
	if err != nil || len(notes) != 3 {
		t.Fatalf("bot-thread: notes=%d err=%v", len(notes), err)
	}
	if notes[0].AuthorUsername != "revi-bot" || !notes[1].System || notes[2].Body != "why is that a leak?" {
		t.Fatalf("note fields not parsed: %+v", notes)
	}
	if !NotesHaveAuthor(notes, bot) {
		t.Fatal("bot-thread should classify as a Revi thread")
	}
	// (b) only humans in the thread → not a Revi thread.
	human, _ := api.Discussion(context.Background(), 42, 7, "human-thread")
	if NotesHaveAuthor(human, bot) {
		t.Fatal("human-thread should not count as a Revi thread")
	}
	// (c) unknown discussion (404) → nil, no error.
	if n, err := api.Discussion(context.Background(), 42, 7, "missing"); err != nil || n != nil {
		t.Fatalf("missing: notes=%v err=%v", n, err)
	}
	// (d) guards: empty discussion id → nil without a call; zero user id →
	// never classified as a bot thread.
	if n, _ := api.Discussion(context.Background(), 42, 7, ""); n != nil {
		t.Fatal("empty discussion id → nil")
	}
	if NotesHaveAuthor(notes, 0) {
		t.Fatal("zero bot id → false")
	}
}

func TestFormatThreadTranscript(t *testing.T) {
	notes := []DiscussionNote{
		{AuthorID: 500, AuthorUsername: "revi-bot", Body: "### Review\nToken leak at client.go:12."},
		{AuthorID: 0, AuthorUsername: "ghost", Body: "changed this line in version 2", System: true},
		{AuthorID: 1, AuthorUsername: "alice", Body: "why is that a leak?"},
	}

	// Bot notes labelled, system notes skipped, chronological order.
	out := FormatThreadTranscript(notes, 500, 0)
	if !strings.Contains(out, "@revi-bot (you, the bot):") || !strings.Contains(out, "@alice:") {
		t.Fatalf("labels missing:\n%s", out)
	}
	if strings.Contains(out, "version 2") {
		t.Fatalf("system note not skipped:\n%s", out)
	}
	if strings.Index(out, "Token leak") > strings.Index(out, "why is that") {
		t.Fatalf("order not chronological:\n%s", out)
	}

	// Empty / all-system → "".
	if FormatThreadTranscript(nil, 500, 0) != "" {
		t.Fatal("nil notes → empty transcript")
	}
	if FormatThreadTranscript([]DiscussionNote{{System: true, Body: "x"}}, 0, 0) != "" {
		t.Fatal("all-system → empty transcript")
	}

	// Cap: the anchor (first note) survives, the middle is elided, the
	// newest notes win.
	long := []DiscussionNote{{AuthorID: 500, AuthorUsername: "revi-bot", Body: "ANCHOR review " + strings.Repeat("a", 100)}}
	for i := 0; i < 30; i++ {
		long = append(long, DiscussionNote{AuthorID: 1, AuthorUsername: "alice", Body: "note " + strings.Repeat("b", 100)})
	}
	long = append(long, DiscussionNote{AuthorID: 1, AuthorUsername: "alice", Body: "NEWEST question"})
	capped := FormatThreadTranscript(long, 500, 800)
	if len(capped) > 800 {
		t.Fatalf("cap not enforced: %d chars", len(capped))
	}
	if !strings.Contains(capped, "ANCHOR review") || !strings.Contains(capped, "NEWEST question") || !strings.Contains(capped, "omitted") {
		t.Fatalf("anchor/newest/omission-marker missing:\n%s", capped)
	}

	// Anchor alone over budget → rune-safe hard truncation.
	huge := []DiscussionNote{{AuthorID: 500, AuthorUsername: "revi-bot", Body: strings.Repeat("é", 600)}}
	trunc := FormatThreadTranscript(huge, 500, 401)
	if len(trunc) > 401+len("\n[… truncated …]") || !strings.HasSuffix(trunc, "[… truncated …]") {
		t.Fatalf("hard truncation wrong: %d chars, suffix %q", len(trunc), trunc[len(trunc)-20:])
	}
}
