package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// callbackToken is the opaque value the adapter hands to iterion at
// launch and gets back verbatim in the completion webhook. It carries
// only ROUTING ids — never an identity, never message content — so a
// completion callback can be routed to the right thread without the
// adapter keeping per-run server-side state.
type callbackToken struct {
	ChannelID string `json:"c"`
	RootID    string `json:"r"`
}

// encodeToken serialises t to a URL-safe base64 JSON string.
func encodeToken(t callbackToken) string {
	b, _ := json.Marshal(t) // shape is fixed; marshal cannot fail
	return base64.RawURLEncoding.EncodeToString(b)
}

// decodeToken parses a token produced by encodeToken.
func decodeToken(s string) (callbackToken, error) {
	var t callbackToken
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return t, fmt.Errorf("decode token: %w", err)
	}
	if err := json.Unmarshal(raw, &t); err != nil {
		return t, fmt.Errorf("unmarshal token: %w", err)
	}
	if t.ChannelID == "" || t.RootID == "" {
		return t, fmt.Errorf("token missing channel/root id")
	}
	return t, nil
}
