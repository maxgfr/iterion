package gitlab

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDiscussionHasBotNote(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/projects/42/merge_requests/7/discussions/bot-thread":
			// Revi (id 500) opened this thread; a human (id 1) replied.
			w.Write([]byte(`{"id":"bot-thread","notes":[{"author":{"id":500}},{"author":{"id":1}}]}`))
		case "/api/v4/projects/42/merge_requests/7/discussions/human-thread":
			w.Write([]byte(`{"id":"human-thread","notes":[{"author":{"id":1}},{"author":{"id":2}}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	api := API{HTTP: srv.Client(), BaseURL: srv.URL, Token: "t"}
	const bot = int64(500)

	// (a) the bot authored a note in the thread → a reply is "talking to Revi".
	if in, err := api.DiscussionHasBotNote(context.Background(), 42, 7, "bot-thread", bot); err != nil || !in {
		t.Fatalf("bot-thread: in=%v err=%v", in, err)
	}
	// (b) only humans in the thread → not a Revi thread.
	if in, _ := api.DiscussionHasBotNote(context.Background(), 42, 7, "human-thread", bot); in {
		t.Fatal("human-thread should not count as a Revi thread")
	}
	// (c) unknown discussion (404) → false, no error.
	if in, err := api.DiscussionHasBotNote(context.Background(), 42, 7, "missing", bot); err != nil || in {
		t.Fatalf("missing: in=%v err=%v", in, err)
	}
	// (d) guards: empty discussion id / zero bot id → false without a call.
	if in, _ := api.DiscussionHasBotNote(context.Background(), 42, 7, "", bot); in {
		t.Fatal("empty discussion id → false")
	}
	if in, _ := api.DiscussionHasBotNote(context.Background(), 42, 7, "bot-thread", 0); in {
		t.Fatal("zero bot id → false")
	}
}
