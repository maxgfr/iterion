package forge

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/SocialGouv/iterion/pkg/secrets"
)

// connectionSecret is the token blob sealed on Connection.SealedPayload.
// It is never exposed outside this package — the orchestrator and refresh
// worker open it, use the token in memory, and re-seal.
type connectionSecret struct {
	// AccessToken is the OAuth-app user token or the GitHub-App installation
	// token. Empty for KindPAT.
	AccessToken string `json:"access_token,omitempty"`
	// RefreshToken renews AccessToken (OAuth apps that issue one). Empty for
	// PAT and GitHub-App (the App re-mints from its private key instead).
	RefreshToken string `json:"refresh_token,omitempty"`
	// PATToken is the operator-pasted personal access token. KindPAT only.
	PATToken string `json:"pat_token,omitempty"`
	// TokenType is the OAuth token type (usually "bearer"); informational.
	TokenType string `json:"token_type,omitempty"`
	// ExpiresAt mirrors Connection.AccessTokenExpiresAt; kept here too so an
	// opened blob is self-describing. Zero = non-expiring.
	ExpiresAt time.Time `json:"expires_at,omitempty"`
}

// AdminToken returns the token the admin client authenticates with — the
// PAT for KindPAT, else the access token.
func (s connectionSecret) AdminToken() string {
	if s.PATToken != "" {
		return s.PATToken
	}
	return s.AccessToken
}

// forgeConnAAD binds a sealed connection blob to its record id so a sealed
// payload cannot be silently transplanted to another connection (same
// convention as secrets.genericSecretAAD / webhooks.hmacSecretAAD).
func forgeConnAAD(connID string) []byte {
	return []byte("forge_conn:" + connID)
}

func sealConnectionSecret(sealer secrets.Sealer, connID string, sec connectionSecret) ([]byte, error) {
	if sealer == nil {
		return nil, fmt.Errorf("forge: nil sealer")
	}
	raw, err := json.Marshal(sec)
	if err != nil {
		return nil, fmt.Errorf("forge: marshal connection secret: %w", err)
	}
	return sealer.Seal(raw, forgeConnAAD(connID))
}

func openConnectionSecret(sealer secrets.Sealer, connID string, sealed []byte) (connectionSecret, error) {
	if sealer == nil {
		return connectionSecret{}, fmt.Errorf("forge: nil sealer")
	}
	raw, err := sealer.Open(sealed, forgeConnAAD(connID))
	if err != nil {
		return connectionSecret{}, fmt.Errorf("forge: open connection secret: %w", err)
	}
	var sec connectionSecret
	if err := json.Unmarshal(raw, &sec); err != nil {
		return connectionSecret{}, fmt.Errorf("forge: unmarshal connection secret: %w", err)
	}
	return sec, nil
}
