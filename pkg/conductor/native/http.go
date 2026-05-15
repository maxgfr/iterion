package native

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/SocialGouv/iterion/pkg/conductor/tracker"
)

// RegisterRoutes mounts the native tracker's REST surface on mux under
// prefix. Pass "" to mount at the mux root.
//
//	GET    <prefix>/issues
//	POST   <prefix>/issues
//	GET    <prefix>/issues/{id}
//	PATCH  <prefix>/issues/{id}
//	DELETE <prefix>/issues/{id}
//	POST   <prefix>/issues/{id}/transition
//	GET    <prefix>/board
//	PUT    <prefix>/board
func (s *Store) RegisterRoutes(mux *http.ServeMux, prefix string) {
	p := strings.TrimSuffix(prefix, "/")
	mux.HandleFunc(p+"/issues", s.handleIssuesCollection)
	mux.HandleFunc(p+"/issues/", s.handleIssueItem)
	mux.HandleFunc(p+"/board", s.handleBoard)
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

func (s *Store) handleIssuesCollection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		filter := ListFilter{
			States: r.URL.Query()["state"],
			Labels: r.URL.Query()["label"],
		}
		if a := r.URL.Query().Get("assignee"); a != "" {
			filter.Assignee = a
		}
		issues, err := s.List(filter)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, issues)
	case http.MethodPost:
		var in issueCreateReq
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, err)
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
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusCreated, out)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
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

func (s *Store) handleIssueItem(w http.ResponseWriter, r *http.Request) {
	prefix := strings.Split(r.URL.Path, "/issues/")
	if len(prefix) != 2 || prefix[1] == "" {
		http.NotFound(w, r)
		return
	}
	rest := strings.TrimSuffix(prefix[1], "/")
	segments := strings.Split(rest, "/")
	id, err := s.Resolve(segments[0])
	if err != nil {
		if errors.Is(err, tracker.ErrNotFound) {
			http.Error(w, "issue not found", http.StatusNotFound)
			return
		}
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if len(segments) == 1 {
		s.routeIssueByMethod(w, r, id)
		return
	}
	if len(segments) == 2 && segments[1] == "transition" && r.Method == http.MethodPost {
		s.handleTransition(w, r, id)
		return
	}
	http.NotFound(w, r)
}

func (s *Store) routeIssueByMethod(w http.ResponseWriter, r *http.Request, id string) {
	switch r.Method {
	case http.MethodGet:
		iss, err := s.Get(id)
		if err != nil {
			writeErr(w, statusForErr(err), err)
			return
		}
		writeJSON(w, http.StatusOK, iss)
	case http.MethodPatch:
		var in issuePatchReq
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, err)
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
			writeErr(w, statusForErr(err), err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	case http.MethodDelete:
		if err := s.Delete(id); err != nil {
			writeErr(w, statusForErr(err), err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Store) handleTransition(w http.ResponseWriter, r *http.Request, id string) {
	var in transitionReq
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if in.To == "" {
		writeErr(w, http.StatusBadRequest, errors.New("transition: to is required"))
		return
	}
	iss, err := s.SetState(id, in.To)
	if err != nil {
		writeErr(w, statusForErr(err), err)
		return
	}
	writeJSON(w, http.StatusOK, iss)
}

// ---------------------------------------------------------------------------
// /board
// ---------------------------------------------------------------------------

func (s *Store) handleBoard(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.Board())
	case http.MethodPut:
		var b Board
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if err := s.SetBoard(&b); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, s.Board())
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
