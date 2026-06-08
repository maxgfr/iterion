package model

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/SocialGouv/iterion/pkg/backend/delegate"
)

// ContextOverflowError indicates the prompt exceeded the model's context window.
type ContextOverflowError struct {
	Message string
}

func (e *ContextOverflowError) Error() string {
	return e.Message
}

// APIError represents a non-overflow API error.
type APIError struct {
	Message     string
	StatusCode  int
	IsRetryable bool
}

func (e *APIError) Error() string {
	return e.Message
}

// ClassifyStreamError parses a stream error event and returns the appropriate
// typed error (*ContextOverflowError or *APIError), or nil if the data is not
// a recognized error event.
func ClassifyStreamError(body []byte) error {
	var obj struct {
		Type  string `json:"type"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &obj); err != nil {
		return nil
	}
	if obj.Type != "error" {
		return nil
	}

	switch obj.Error.Code {
	case "context_length_exceeded":
		return &ContextOverflowError{Message: "Input exceeds context window of this model"}
	case "insufficient_quota":
		return &APIError{Message: "Quota exceeded. Check your plan and billing details."}
	case "usage_not_included":
		return &APIError{Message: "To use Codex with your ChatGPT plan, upgrade to Plus."}
	case "invalid_prompt":
		msg := "Invalid prompt."
		if obj.Error.Message != "" {
			msg = obj.Error.Message
		}
		return &APIError{Message: msg}
	}

	return nil
}

// streamTransportMarkers identify a stream `error` event that stems from a
// truncated / partially-read stream — the protocol-shape failures the
// generic network-signature list (delegate.MatchesNetworkSignature) does
// NOT cover. Genuine network/transport signatures (connection reset, EOF,
// timeout, 5xx, …) are matched via that shared list so the two never
// drift; these are only the stream-reader-specific additions
// (claw-code-go surfaces them as "read stream: …", "openai stream
// read: …", "… truncated …", "parse SSE: …").
var streamTransportMarkers = []string{
	"read stream", "stream read", "parse sse", "truncat", "incomplete",
}

// classifyStreamEventError turns a stream `error` event's message into a
// typed error. A recognised provider error (quota, context overflow,
// invalid prompt) keeps its permanent classification via
// ClassifyStreamError. A transport / truncation failure — matched either by
// the shared network-signature list or a stream-reader-specific marker — is
// wrapped as a RETRYABLE *APIError so the retry loop re-issues the request
// instead of surfacing a half-response as if it were complete. Anything
// else stays a plain, non-retryable stream error.
func classifyStreamEventError(msg string) error {
	if classified := ClassifyStreamError([]byte(msg)); classified != nil {
		return classified
	}
	if delegate.MatchesNetworkSignature(msg) || matchesStreamTransportMarker(msg) {
		return &APIError{Message: "stream error: " + msg, IsRetryable: true}
	}
	return fmt.Errorf("stream error: %s", msg)
}

func matchesStreamTransportMarker(msg string) bool {
	lower := strings.ToLower(msg)
	for _, marker := range streamTransportMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}
