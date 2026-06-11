package gitlab

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// EventHeaderNote is the value GitLab sends in X-Gitlab-Event for a
// comment / discussion reply ("Note Hook"). It is how a user "talks back"
// to the bot — a reply in the bot's discussion thread or a `/revi`
// command on the MR.
const EventHeaderNote = "Note Hook"

// NoteEvent is the subset of GitLab's note webhook we decode. We only
// model MR notes; issue/commit/snippet notes are filtered out upstream.
type NoteEvent struct {
	ObjectKind       string                `json:"object_kind"`
	EventType        string                `json:"event_type"`
	User             User                  `json:"user"`
	Project          Project               `json:"project"`
	ObjectAttributes NoteAttributes        `json:"object_attributes"`
	MergeRequest     *MergeRequestNoteable `json:"merge_request"`
}

// User is the GitLab account that authored the note — the candidate
// "replier" the handler authorizes (role-gate + allowlist), and whose
// identity the loop-prevention check compares against the bot.
type User struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
}

type NoteAttributes struct {
	ID           int64  `json:"id"`
	Note         string `json:"note"`
	NoteableType string `json:"noteable_type"` // "MergeRequest" | "Issue" | "Commit" | ...
	DiscussionID string `json:"discussion_id"`
	AuthorID     int64  `json:"author_id"`
	URL          string `json:"url"`
}

// MergeRequestNoteable is the MR a note is attached to (present on MR notes).
type MergeRequestNoteable struct {
	IID          int64  `json:"iid"`
	State        string `json:"state"` // "opened" | "closed" | "merged" — gates re-review
	SourceBranch string `json:"source_branch"`
	TargetBranch string `json:"target_branch"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	URL          string `json:"url"`
	LastCommit   Commit `json:"last_commit"`
}

// ParsedNote is the normalized note view the handler consumes — the MR it
// targets, the discussion thread to reply in, the author to authorize, and
// the note body.
type ParsedNote struct {
	ProjectID    int64
	ProjectPath  string
	CloneURL     string
	MRIID        int64
	SourceBranch string
	TargetBranch string
	MRTitle      string
	MRDesc       string
	MRURL        string
	HeadSHA      string

	// MRState gates command handling: re-review only acts on "opened"
	// MRs (closed/merged notes are filtered, not errors).
	MRState string

	NoteID       int64
	NoteBody     string
	DiscussionID string // the thread to reply in
	NoteURL      string

	AuthorID       int64
	AuthorUsername string
}

// ParseNote decodes a GitLab note webhook body.
func ParseNote(body []byte) (ParsedNote, error) {
	var e NoteEvent
	if err := json.Unmarshal(body, &e); err != nil {
		return ParsedNote{}, fmt.Errorf("gitlab: decode note event: %w", err)
	}
	if e.ObjectKind != "note" {
		return ParsedNote{}, fmt.Errorf("gitlab: not a note event (object_kind=%q)", e.ObjectKind)
	}
	oa := e.ObjectAttributes
	p := ParsedNote{
		ProjectID:      e.Project.ID,
		ProjectPath:    e.Project.PathWithNamespace,
		CloneURL:       e.Project.GitHTTPURL,
		NoteID:         oa.ID,
		NoteBody:       oa.Note,
		DiscussionID:   oa.DiscussionID,
		NoteURL:        oa.URL,
		AuthorID:       e.User.ID,
		AuthorUsername: e.User.Username,
	}
	if e.MergeRequest != nil {
		p.MRIID = e.MergeRequest.IID
		p.MRState = e.MergeRequest.State
		p.SourceBranch = e.MergeRequest.SourceBranch
		p.TargetBranch = e.MergeRequest.TargetBranch
		p.MRTitle = e.MergeRequest.Title
		p.MRDesc = e.MergeRequest.Description
		p.MRURL = e.MergeRequest.URL
		p.HeadSHA = e.MergeRequest.LastCommit.ID
	}
	// noteable_type lives on object_attributes; surface it for the MR filter.
	if oa.NoteableType != "" && oa.NoteableType != "MergeRequest" {
		return p, fmt.Errorf("gitlab: note on %q is not a merge request", oa.NoteableType)
	}
	return p, nil
}

// IsMergeRequestNote reports whether this note is attached to an MR (the
// only noteable the conversational flow handles).
func (p ParsedNote) IsMergeRequestNote() bool { return p.MRIID != 0 && p.DiscussionID != "" }

// Command extracts a leading slash-command from the note body, e.g.
// "/revi please re-review" → ("revi", "please re-review"). Returns
// ("", "") when the note does not start with a command. The match is
// case-insensitive and tolerates leading whitespace / a quote prefix
// (GitLab quote-replies prepend "> …").
func (p ParsedNote) Command() (cmd, args string) {
	for _, line := range strings.Split(p.NoteBody, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ">") {
			continue // skip blank lines and quoted context
		}
		if !strings.HasPrefix(line, "/") {
			return "", ""
		}
		rest := strings.TrimPrefix(line, "/")
		if i := strings.IndexAny(rest, " \t"); i >= 0 {
			return strings.ToLower(rest[:i]), strings.TrimSpace(rest[i:])
		}
		return strings.ToLower(rest), ""
	}
	return "", ""
}

// SubjectID is the stable per-note identifier used in delivery records +
// idempotency (one launch per note).
func (p ParsedNote) SubjectID() string {
	return "note:" + strconv.FormatInt(p.NoteID, 10)
}

// IsReviewCommand is the `/revi` specialization of Command(): true only
// for a note on an OPEN merge request whose leading slash-command is
// `revi` (args tolerated and ignored v1). Built on the generic
// extractor so the forge-conversations layer and the re-review trigger
// share one command grammar (quote-reply tolerance included).
func (p ParsedNote) IsReviewCommand() bool {
	if p.MRIID == 0 || p.MRState != "opened" {
		return false
	}
	cmd, _ := p.Command()
	return cmd == "revi"
}
