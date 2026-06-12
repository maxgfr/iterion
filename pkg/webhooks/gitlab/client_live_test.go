//go:build live

package gitlab

import (
	"context"
	"os"
	"strconv"
	"testing"
)

// TestLiveDiscussionTranscript exercises Discussion + FormatThreadTranscript
// against a real GitLab instance — the same calls the webhook note gate
// makes to build the converse bot's {{vars.thread_context}}. Skipped unless
// the env quad is set:
//
//	GITLAB_LIVE_TOKEN=<PRIVATE-TOKEN>          (never logged)
//	GITLAB_LIVE_BASE_URL=https://gitlab.example.com
//	GITLAB_LIVE_PROJECT_ID=<numeric project id>
//	GITLAB_LIVE_MR_IID=<merge request iid>
//
// e.g. the revi-playground fixture: project 194, MR 7 on
// gitlab.fabrique.social.gouv.fr (threads authored by the Revi bot user).
func TestLiveDiscussionTranscript(t *testing.T) {
	token, base := os.Getenv("GITLAB_LIVE_TOKEN"), os.Getenv("GITLAB_LIVE_BASE_URL")
	projectStr, iidStr := os.Getenv("GITLAB_LIVE_PROJECT_ID"), os.Getenv("GITLAB_LIVE_MR_IID")
	if token == "" || base == "" || projectStr == "" || iidStr == "" {
		t.Skip("GITLAB_LIVE_TOKEN / GITLAB_LIVE_BASE_URL / GITLAB_LIVE_PROJECT_ID / GITLAB_LIVE_MR_IID not set")
	}
	project, err := strconv.ParseInt(projectStr, 10, 64)
	if err != nil {
		t.Fatalf("GITLAB_LIVE_PROJECT_ID: %v", err)
	}
	iid, err := strconv.ParseInt(iidStr, 10, 64)
	if err != nil {
		t.Fatalf("GITLAB_LIVE_MR_IID: %v", err)
	}
	api := API{BaseURL: base, Token: token}
	ctx := context.Background()

	bot, err := api.CurrentUser(ctx)
	if err != nil {
		t.Fatalf("CurrentUser: %v", err)
	}
	t.Logf("token user: %s (id %d)", bot.Username, bot.ID)

	// List the MR discussions raw (one extra GET the production gate does
	// not need — it receives discussion_id from the webhook payload).
	var discussions []struct {
		ID    string `json:"id"`
		Notes []struct {
			Author struct {
				ID int64 `json:"id"`
			} `json:"author"`
		} `json:"notes"`
	}
	code, err := api.get(ctx, "/projects/"+projectStr+"/merge_requests/"+iidStr+"/discussions?per_page=50", &discussions)
	if err != nil || code != 200 {
		t.Fatalf("list discussions: code=%d err=%v", code, err)
	}
	if len(discussions) == 0 {
		t.Skipf("MR %s!%s has no discussions to exercise", projectStr, iidStr)
	}

	for _, d := range discussions {
		notes, err := api.Discussion(ctx, project, iid, d.ID)
		if err != nil {
			t.Fatalf("Discussion(%s): %v", d.ID, err)
		}
		if len(notes) != len(d.Notes) {
			t.Fatalf("Discussion(%s): %d notes, list endpoint says %d", d.ID, len(notes), len(d.Notes))
		}
		inThread := NotesHaveAuthor(notes, bot.ID)
		transcript := FormatThreadTranscript(notes, bot.ID, 16000)
		if len(notes) > 0 && transcript == "" {
			// Only system notes can legitimately produce an empty transcript.
			for _, n := range notes {
				if !n.System {
					t.Fatalf("Discussion(%s): non-system notes but empty transcript", d.ID)
				}
			}
		}
		t.Logf("discussion %.12s: %d notes, botInThread=%v, transcript=%d chars", d.ID, len(notes), inThread, len(transcript))
	}

	// 404 discussion → benign nil (same contract the unit test pins, but
	// against the real API's 404 shape).
	if notes, err := api.Discussion(ctx, project, iid, "deadbeefdeadbeefdeadbeefdeadbeef"); err != nil || notes != nil {
		t.Fatalf("unknown discussion: notes=%v err=%v", notes, err)
	}
}
