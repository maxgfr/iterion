package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SocialGouv/iterion/pkg/knowledge"
	iterlog "github.com/SocialGouv/iterion/pkg/log"
)

func TestMemoryRoutes_RoundTrip(t *testing.T) {
	t.Setenv("ITERION_HOME", t.TempDir())
	s := New(Config{SkipProjectRegistration: true}, iterlog.New(iterlog.LevelError, nil))
	ctx := context.Background()
	const space = "visibility=org&name=conv"

	// write
	w := httptest.NewRecorder()
	s.handleMemoryWriteDoc(w, httptest.NewRequest("PUT", "/api/memory/doc?"+space+"&path=a.md", strings.NewReader("# A")).WithContext(ctx))
	if w.Code != http.StatusOK {
		t.Fatalf("write: %d %s", w.Code, w.Body.String())
	}

	// read
	w = httptest.NewRecorder()
	s.handleMemoryReadDoc(w, httptest.NewRequest("GET", "/api/memory/doc?"+space+"&path=a.md", nil).WithContext(ctx))
	if w.Code != http.StatusOK || w.Body.String() != "# A" {
		t.Fatalf("read: %d %q", w.Code, w.Body.String())
	}

	// usage
	w = httptest.NewRecorder()
	s.handleMemoryUsage(w, httptest.NewRequest("GET", "/api/memory/usage?"+space, nil).WithContext(ctx))
	var u map[string]int64
	json.Unmarshal(w.Body.Bytes(), &u)
	if u["used_bytes"] != 3 {
		t.Fatalf("usage: %v", u)
	}

	// list
	w = httptest.NewRecorder()
	s.handleMemoryListDocs(w, httptest.NewRequest("GET", "/api/memory/docs?"+space, nil).WithContext(ctx))
	var lr struct {
		Documents []knowledge.DocumentMeta `json:"documents"`
	}
	json.Unmarshal(w.Body.Bytes(), &lr)
	if len(lr.Documents) != 1 || lr.Documents[0].Path != "a.md" {
		t.Fatalf("list: %+v", lr.Documents)
	}

	// export → non-empty gzip
	w = httptest.NewRecorder()
	s.handleMemoryExport(w, httptest.NewRequest("GET", "/api/memory/export?"+space, nil).WithContext(ctx))
	if w.Code != http.StatusOK || w.Body.Len() == 0 {
		t.Fatalf("export: %d len=%d", w.Code, w.Body.Len())
	}

	// missing name → 400
	w = httptest.NewRecorder()
	s.handleMemoryUsage(w, httptest.NewRequest("GET", "/api/memory/usage?visibility=org", nil).WithContext(ctx))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("missing name should 400, got %d", w.Code)
	}
}
