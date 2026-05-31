package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// LaunchClient submits `clarify` runs to an iterion server over its
// HTTP launch API (POST /api/runs) and asks for a completion webhook
// back to this adapter.
type LaunchClient struct {
	baseURL     string // iterion server base, e.g. http://localhost:8080
	authToken   string // optional bearer for iterion's auth middleware
	botSource   string // the clarify .bot source, sent inline
	callbackURL string // where iterion POSTs the completion webhook
	model       string // optional model override (vars.model)
	http        *http.Client
}

// LaunchConfig configures a LaunchClient.
type LaunchConfig struct {
	BaseURL     string
	AuthToken   string
	BotSource   string
	CallbackURL string
	Model       string
	Timeout     time.Duration
}

// NewLaunchClient builds a LaunchClient.
func NewLaunchClient(cfg LaunchConfig) *LaunchClient {
	to := cfg.Timeout
	if to <= 0 {
		to = 30 * time.Second
	}
	return &LaunchClient{
		baseURL:     cfg.BaseURL,
		authToken:   cfg.AuthToken,
		botSource:   cfg.BotSource,
		callbackURL: cfg.CallbackURL,
		model:       cfg.Model,
		http:        &http.Client{Timeout: to},
	}
}

// launchRequest mirrors the subset of pkg/server.launchRunRequest the
// adapter needs. Kept local so the adapter doesn't import the server
// package (which would drag in the whole HTTP stack).
type launchRequest struct {
	Source             string            `json:"source"`
	FilePath           string            `json:"file_path,omitempty"`
	Vars               map[string]string `json:"vars,omitempty"`
	CallbackURL        string            `json:"callback_url,omitempty"`
	CallbackToken      string            `json:"callback_token,omitempty"`
	CallbackAnswerNode string            `json:"callback_answer_node,omitempty"`
}

type launchResponse struct {
	RunID  string `json:"run_id"`
	Status string `json:"status"`
}

// Launch submits a clarify run for the given anonymised transcript and
// returns the new run id. The completion webhook (carrying
// final_answer) arrives asynchronously at the adapter's callback URL,
// tagged with token.
func (c *LaunchClient) Launch(ctx context.Context, transcript, latest, threadID, token string) (string, error) {
	vars := map[string]string{
		"transcript":     transcript,
		"latest_message": latest,
		"thread_id":      threadID,
	}
	if c.model != "" {
		vars["model"] = c.model
	}
	body, err := json.Marshal(launchRequest{
		Source:        c.botSource,
		FilePath:      "clarify/main.bot",
		Vars:          vars,
		CallbackURL:   c.callbackURL,
		CallbackToken: token,
		// The clarify bot publishes its answer from the `facilitator`
		// node; pinning it lets the notifier resolve final_answer
		// without scanning every artifact.
		CallbackAnswerNode: "facilitator",
	})
	if err != nil {
		return "", fmt.Errorf("marshal launch: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/runs", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build launch request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("launch run: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("launch run: iterion returned %d", resp.StatusCode)
	}
	var lr launchResponse
	if err := json.NewDecoder(resp.Body).Decode(&lr); err != nil {
		return "", fmt.Errorf("decode launch response: %w", err)
	}
	return lr.RunID, nil
}
