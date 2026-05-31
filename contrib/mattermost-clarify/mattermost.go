package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// MattermostDriver implements ChannelDriver against a Mattermost
// server: a WebSocket event stream for inbound posts, and the REST API
// for posting replies + interactive consent prompts.
//
// NOTE: the WebSocket event envelope + REST shapes follow Mattermost's
// documented API. This driver compiles (against the vendored
// gorilla/websocket) and is structurally complete, but must be
// validated against a live Mattermost instance (see README) — it is the
// one part of the adapter that cannot be unit-tested without a server.
type MattermostDriver struct {
	httpBase    string // e.g. https://mm.example.com
	wsURL       string // e.g. wss://mm.example.com/api/v4/websocket
	token       string // bot access token
	botUserID   string // bot's user id (to flag FromBot)
	actionURL   string // adapter's /mm/actions endpoint (consent button target)
	actionToken string // shared secret embedded in button contexts (auth)
	httpClient  *http.Client
}

// MattermostConfig configures a MattermostDriver.
type MattermostConfig struct {
	HTTPBase    string
	WSURL       string
	Token       string
	BotUserID   string
	ActionURL   string
	ActionToken string
}

// NewMattermostDriver builds a driver.
func NewMattermostDriver(cfg MattermostConfig) *MattermostDriver {
	return &MattermostDriver{
		httpBase:    strings.TrimRight(cfg.HTTPBase, "/"),
		wsURL:       cfg.WSURL,
		token:       cfg.Token,
		botUserID:   cfg.BotUserID,
		actionURL:   cfg.ActionURL,
		actionToken: cfg.ActionToken,
		httpClient:  &http.Client{Timeout: 15 * time.Second},
	}
}

// --- inbound (WebSocket) ---------------------------------------------

// wsEnvelope is the Mattermost WebSocket event envelope.
type wsEnvelope struct {
	Event string `json:"event"`
	Data  struct {
		// Post is a JSON-encoded string of the post object.
		Post        string `json:"post"`
		ChannelType string `json:"channel_type"`
		Mentions    string `json:"mentions"` // JSON array string of mentioned user ids
	} `json:"data"`
	Seq int `json:"seq"`
}

// mmPost is the subset of the Mattermost post object we consume.
type mmPost struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id"`
	RootID    string `json:"root_id"`
	UserID    string `json:"user_id"`
	Message   string `json:"message"`
	CreateAt  int64  `json:"create_at"`
}

// Listen dials the Mattermost WS, authenticates, and streams posts.
func (m *MattermostDriver) Listen(ctx context.Context) (<-chan InboundPost, error) {
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, m.wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("mattermost ws dial: %w", err)
	}
	// Authentication challenge (Mattermost WS handshake).
	auth := map[string]any{
		"seq":    1,
		"action": "authentication_challenge",
		"data":   map[string]string{"token": m.token},
	}
	if err := conn.WriteJSON(auth); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("mattermost ws auth: %w", err)
	}

	// gorilla's ReadJSON is blocking and not ctx-aware; close the
	// connection when ctx is cancelled so the read loop unblocks.
	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()

	out := make(chan InboundPost)
	go func() {
		defer close(out)
		defer conn.Close()
		for {
			var env wsEnvelope
			if err := conn.ReadJSON(&env); err != nil {
				return // ctx cancelled (conn closed above) or read error
			}
			if env.Event != "posted" || env.Data.Post == "" {
				continue
			}
			var p mmPost
			if err := json.Unmarshal([]byte(env.Data.Post), &p); err != nil {
				continue
			}
			root := p.RootID
			if root == "" {
				root = p.ID // a thread's root post has empty root_id
			}
			select {
			case <-ctx.Done():
				return
			case out <- InboundPost{
				Thread:         ThreadRef{ChannelID: p.ChannelID, RootID: root},
				UserID:         p.UserID,
				Text:           p.Message,
				CreateAtMillis: p.CreateAt,
				MentionsBot:    m.botUserID != "" && strings.Contains(env.Data.Mentions, m.botUserID),
				FromBot:        p.UserID == m.botUserID,
			}:
			}
		}
	}()
	return out, nil
}

// --- outbound (REST) -------------------------------------------------

// createPost POSTs a message into a thread.
func (m *MattermostDriver) createPost(ctx context.Context, payload map[string]any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.httpBase+"/api/v4/posts", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+m.token)
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("mattermost createPost: status %d", resp.StatusCode)
	}
	return nil
}

// PostReply posts text as a threaded reply.
func (m *MattermostDriver) PostReply(ctx context.Context, thread ThreadRef, text string) error {
	return m.createPost(ctx, map[string]any{
		"channel_id": thread.ChannelID,
		"root_id":    thread.RootID,
		"message":    text,
	})
}

// RequestConsent posts an interactive Accept/Decline prompt addressed to
// userID. The buttons POST back to the adapter's action endpoint, which
// translates them into ConsentActions.
func (m *MattermostDriver) RequestConsent(ctx context.Context, thread ThreadRef, userID string) error {
	mkAction := func(name string, granted bool) map[string]any {
		return map[string]any{
			"id":   name,
			"name": name,
			"integration": map[string]any{
				"url": m.actionURL,
				"context": map[string]any{
					"action":     "consent",
					"granted":    granted,
					"user_id":    userID,
					"channel_id": thread.ChannelID,
					"root_id":    thread.RootID,
				},
			},
		}
	}
	return m.createPost(ctx, map[string]any{
		"channel_id": thread.ChannelID,
		"root_id":    thread.RootID,
		"message":    "👋 **@clarify-bot** can help clarify this thread. It will only read messages from people who agree. Do you consent to your messages in *this thread* being sent to the assistant?",
		"props": map[string]any{
			"attachments": []map[string]any{{
				"text": "Your choice applies to this thread only. You can decline and still take part — your messages just won't be sent.",
				"actions": []map[string]any{
					mkAction("Accept", true),
					mkAction("Decline", false),
				},
			}},
		},
	})
}
