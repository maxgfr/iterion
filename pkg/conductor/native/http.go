package native

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/SocialGouv/iterion/pkg/conductor"
	"github.com/SocialGouv/iterion/pkg/conductor/tracker"
)

// RegisterRoutes mounts the native tracker's REST surface on mux under
// prefix. Pass "" to mount at the mux root.
//
// We register one pattern per (method, path) so Go 1.22's ServeMux
// doesn't flag ambiguities against other catch-all method routes
// (e.g. the server's `OPTIONS /api/` CORS preflight).
func (s *Store) RegisterRoutes(mux *http.ServeMux, prefix string) {
	p := strings.TrimSuffix(prefix, "/")
	mux.HandleFunc("GET "+p+"/issues", s.handleListIssues)
	mux.HandleFunc("POST "+p+"/issues", s.handleCreateIssue)
	mux.HandleFunc("GET "+p+"/issues/{id}", s.handleGetIssue)
	mux.HandleFunc("PATCH "+p+"/issues/{id}", s.handlePatchIssue)
	mux.HandleFunc("DELETE "+p+"/issues/{id}", s.handleDeleteIssue)
	mux.HandleFunc("POST "+p+"/issues/{id}/transition", s.handleTransitionIssue)
	mux.HandleFunc("GET "+p+"/board", s.handleGetBoard)
	mux.HandleFunc("PUT "+p+"/board", s.handlePutBoard)
}

// ---------------------------------------------------------------------------
// /issues
// ---------------------------------------------------------------------------

type issueCreateReq struct {
	Title    string         `json:"title"`
	Body     string         `json:"body,omitempty"`
	State    string         `json:"state,omitempty"`
	Labels   []string       `json:"labels,omitempty"`
	Priority int            `json:"priority,omitempty"`
	Assignee string         `json:"assignee,omitempty"`
	Blockers []string       `json:"blockers,omitempty"`
	Fields   map[string]any `json:"fields,omitempty"`
}

func (s *Store) handleListIssues(w http.ResponseWriter, r *http.Request) {
	filter := ListFilter{
		States: r.URL.Query()["state"],
		Labels: r.URL.Query()["label"],
	}
	if a := r.URL.Query().Get("assignee"); a != "" {
		filter.Assignee = a
	}
	issues, err := s.List(filter)
	if err != nil {
		conductor.WriteErr(w, http.StatusInternalServerError, err)
		return
	}
	conductor.WriteJSON(w, http.StatusOK, issues)
}

func (s *Store) handleCreateIssue(w http.ResponseWriter, r *http.Request) {
	var in issueCreateReq
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		conductor.WriteErr(w, http.StatusBadRequest, err)
		return
	}
	iss := Issue{
		Title:    in.Title,
		Body:     in.Body,
		State:    in.State,
		Labels:   in.Labels,
		Priority: in.Priority,
		Assignee: in.Assignee,
		Blockers: in.Blockers,
		Fields:   in.Fields,
	}
	out, err := s.Create(iss)
	if err != nil {
		conductor.WriteErr(w, http.StatusBadRequest, err)
		return
	}
	conductor.WriteJSON(w, http.StatusCreated, out)
}

// ---------------------------------------------------------------------------
// /issues/{id} and /issues/{id}/transition
// ---------------------------------------------------------------------------

type issuePatchReq struct {
	Title    *string        `json:"title,omitempty"`
	Body     *string        `json:"body,omitempty"`
	Labels   *[]string      `json:"labels,omitempty"`
	Priority *int           `json:"priority,omitempty"`
	Assignee *string        `json:"assignee,omitempty"`
	Blockers *[]string      `json:"blockers,omitempty"`
	Fields   map[string]any `json:"fields,omitempty"`
}

type transitionReq struct {
	To string `json:"to"`
}

// resolvePathID extracts the {id} segment, runs prefix resolution, and
// returns the full ID. On miss/ambiguity writes the appropriate HTTP
// error and returns "" + false.
func (s *Store) resolvePathID(w http.ResponseWriter, r *http.Request) (string, bool) {
	raw := r.PathValue("id")
	if raw == "" {
		http.NotFound(w, r)
		return "", false
	}
	id, err := s.Resolve(raw)
	if err != nil {
		if errors.Is(err, tracker.ErrNotFound) {
			http.Error(w, "issue not found", http.StatusNotFound)
			return "", false
		}
		conductor.WriteErr(w, http.StatusBadRequest, err)
		return "", false
	}
	return id, true
}

func (s *Store) handleGetIssue(w http.ResponseWriter, r *http.Request) {
	id, ok := s.resolvePathID(w, r)
	if !ok {
		return
	}
	iss, err := s.Get(id)
	if err != nil {
		conductor.WriteErr(w, statusForErr(err), err)
		return
	}
	conductor.WriteJSON(w, http.StatusOK, iss)
}

func (s *Store) handlePatchIssue(w http.ResponseWriter, r *http.Request) {
	id, ok := s.resolvePathID(w, r)
	if !ok {
		return
	}
	var in issuePatchReq
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		conductor.WriteErr(w, http.StatusBadRequest, err)
		return
	}
	out, err := s.Update(id, Patch{
		Title:    in.Title,
		Body:     in.Body,
		Labels:   in.Labels,
		Priority: in.Priority,
		Assignee: in.Assignee,
		Blockers: in.Blockers,
		Fields:   in.Fields,
	})
	if err != nil {
		conductor.WriteErr(w, statusForErr(err), err)
		return
	}
	conductor.WriteJSON(w, http.StatusOK, out)
}

func (s *Store) handleDeleteIssue(w http.ResponseWriter, r *http.Request) {
	id, ok := s.resolvePathID(w, r)
	if !ok {
		return
	}
	if err := s.Delete(id); err != nil {
		conductor.WriteErr(w, statusForErr(err), err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Store) handleTransitionIssue(w http.ResponseWriter, r *http.Request) {
	id, ok := s.resolvePathID(w, r)
	if !ok {
		return
	}
	var in transitionReq
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		conductor.WriteErr(w, http.StatusBadRequest, err)
		return
	}
	if in.To == "" {
		conductor.WriteErr(w, http.StatusBadRequest, errors.New("transition: to is required"))
		return
	}
	iss, err := s.SetState(id, in.To)
	if err != nil {
		conductor.WriteErr(w, statusForErr(err), err)
		return
	}
	conductor.WriteJSON(w, http.StatusOK, iss)
}

// ---------------------------------------------------------------------------
// /board
// ---------------------------------------------------------------------------

func (s *Store) handleGetBoard(w http.ResponseWriter, _ *http.Request) {
	conductor.WriteJSON(w, http.StatusOK, s.Board())
}

func (s *Store) handlePutBoard(w http.ResponseWriter, r *http.Request) {
	var b Board
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		conductor.WriteErr(w, http.StatusBadRequest, err)
		return
	}
	if err := s.SetBoard(&b); err != nil {
		conductor.WriteErr(w, http.StatusBadRequest, err)
		return
	}
	conductor.WriteJSON(w, http.StatusOK, s.Board())
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func statusForErr(err error) int {
	switch {
	case errors.Is(err, tracker.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, tracker.ErrTransitionRejected),
		errors.Is(err, tracker.ErrClaimConflict):
		return http.StatusConflict
	default:
		return http.StatusBadRequest
	}
}
