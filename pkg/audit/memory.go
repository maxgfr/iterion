package audit

import (
	"context"
	"sort"
	"sync"
)

// MemoryStore is the in-process audit log for tests and local mode.
// Keep semantics in lock-step with MongoStore.
type MemoryStore struct {
	mu     sync.RWMutex
	events []Event
}

func NewMemoryStore() *MemoryStore { return &MemoryStore{} }

func (s *MemoryStore) Insert(_ context.Context, e Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, e)
	return nil
}

func (s *MemoryStore) ListByTenant(_ context.Context, tenantID string, p Page) ([]Event, error) {
	return s.list(func(e Event) bool {
		return e.Scope == ScopeTenant && e.TenantID == tenantID
	}, p), nil
}

func (s *MemoryStore) ListPlatform(_ context.Context, p Page) ([]Event, error) {
	return s.list(func(e Event) bool { return e.Scope == ScopePlatform }, p), nil
}

func (s *MemoryStore) list(match func(Event) bool, p Page) []Event {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Event
	for _, e := range s.events {
		if !match(e) {
			continue
		}
		if p.Action != "" && e.Action != p.Action {
			continue
		}
		if p.ActorID != "" && e.ActorID != p.ActorID {
			continue
		}
		if !p.From.IsZero() && e.CreatedAt.Before(p.From) {
			continue
		}
		if !p.To.IsZero() && e.CreatedAt.After(p.To) {
			continue
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if p.Offset > 0 {
		if p.Offset >= len(out) {
			return nil
		}
		out = out[p.Offset:]
	}
	if lim := ClampLimit(p.Limit); len(out) > lim {
		out = out[:lim]
	}
	return out
}
