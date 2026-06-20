package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestConvertManifest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v3/app-manifests/code123/conversions" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": 42, "slug": "iterion-forge-abc", "client_id": "Iv1.cid", "client_secret": "ghsec",
		})
	}))
	defer srv.Close()

	// srv.URL is a non-github.com host → APIBaseFor appends /api/v3.
	conv, err := ConvertManifest(context.Background(), srv.Client(), srv.URL, "code123")
	if err != nil {
		t.Fatal(err)
	}
	if conv.ClientID != "Iv1.cid" || conv.ClientSecret != "ghsec" || conv.ID != 42 {
		t.Fatalf("conv = %+v", conv)
	}
}

func TestConvertManifest_ExpiredCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}))
	defer srv.Close()
	if _, err := ConvertManifest(context.Background(), srv.Client(), srv.URL, "stale"); err == nil {
		t.Fatal("expected an error for an expired/invalid code")
	}
}

func TestBuildAppManifest(t *testing.T) {
	m := BuildAppManifest("iterion-forge-x", "https://it", "https://it/cb")
	if m.RedirectURL != "https://it/cb" || m.Public {
		t.Fatalf("manifest = %+v", m)
	}
	if m.DefaultPermissions["administration"] != "write" {
		t.Fatalf("missing administration perm: %+v", m.DefaultPermissions)
	}
	// The App-level webhook must be disabled — iterion creates per-repo hooks.
	if active, _ := m.HookAttributes["active"].(bool); active {
		t.Fatal("hook attributes should disable the app-level webhook")
	}
}
