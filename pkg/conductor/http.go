package conductor

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

// Routes returns an http.Handler exposing the conductor's REST + WS
// surface. Mount it under a prefix like "/api/v1/conductor".
func (c *Conductor) Routes() http.Handler {
	mux := http.NewServeMux()
	c.RegisterRoutes(mux, "")
	return mux
}

// RegisterRoutes registers the conductor's HTTP handlers on the given
// mux under the supplied prefix. Pass "" to mount at the mux root.
// Method-specific patterns are used so registration coexists with
// other method+path routes (e.g. the editor server's CORS preflight).
func (c *Conductor) RegisterRoutes(mux *http.ServeMux, prefix string) {
	p := strings.TrimSuffix(prefix, "/")
	mux.HandleFunc("GET "+p+"/state", c.handleState)
	mux.HandleFunc("POST "+p+"/refresh", c.handleRefresh)
	mux.HandleFunc("POST "+p+"/reload", c.handleReload)
	mux.HandleFunc("GET "+p+"/issues/{id}", c.handleIssueDetail)
	mux.HandleFunc("POST "+p+"/issues/{id}/cancel", c.handleIssueCancel)
	mux.HandleFunc("GET "+p+"/ws", c.handleWS)
}

func (c *Conductor) handleState(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, c.Snapshot())
}

func (c *Conductor) handleRefresh(w http.ResponseWriter, _ *http.Request) {
	c.Refresh()
	WriteJSON(w, http.StatusAccepted, map[string]bool{"queued": true})
}

func (c *Conductor) handleReload(w http.ResponseWriter, _ *http.Request) {
	cfg, err := Load(c.cfg.Load().SourcePath)
	if err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	c.Reload(cfg)
	WriteJSON(w, http.StatusOK, map[string]any{"reloaded": true, "polling_interval_s": cfg.PollingInterval().Seconds()})
}

func (c *Conductor) handleIssueDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	snap := c.Snapshot()
	for _, r := range snap.Running {
		if r.IssueID == id {
			WriteJSON(w, http.StatusOK, r)
			return
		}
	}
	for _, r := range snap.Retries {
		if r.IssueID == id {
			WriteJSON(w, http.StatusOK, r)
			return
		}
	}
	http.Error(w, "issue not tracked by conductor", http.StatusNotFound)
}

func (c *Conductor) handleIssueCancel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	c.Cancel(id)
	WriteJSON(w, http.StatusAccepted, map[string]string{"issue_id": id, "status": "cancel_requested"})
}

// ---------------------------------------------------------------------------
// WebSocket fan-out
// ---------------------------------------------------------------------------

// conductorUpgrader is permissive — the conductor binds to localhost
// by default and the operator chooses when to expose it. Operators in
// hostile environments should run the conductor behind a reverse proxy
// that enforces origin policy.
var conductorUpgrader = websocket.Upgrader{
	CheckOrigin: func(*http.Request) bool { return true },
}

func (c *Conductor) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := conductorUpgrader.Upgrade(w, r, nil)
	if err != nil {
		c.logger.Warn("conductor: ws upgrade: %v", err)
		return
	}
	// Push the current snapshot immediately so a fresh subscriber
	// gets state without waiting for the next tick.
	c.ws.attach(conn, c.Snapshot())
}

// wsClientConn wraps a single websocket subscription.
type wsClientConn struct {
	conn *websocket.Conn
	send chan []byte
}

// wsBridge fans Snapshot publications out to every connected client.
type wsBridge struct {
	mu      sync.Mutex
	clients map[*wsClientConn]struct{}
}

func newWsBridge() *wsBridge {
	return &wsBridge{clients: map[*wsClientConn]struct{}{}}
}

func (b *wsBridge) attach(conn *websocket.Conn, initial Snapshot) {
	client := &wsClientConn{
		conn: conn,
		send: make(chan []byte, 16),
	}
	b.mu.Lock()
	b.clients[client] = struct{}{}
	b.mu.Unlock()

	if data, err := json.Marshal(initial); err == nil {
		select {
		case client.send <- data:
		default:
		}
	}

	go client.writer(b)
	client.reader(b)
}

func (b *wsBridge) drop(client *wsClientConn) {
	b.mu.Lock()
	if _, ok := b.clients[client]; ok {
		delete(b.clients, client)
		close(client.send)
	}
	b.mu.Unlock()
	_ = client.conn.Close()
}

func (b *wsBridge) broadcast(s Snapshot) {
	data, err := json.Marshal(s)
	if err != nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for client := range b.clients {
		select {
		case client.send <- data:
		default:
			// slow consumer: drop, let the writer's close kick them off.
		}
	}
}

func (c *wsClientConn) writer(b *wsBridge) {
	defer b.drop(c)
	for data := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
			return
		}
	}
}

func (c *wsClientConn) reader(b *wsBridge) {
	defer b.drop(c)
	c.conn.SetReadLimit(1 << 16)
	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) &&
				!errors.Is(err, websocket.ErrCloseSent) {
				return
			}
			return
		}
	}
}
