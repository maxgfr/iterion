package webhooks

import (
	"context"
	"sort"
	"sync"
	"time"
)

// MemoryConfigStore is an in-process ConfigStore for tests and local
// mode. Keep its semantics in lock-step with MongoConfigStore.
type MemoryConfigStore struct {
	mu      sync.RWMutex
	configs map[string]Config
}

func NewMemoryConfigStore() *MemoryConfigStore {
	return &MemoryConfigStore{configs: make(map[string]Config)}
}

func (s *MemoryConfigStore) Create(_ context.Context, c Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.configs[c.ID]; ok {
		return ErrDuplicate
	}
	s.configs[c.ID] = c
	return nil
}

func (s *MemoryConfigStore) Get(_ context.Context, id string) (Config, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.configs[id]
	if !ok {
		return Config{}, ErrNotFound
	}
	return c, nil
}

func (s *MemoryConfigStore) Update(_ context.Context, c Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.configs[c.ID]; !ok {
		return ErrNotFound
	}
	s.configs[c.ID] = c
	return nil
}

func (s *MemoryConfigStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.configs[id]; !ok {
		return ErrNotFound
	}
	delete(s.configs, id)
	return nil
}

func (s *MemoryConfigStore) ListByTenant(_ context.Context, tenantID string) ([]Config, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Config
	for _, c := range s.configs {
		if c.TenantID == tenantID {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (s *MemoryConfigStore) MarkUsed(_ context.Context, id string, t time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.configs[id]
	if !ok {
		return ErrNotFound
	}
	tt := t
	c.LastUsedAt = &tt
	s.configs[id] = c
	return nil
}

// MemoryDeliveryStore is an in-process DeliveryStore.
type MemoryDeliveryStore struct {
	mu     sync.RWMutex
	byID   map[string]Delivery
	byIdem map[string]string // idempotency key -> delivery id
}

func NewMemoryDeliveryStore() *MemoryDeliveryStore {
	return &MemoryDeliveryStore{byID: make(map[string]Delivery), byIdem: make(map[string]string)}
}

func (s *MemoryDeliveryStore) Insert(_ context.Context, d Delivery) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d.IdempotencyKey != "" {
		if _, ok := s.byIdem[d.IdempotencyKey]; ok {
			return ErrDuplicate
		}
		s.byIdem[d.IdempotencyKey] = d.ID
	}
	s.byID[d.ID] = d
	return nil
}

func (s *MemoryDeliveryStore) GetByIdempotencyKey(_ context.Context, key string) (Delivery, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.byIdem[key]
	if !ok {
		return Delivery{}, ErrNotFound
	}
	return s.byID[id], nil
}

func (s *MemoryDeliveryStore) Update(_ context.Context, d Delivery) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byID[d.ID]; !ok {
		return ErrNotFound
	}
	s.byID[d.ID] = d
	return nil
}

func (s *MemoryDeliveryStore) ListByWebhook(_ context.Context, tenantID, webhookID string, limit int) ([]Delivery, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Delivery
	for _, d := range s.byID {
		if d.TenantID == tenantID && d.WebhookID == webhookID {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ReceivedAt.After(out[j].ReceivedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// MemoryCounter is an in-process monthly Counter. Production uses the
// Mongo CAS variant; this one is mutex-serialised.
type MemoryCounter struct {
	mu  sync.Mutex
	org map[string]int // tenant|YYYY-MM -> count
	wh  map[string]int // tenant|webhook|YYYY-MM -> count
}

func NewMemoryCounter() *MemoryCounter {
	return &MemoryCounter{org: make(map[string]int), wh: make(map[string]int)}
}

func monthKey(when time.Time) string { return when.UTC().Format("2006-01") }

func (c *MemoryCounter) Allow(_ context.Context, tenantID, webhookID string, when time.Time, lim Limits) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	m := monthKey(when)
	ok := tenantID + "|" + m
	wk := tenantID + "|" + webhookID + "|" + m
	if lim.PerOrgMonthly > 0 && c.org[ok]+1 > lim.PerOrgMonthly {
		return false, nil
	}
	if lim.PerWebhookMonthly > 0 && c.wh[wk]+1 > lim.PerWebhookMonthly {
		return false, nil
	}
	c.org[ok]++
	c.wh[wk]++
	return true, nil
}

func (c *MemoryCounter) OrgCount(_ context.Context, tenantID string, when time.Time) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.org[tenantID+"|"+monthKey(when)], nil
}
