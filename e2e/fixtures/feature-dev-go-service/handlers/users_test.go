package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"example.com/userservice/internal/store"
)

func newSeededDB(t *testing.T) *store.Store {
	t.Helper()
	db := store.New()
	if err := db.CreateUser(store.User{ID: "u1", Name: "Alice", Email: "alice@example.com"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return db
}

func TestUsersList(t *testing.T) {
	db := newSeededDB(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	NewUsersList(db).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got []store.User
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(users) = %d, want 1", len(got))
	}
}

func TestUserByID(t *testing.T) {
	cases := []struct {
		name       string
		path       string
		method     string
		wantStatus int
	}{
		{"existing user", "/users/u1", http.MethodGet, http.StatusOK},
		{"missing user", "/users/u99", http.MethodGet, http.StatusNotFound},
		{"empty id", "/users/", http.MethodGet, http.StatusNotFound},
		{"wrong method", "/users/u1", http.MethodPost, http.StatusMethodNotAllowed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := newSeededDB(t)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(""))
			NewUserByID(db).ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d (body: %s)", rec.Code, tc.wantStatus, rec.Body.String())
			}
		})
	}
}
