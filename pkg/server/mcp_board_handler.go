package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/SocialGouv/iterion/pkg/conductor/native"
	"github.com/SocialGouv/iterion/pkg/conductor/native/boardops"
)

// BoardMCPTokenRegistry is the ephemeral token store used to authorize
// sandbox-running bots that talk to the host's board MCP HTTP endpoint.
// The runtime calls Register at the start of every run, then Revoke at
// the end. A token grants exactly the capabilities recorded at registration
// time; the bot sends them back as `X-Iterion-Caps` so the handler can run
// a consistent capability check.
type BoardMCPTokenRegistry struct {
	mu     sync.RWMutex
	tokens map[string]boardMCPGrant
}

type boardMCPGrant struct {
	Capabilities map[string]bool
}

// NewBoardMCPTokenRegistry returns an empty registry.
func NewBoardMCPTokenRegistry() *BoardMCPTokenRegistry {
	return &BoardMCPTokenRegistry{tokens: map[string]boardMCPGrant{}}
}

// Register stores a token with its grant. A subsequent call with the same
// token replaces the grant.
func (r *BoardMCPTokenRegistry) Register(token string, caps []string) {
	grant := boardMCPGrant{Capabilities: map[string]bool{}}
	for _, c := range caps {
		grant.Capabilities[c] = true
	}
	r.mu.Lock()
	r.tokens[token] = grant
	r.mu.Unlock()
}

// Revoke removes the token. A revoked token's subsequent calls fail with 401.
func (r *BoardMCPTokenRegistry) Revoke(token string) {
	r.mu.Lock()
	delete(r.tokens, token)
	r.mu.Unlock()
}

func (r *BoardMCPTokenRegistry) lookup(token string) (boardMCPGrant, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	g, ok := r.tokens[token]
	return g, ok
}

// RegisterBoardMCPRoutes wires the board MCP HTTP endpoints onto the given
// mux. The handler authenticates via `X-Iterion-Run` (token issued by the
// runtime at run-start), gates each tool by the granted capability set,
// and dispatches into boardops. The endpoint speaks line-delimited JSON-RPC
// over POST — one request per body, one response.
func RegisterBoardMCPRoutes(mux *http.ServeMux, prefix string, store *native.Store, reg *BoardMCPTokenRegistry) {
	h := &boardMCPHandler{store: store, registry: reg}
	mux.HandleFunc("POST "+strings.TrimRight(prefix, "/"), h.serve)
	mux.HandleFunc("POST "+strings.TrimRight(prefix, "/")+"/", h.serve)
}

type boardMCPHandler struct {
	store    *native.Store
	registry *BoardMCPTokenRegistry
}

type mcpReq struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method"`
	Params  json.RawMessage  `json:"params,omitempty"`
}

type mcpResp struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id"`
	Result  any              `json:"result,omitempty"`
	Error   *mcpRespError    `json:"error,omitempty"`
}

type mcpRespError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (h *boardMCPHandler) serve(w http.ResponseWriter, r *http.Request) {
	token := r.Header.Get("X-Iterion-Run")
	if token == "" {
		http.Error(w, "missing X-Iterion-Run header", http.StatusUnauthorized)
		return
	}
	grant, ok := h.registry.lookup(token)
	if !ok {
		http.Error(w, "unknown run token", http.StatusUnauthorized)
		return
	}

	var req mcpReq
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil {
		writeJSONStatus(w, http.StatusBadRequest, mcpResp{
			JSONRPC: "2.0",
			Error:   &mcpRespError{Code: -32700, Message: "parse error: " + err.Error()},
		})
		return
	}
	if req.ID == nil {
		// Notification — no response expected, no work to do for now.
		w.WriteHeader(http.StatusNoContent)
		return
	}
	resp := dispatchHTTP(req, h.store, grant.Capabilities)
	writeJSONStatus(w, http.StatusOK, resp)
}

func dispatchHTTP(req mcpReq, store *native.Store, grants map[string]bool) mcpResp {
	resp := mcpResp{JSONRPC: "2.0", ID: req.ID}

	caps := boardops.Capabilities(grants)
	switch req.Method {
	case "initialize":
		resp.Result = map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "iterion-board-http"},
		}
	case "tools/list":
		tools := boardops.ToolsFor(caps)
		entries := make([]map[string]any, 0, len(tools))
		for _, t := range tools {
			entries = append(entries, map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"inputSchema": t.InputSchema,
			})
		}
		resp.Result = map[string]any{"tools": entries}
	case "tools/call":
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			resp.Error = &mcpRespError{Code: -32602, Message: "invalid params: " + err.Error()}
			return resp
		}
		raw, err := boardops.Call(store, caps, params.Name, params.Arguments)
		if err != nil {
			if errors.Is(err, boardops.ErrCapabilityDenied) {
				resp.Error = &mcpRespError{Code: -32601, Message: err.Error()}
				return resp
			}
			resp.Result = map[string]any{
				"content": []map[string]any{{"type": "text", "text": err.Error()}},
				"isError": true,
			}
			return resp
		}
		resp.Result = map[string]any{
			"content": []map[string]any{{"type": "text", "text": string(raw)}},
			"isError": false,
		}
	default:
		resp.Error = &mcpRespError{Code: -32601, Message: "method not found: " + req.Method}
	}
	return resp
}

func writeJSONStatus(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
