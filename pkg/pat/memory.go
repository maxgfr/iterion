package pat

import (
	"context"
	"sort"
	"sync"
	"time"
)

// MemoryStore is the in-process PAT store for tests and local mode.
// Keep semantics in lock-step with MongoStore.
type MemoryStore struct {
	mu     sync.RWMutex
	byID   map[string]Token
	byHash map[string]string // token hash -> id
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{byID: make(map[string]Token), byHash: make(map[string]string)}
}

func (s *MemoryStore) Create(_ context.Context, t Token) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byID[t.ID] = t
	s.byHash[t.TokenHash] = t.ID
	return nil
}

func (s *MemoryStore) GetByTokenHash(_ context.Context, hash string) (Token, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.byHash[hash]
	if !ok {
		return Token{}, ErrNotFound
	}
	return s.byID[id], nil
}

func (s *MemoryStore) Get(_ context.Context, id string) (Token, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.byID[id]
	if !ok {
		return Token{}, ErrNotFound
	}
	return t, nil
}

func (s *MemoryStore) ListByUser(_ context.Context, userID string) ([]Token, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Token
	for _, t := range s.byID {
		if t.UserID == userID {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *MemoryStore) Revoke(_ context.Context, id string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.byID[id]
	if !ok {
		return ErrNotFound
	}
	at2 := at
	t.RevokedAt = &at2
	s.byID[id] = t
	return nil
}

func (s *MemoryStore) MarkUsed(_ context.Context, id string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.byID[id]
	if !ok {
		return ErrNotFound
	}
	at2 := at
	t.LastUsedAt = &at2
	s.byID[id] = t
	return nil
}
