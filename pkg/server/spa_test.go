package server

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestSPAHandler(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html":            &fstest.MapFile{Data: []byte("<html>SHELL</html>")},
		"assets/app.js":         &fstest.MapFile{Data: []byte("console.log('hi')")},
		"favicon.ico":           &fstest.MapFile{Data: []byte("ICO")},
		"runs/static-file.json": &fstest.MapFile{Data: []byte(`{"ok":true}`)},
	}
	sub, err := fs.Sub(fsys, ".")
	if err != nil {
		t.Fatalf("sub: %v", err)
	}
	h := SPAHandler(sub)

	cases := []struct {
		name        string
		method      string
		path        string
		wantStatus  int
		wantBodySub string
	}{
		{"root serves index", "GET", "/", http.StatusOK, "SHELL"},
		{"existing asset served verbatim", "GET", "/assets/app.js", http.StatusOK, "console.log"},
		{"existing favicon served", "GET", "/favicon.ico", http.StatusOK, "ICO"},
		{"existing file under spa-like path served", "GET", "/runs/static-file.json", http.StatusOK, `"ok":true`},
		{"unknown spa route falls back to index", "GET", "/runs/abc-xyz", http.StatusOK, "SHELL"},
		{"deep unknown spa route falls back to index", "GET", "/projects/p1/runs/abc", http.StatusOK, "SHELL"},
		{"HEAD on unknown route returns 200 no body", "HEAD", "/runs/abc-xyz", http.StatusOK, ""},
		// Unrouted /api/* must be an authoritative 404, never the SPA shell —
		// else a JSON client JSON.parses index.html and crashes (the
		// /admin/orgs-in-local-mode bug). Covers GET and non-GET.
		{"unrouted GET /api is 404 not shell", "GET", "/api/admin/orgs", http.StatusNotFound, "no such API endpoint"},
		{"unrouted POST /api is 404 not shell", "POST", "/api/foo", http.StatusNotFound, "no such API endpoint"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status: want %d got %d body=%q", tc.wantStatus, rec.Code, rec.Body.String())
			}
			if tc.wantBodySub != "" && !strings.Contains(rec.Body.String(), tc.wantBodySub) {
				t.Fatalf("body missing %q: got %q", tc.wantBodySub, rec.Body.String())
			}
			if tc.method == "HEAD" && rec.Body.Len() != 0 {
				t.Fatalf("HEAD body should be empty, got %q", rec.Body.String())
			}
		})
	}
}

func TestSPAHandlerNoIndex(t *testing.T) {
	fsys := fstest.MapFS{
		"assets/app.js": &fstest.MapFile{Data: []byte("ok")},
	}
	sub, _ := fs.Sub(fsys, ".")
	h := SPAHandler(sub)
	req := httptest.NewRequest("GET", "/runs/abc", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when index missing, got %d", rec.Code)
	}
}
