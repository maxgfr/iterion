// Package store is the fixture's in-memory data layer.
//
// Concurrency: every public method takes a single mutex covering both
// users and posts. That's coarse but consistent — callers don't need
// to reason about which lock protects which map.
package store

import (
	"errors"
	"sync"
	"time"
)

// User is the minimal record exposed by the API.
type User struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Post is reserved for the feature the bot will add. The type is
// defined here so the bot has a starting shape; the storage methods
// to manipulate posts intentionally do NOT exist yet.
type Post struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// ErrNotFound is returned when a lookup by ID fails.
var ErrNotFound = errors.New("not found")

// ErrAlreadyExists is returned when CreateUser sees a duplicate ID.
var ErrAlreadyExists = errors.New("already exists")

// Store is an in-memory user/post repository safe for concurrent use.
type Store struct {
	mu    sync.Mutex
	users map[string]User
	posts map[string]Post
}

// New returns a Store with empty user and post tables.
func New() *Store {
	return &Store{
		users: make(map[string]User),
		posts: make(map[string]Post),
	}
}

// CreateUser inserts u. Returns ErrAlreadyExists when u.ID is taken.
func (s *Store) CreateUser(u User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[u.ID]; ok {
		return ErrAlreadyExists
	}
	s.users[u.ID] = u
	return nil
}

// GetUser returns the user with id or ErrNotFound.
func (s *Store) GetUser(id string) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[id]
	if !ok {
		return User{}, ErrNotFound
	}
	return u, nil
}

// ListUsers returns every user. Order is undefined — callers that
// need stable order must sort by ID themselves.
func (s *Store) ListUsers() []User {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]User, 0, len(s.users))
	for _, u := range s.users {
		out = append(out, u)
	}
	return out
}
