// Package webhooks is iterion's inbound-webhook spine: long-lived,
// per-org webhook tokens that authenticate an external caller (a forge,
// CI, a script) and authorize it to launch a configured set of bots.
//
// It is the first long-lived-token concept in iterion (operator auth is
// short-lived JWT + refresh). Tokens are shown once and stored only as a
// salted hash + last4 + fingerprint, mirroring the invitation/session
// token pattern in pkg/auth.
//
// This package is provider-agnostic; the GitLab merge-request handler
// that consumes it lives in pkg/webhooks/gitlab + the server route.
package webhooks

import "time"

// Provider identifies the external event source.
type Provider string

const (
	ProviderGitLab Provider = "gitlab"
)

// Rate is a token-bucket rate limit for a webhook.
type Rate struct {
	Rate  float64 `bson:"rate" json:"rate"`   // sustained tokens/second
	Burst float64 `bson:"burst" json:"burst"` // bucket capacity
}

// Config is a per-org inbound webhook. The token plaintext is returned
// exactly once at create/rotate; only TokenHash/TokenLast4/Fingerprint
// persist.
type Config struct {
	ID          string   `bson:"_id" json:"id"`
	TenantID    string   `bson:"tenant_id" json:"tenant_id"`
	Name        string   `bson:"name" json:"name"`
	Provider    Provider `bson:"provider" json:"provider"`
	Enabled     bool     `bson:"enabled" json:"enabled"`
	TokenHash   string   `bson:"token_hash" json:"-"`
	TokenLast4  string   `bson:"token_last4" json:"token_last4"`
	Fingerprint string   `bson:"fingerprint,omitempty" json:"fingerprint,omitempty"`

	// Bot scoping. BotIDs lists the allowed bot names; WildcardBots
	// (BotIDs == ["*"]) permits any bot and must be set explicitly so
	// the UI + audit can flag it.
	BotIDs       []string `bson:"bot_ids" json:"bot_ids"`
	WildcardBots bool     `bson:"wildcard_bots,omitempty" json:"wildcard_bots,omitempty"`
	DefaultBotID string   `bson:"default_bot_id,omitempty" json:"default_bot_id,omitempty"`

	// Source allowlists (empty = allow-all within the tenant).
	ProjectAllowlist []string `bson:"project_allowlist,omitempty" json:"project_allowlist,omitempty"`
	EventAllowlist   []string `bson:"event_allowlist,omitempty" json:"event_allowlist,omitempty"`

	// Limits.
	RateLimit        Rate `bson:"rate_limit" json:"rate_limit"`
	MonthlyCallLimit int  `bson:"monthly_call_limit,omitempty" json:"monthly_call_limit,omitempty"` // 0 = inherit org

	// LaunchVars are stamped onto every run launched through this webhook
	// (e.g. severity_threshold), overriding the handler-derived vars.
	LaunchVars map[string]string `bson:"launch_vars,omitempty" json:"launch_vars,omitempty"`

	// KeyOverrides pins a BYOK key per LLM provider for runs launched
	// through this webhook (provider name → api_key id), overriding the
	// org/user default in secrets.Resolve. Lets several webhooks for the
	// same bot bill against different keys. See docs/byok.md.
	KeyOverrides map[string]string `bson:"key_overrides,omitempty" json:"key_overrides,omitempty"`

	// SecretOverrides pins a specific stored secret per workflow-secret name
	// (name -> secret id) for runs launched through this webhook, overriding
	// the org bot-secret binding in secrets.ResolveGenericWithBindings. Lets
	// several webhooks for the same bot post under different forge tokens /
	// bot identities. See docs/byok.md.
	SecretOverrides map[string]string `bson:"secret_overrides,omitempty" json:"secret_overrides,omitempty"`

	// AuthorizedRepliers + MinReplierRole gate who may "talk back" to the bot
	// via a note (a /revi command or a reply): a note author is authorized
	// when they are in AuthorizedRepliers (usernames with/without @, or numeric
	// ids) OR a project member at >= MinReplierRole (guest|reporter|developer|
	// maintainer|owner; empty → developer). See docs/forge-conversations.md.
	AuthorizedRepliers []string `bson:"authorized_repliers,omitempty" json:"authorized_repliers,omitempty"`
	MinReplierRole     string   `bson:"min_replier_role,omitempty" json:"min_replier_role,omitempty"`

	CreatedBy  string     `bson:"created_by" json:"created_by"`
	CreatedAt  time.Time  `bson:"created_at" json:"created_at"`
	UpdatedAt  time.Time  `bson:"updated_at" json:"updated_at"`
	LastUsedAt *time.Time `bson:"last_used_at,omitempty" json:"last_used_at,omitempty"`
	RotatedAt  *time.Time `bson:"rotated_at,omitempty" json:"rotated_at,omitempty"`
}

// AllowsBot reports whether this webhook may launch botID.
func (c *Config) AllowsBot(botID string) bool {
	if c == nil || botID == "" {
		return false
	}
	if c.WildcardBots {
		return true
	}
	for _, b := range c.BotIDs {
		if b == "*" || b == botID {
			return true
		}
	}
	return false
}

// SelectBot resolves which bot a delivery should launch: the explicit
// default, else the sole allowed bot. Returns "" when ambiguous (the
// caller decides per-provider, e.g. GitLab V1 pins Revi).
func (c *Config) SelectBot() string {
	if c == nil {
		return ""
	}
	if c.DefaultBotID != "" {
		return c.DefaultBotID
	}
	if len(c.BotIDs) == 1 && c.BotIDs[0] != "*" {
		return c.BotIDs[0]
	}
	return ""
}

// Delivery records an inbound webhook delivery for audit + idempotency.
// It NEVER stores the raw payload — only a hash and the selected fields.
type Delivery struct {
	ID             string   `bson:"_id" json:"id"`
	TenantID       string   `bson:"tenant_id" json:"tenant_id"`
	WebhookID      string   `bson:"webhook_id" json:"webhook_id"`
	Provider       Provider `bson:"provider" json:"provider"`
	IdempotencyKey string   `bson:"idempotency_key" json:"idempotency_key"`

	EventKind   string `bson:"event_kind,omitempty" json:"event_kind,omitempty"`
	EventAction string `bson:"event_action,omitempty" json:"event_action,omitempty"`
	ProjectPath string `bson:"project_path,omitempty" json:"project_path,omitempty"`
	SubjectID   string `bson:"subject_id,omitempty" json:"subject_id,omitempty"`
	SubjectSHA  string `bson:"subject_sha,omitempty" json:"subject_sha,omitempty"`
	PayloadHash string `bson:"payload_hash,omitempty" json:"payload_hash,omitempty"`

	Status     string     `bson:"status" json:"status"`
	BotID      string     `bson:"bot_id,omitempty" json:"bot_id,omitempty"`
	RunID      string     `bson:"run_id,omitempty" json:"run_id,omitempty"`
	Error      string     `bson:"error,omitempty" json:"error,omitempty"`
	SourceIP   string     `bson:"source_ip,omitempty" json:"source_ip,omitempty"`
	ReceivedAt time.Time  `bson:"received_at" json:"received_at"`
	LaunchedAt *time.Time `bson:"launched_at,omitempty" json:"launched_at,omitempty"`
}

// Delivery status values.
const (
	StatusAccepted      = "accepted"
	StatusDuplicate     = "duplicate"
	StatusRateLimited   = "rate_limited"
	StatusQuotaExceeded = "quota_exceeded"
	StatusInvalid       = "invalid"
	StatusFiltered      = "filtered"
	StatusLaunched      = "launched"
	StatusLaunchError   = "launch_error"
)
