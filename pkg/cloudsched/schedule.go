// Package cloudsched is the cloud-mode recurring-bot scheduler: a per-org
// store of cron-scheduled bots and a multi-replica-safe ticker that fires each
// due schedule exactly once (CAS on the next-fire time, no leader election
// needed). The self-hosted equivalent is `iterion schedule` (host crontab);
// this is its cloud counterpart.
package cloudsched

import (
	"context"
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
)

// ScheduledBot is one cron-scheduled bot run bound to a repo integration.
type ScheduledBot struct {
	ID                string            `bson:"_id" json:"id"`
	TenantID          string            `bson:"tenant_id" json:"tenant_id"`
	RepoIntegrationID string            `bson:"repo_integration_id,omitempty" json:"repo_integration_id,omitempty"`
	BotID             string            `bson:"bot_id" json:"bot_id"`
	Cron              string            `bson:"cron" json:"cron"` // 5-field standard cron
	Vars              map[string]string `bson:"vars,omitempty" json:"vars,omitempty"`
	Disabled          bool              `bson:"disabled,omitempty" json:"disabled,omitempty"`

	// NextFireAt is the next UTC instant this schedule is due. The ticker
	// CAS-advances it the moment it claims a tick, so a second replica racing
	// on the same row finds next_fire_at already moved and backs off — exactly
	// one fire per slot without a leader.
	NextFireAt time.Time  `bson:"next_fire_at" json:"next_fire_at"`
	LastFireAt *time.Time `bson:"last_fire_at,omitempty" json:"last_fire_at,omitempty"`

	CreatedBy string    `bson:"created_by,omitempty" json:"created_by,omitempty"`
	CreatedAt time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time `bson:"updated_at" json:"updated_at"`
}

// Store persists scheduled bots. Mongo (cloud) and an in-memory impl (tests)
// satisfy it.
type Store interface {
	Create(ctx context.Context, sb ScheduledBot) error
	Get(ctx context.Context, id string) (ScheduledBot, error)
	ListByIntegration(ctx context.Context, tenantID, integrationID string) ([]ScheduledBot, error)
	// ListDue returns enabled schedules whose next_fire_at <= now (capped by
	// limit, 0 = no cap), oldest-due first.
	ListDue(ctx context.Context, now time.Time, limit int) ([]ScheduledBot, error)
	// ClaimTick atomically advances a schedule's next_fire_at from expectedNext
	// to newNext (and stamps last_fire_at = firedAt), returning true only when
	// THIS caller won the CAS. A losing replica gets (false, nil). This is the
	// exactly-once primitive — no leader election.
	ClaimTick(ctx context.Context, id string, expectedNext, newNext, firedAt time.Time) (bool, error)
	Delete(ctx context.Context, id string) error
	DeleteByIntegration(ctx context.Context, tenantID, integrationID string) error
}

// ErrNotFound is returned by Get/Delete for an unknown id.
var ErrNotFound = fmt.Errorf("cloudsched: scheduled bot not found")

// ValidateCron reports whether expr is a valid 5-field standard cron.
func ValidateCron(expr string) error {
	if _, err := cron.ParseStandard(expr); err != nil {
		return fmt.Errorf("cloudsched: invalid cron %q: %w", expr, err)
	}
	return nil
}

// NextFire returns the next instant after `after` at which expr fires.
func NextFire(expr string, after time.Time) (time.Time, error) {
	sched, err := cron.ParseStandard(expr)
	if err != nil {
		return time.Time{}, fmt.Errorf("cloudsched: invalid cron %q: %w", expr, err)
	}
	return sched.Next(after), nil
}
