package server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/SocialGouv/iterion/pkg/auth"
	"github.com/SocialGouv/iterion/pkg/identity"
	iterlog "github.com/SocialGouv/iterion/pkg/log"
)

func newOrgTestServer(t *testing.T) *Server {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand: %v", err)
	}
	signer, err := auth.NewJWTSigner(base64.RawStdEncoding.EncodeToString(key), 15*time.Minute)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	svc, err := auth.NewService(auth.Config{
		Store:      identity.NewMemoryStore(),
		Sessions:   auth.NewMemorySessionStore(),
		Signer:     signer,
		SignupMode: auth.SignupOpen,
		RefreshTTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("auth service: %v", err)
	}
	s := New(Config{}, iterlog.New(iterlog.LevelError, nil))
	s.authSvc = svc
	return s
}

func superAdminCtx() context.Context {
	return auth.WithIdentity(context.Background(), auth.Identity{UserID: "admin", IsSuperAdmin: true})
}

func orgReq(ctx context.Context, method, path, body, id string) *http.Request {
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
	}
	r = r.WithContext(ctx)
	if id != "" {
		r.SetPathValue("id", id)
	}
	return r
}

func seedTeam(t *testing.T, s *Server, id, slug string) {
	t.Helper()
	if _, err := s.authStore().CreateTeam(context.Background(), identity.Team{
		ID: id, Name: id, Slug: slug, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed team: %v", err)
	}
}

func TestOrgCanLaunch(t *testing.T) {
	st := identity.NewMemoryStore()
	ctx := context.Background()
	for id, status := range map[string]identity.TeamStatus{
		"active": identity.TeamStatusActive,
		"susp":   identity.TeamStatusSuspended,
		"ro":     identity.TeamStatusReadOnly,
	} {
		if _, err := st.CreateTeam(ctx, identity.Team{ID: id, Name: id, Slug: id, Status: status}); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct {
		name string
		st   identity.Store
		id   auth.Identity
		want bool
	}{
		{"no store (local mode)", nil, auth.Identity{TeamID: "susp"}, true},
		{"super-admin bypass", st, auth.Identity{TeamID: "susp", IsSuperAdmin: true}, true},
		{"no active team", st, auth.Identity{}, true},
		{"active allows", st, auth.Identity{TeamID: "active"}, true},
		{"suspended denies", st, auth.Identity{TeamID: "susp"}, false},
		{"read_only denies", st, auth.Identity{TeamID: "ro"}, false},
		{"missing team fails open", st, auth.Identity{TeamID: "ghost"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := orgCanLaunch(ctx, c.st, c.id); got != c.want {
				t.Fatalf("orgCanLaunch=%v want %v", got, c.want)
			}
		})
	}
}

func TestHandleAdminSetOrgStatus(t *testing.T) {
	s := newOrgTestServer(t)
	ctx := superAdminCtx()
	seedTeam(t, s, "t1", "acme")

	// invalid status → 400
	w := httptest.NewRecorder()
	s.handleAdminSetOrgStatus(w, orgReq(ctx, "POST", "/api/admin/orgs/t1/status", `{"status":"weird"}`, "t1"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid status: code=%d body=%s", w.Code, w.Body.String())
	}

	// suspend → 200, persisted, gate now denies a member
	w = httptest.NewRecorder()
	s.handleAdminSetOrgStatus(w, orgReq(ctx, "POST", "/api/admin/orgs/t1/status", `{"status":"suspended","reason":"abuse"}`, "t1"))
	if w.Code != http.StatusOK {
		t.Fatalf("suspend: code=%d body=%s", w.Code, w.Body.String())
	}
	tm, _ := s.authStore().GetTeam(ctx, "t1")
	if !tm.Suspended() || tm.SuspendReason != "abuse" || tm.SuspendedBy != "admin" {
		t.Fatalf("suspend not persisted: %+v", tm)
	}
	if orgCanLaunch(ctx, s.authStore(), auth.Identity{TeamID: "t1"}) {
		t.Fatal("suspended team should be denied launch")
	}

	// restore → active, suspend metadata cleared
	w = httptest.NewRecorder()
	s.handleAdminSetOrgStatus(w, orgReq(ctx, "POST", "/api/admin/orgs/t1/status", `{"status":"active"}`, "t1"))
	if w.Code != http.StatusOK {
		t.Fatalf("restore: code=%d", w.Code)
	}
	tm, _ = s.authStore().GetTeam(ctx, "t1")
	if tm.Suspended() || tm.SuspendedAt != nil || tm.SuspendReason != "" {
		t.Fatalf("restore did not clear suspend metadata: %+v", tm)
	}
}

func TestHandleAdminUpdateOrg_QuotaValidation(t *testing.T) {
	s := newOrgTestServer(t)
	ctx := superAdminCtx()
	seedTeam(t, s, "t1", "acme")

	// negative quota → 400
	w := httptest.NewRecorder()
	s.handleAdminUpdateOrg(w, orgReq(ctx, "PATCH", "/api/admin/orgs/t1", `{"memory_quota_bytes":-5}`, "t1"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("negative quota: code=%d", w.Code)
	}

	// valid quotas → 200, persisted
	w = httptest.NewRecorder()
	s.handleAdminUpdateOrg(w, orgReq(ctx, "PATCH", "/api/admin/orgs/t1", `{"memory_quota_bytes":1048576,"monthly_run_quota":100}`, "t1"))
	if w.Code != http.StatusOK {
		t.Fatalf("update: code=%d body=%s", w.Code, w.Body.String())
	}
	tm, _ := s.authStore().GetTeam(ctx, "t1")
	if tm.MemoryQuotaBytes != 1048576 || tm.MonthlyRunQuota != 100 {
		t.Fatalf("quotas not persisted: %+v", tm)
	}
}

func TestHandleAdminListOrgs(t *testing.T) {
	s := newOrgTestServer(t)
	ctx := superAdminCtx()
	seedTeam(t, s, "t1", "a")
	seedTeam(t, s, "t2", "b")

	w := httptest.NewRecorder()
	s.handleAdminListOrgs(w, orgReq(ctx, "GET", "/api/admin/orgs", "", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("list: code=%d", w.Code)
	}
	var resp struct {
		Orgs []orgView `json:"orgs"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Orgs) != 2 {
		t.Fatalf("got %d orgs want 2", len(resp.Orgs))
	}
	if resp.Orgs[0].Status != "active" {
		t.Fatalf("empty status should render as active, got %q", resp.Orgs[0].Status)
	}
}
