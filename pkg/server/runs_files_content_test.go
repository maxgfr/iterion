package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// getContent is a small helper that fetches /files/content for path.
func getContent(t *testing.T, baseURL, runID, path string) (*http.Response, runFileContentResponse) {
	t.Helper()
	u := baseURL + "/api/runs/" + runID + "/files/content?path=" + url.QueryEscape(path)
	resp, err := http.Get(u)
	if err != nil {
		t.Fatalf("GET %s: %v", u, err)
	}
	var out runFileContentResponse
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("decode content: %v", err)
		}
	}
	return resp, out
}

// putContent is a small helper that PUTs to /files/content.
func putContent(t *testing.T, baseURL, runID, path, content string) *http.Response {
	t.Helper()
	body, _ := json.Marshal(saveRunFileRequest{Path: path, Content: content})
	req, err := http.NewRequest(http.MethodPut,
		baseURL+"/api/runs/"+runID+"/files/content", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	return resp
}

func TestRunFileContent_ReadTracked(t *testing.T) {
	srv, hs := newTestServer(t)
	dir := initRepo(t) // a.txt committed with "hello\n"
	seedRunWithWorkDir(t, srv, "read-tracked", dir, true)

	resp, out := getContent(t, hs.URL, "read-tracked", "a.txt")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if !out.Exists || out.Binary {
		t.Fatalf("exists=%v binary=%v, want exists=true binary=false", out.Exists, out.Binary)
	}
	if out.Content != "hello\n" {
		t.Fatalf("content = %q, want %q", out.Content, "hello\n")
	}
}

func TestRunFileContent_ReadUntrackedAndMissing(t *testing.T) {
	srv, hs := newTestServer(t)
	dir := initRepo(t)
	// An untracked .gitignore — the motivating use case (operator wants to
	// edit it even though git doesn't track it / it isn't in the changeset).
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(".tmp-gocache/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	seedRunWithWorkDir(t, srv, "read-untracked", dir, true)

	resp, out := getContent(t, hs.URL, "read-untracked", ".gitignore")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !out.Exists || out.Content != ".tmp-gocache/\n" {
		t.Fatalf("untracked read: status=%d exists=%v content=%q", resp.StatusCode, out.Exists, out.Content)
	}

	// A path that does not exist yet → 200 with exists=false (lets the editor
	// seed a fresh buffer, e.g. creating a brand-new .gitignore).
	resp2, out2 := getContent(t, hs.URL, "read-untracked", "does-not-exist.txt")
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("missing read: status = %d, want 200", resp2.StatusCode)
	}
	if out2.Exists {
		t.Fatalf("missing file should report exists=false")
	}
}

func TestRunFileContent_ReadBinary(t *testing.T) {
	srv, hs := newTestServer(t)
	dir := initRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "img.bin"), []byte{0x00, 0x01, 0x02, 0x00}, 0o644); err != nil {
		t.Fatal(err)
	}
	seedRunWithWorkDir(t, srv, "read-bin", dir, true)

	resp, out := getContent(t, hs.URL, "read-bin", "img.bin")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if !out.Binary || out.Content != "" {
		t.Fatalf("binary=%v content=%q, want binary=true content empty", out.Binary, out.Content)
	}
}

func TestRunFileContent_WriteRoundTrip(t *testing.T) {
	srv, hs := newTestServer(t)
	dir := initRepo(t)
	seedRunWithWorkDir(t, srv, "write-rt", dir, true)

	// Overwrite an existing tracked file.
	resp := putContent(t, hs.URL, "write-rt", "a.txt", "rewritten\n")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT a.txt status = %d, want 200", resp.StatusCode)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "a.txt")); err != nil || string(got) != "rewritten\n" {
		t.Fatalf("a.txt on disk = %q (err=%v), want %q", got, err, "rewritten\n")
	}

	// Create a brand-new .gitignore that did not exist before.
	resp2 := putContent(t, hs.URL, "write-rt", ".gitignore", ".tmp-gocache/\n")
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("PUT .gitignore status = %d, want 200", resp2.StatusCode)
	}
	if got, err := os.ReadFile(filepath.Join(dir, ".gitignore")); err != nil || string(got) != ".tmp-gocache/\n" {
		t.Fatalf(".gitignore on disk = %q (err=%v)", got, err)
	}

	// And the write should be visible through a subsequent read.
	_, out := getContent(t, hs.URL, "write-rt", ".gitignore")
	if out.Content != ".tmp-gocache/\n" {
		t.Fatalf("read-back content = %q", out.Content)
	}
}

// TestRunFileContent_RejectsTraversal is the core security matrix: every
// escape attempt must 400 (read) and leave no file written (write), and the
// guard must hold even against a symlink that resolves outside the worktree.
func TestRunFileContent_RejectsTraversal(t *testing.T) {
	srv, hs := newTestServer(t)
	dir := initRepo(t)
	seedRunWithWorkDir(t, srv, "trav", dir, true)

	// A directory OUTSIDE the worktree holding a secret we must never reach.
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("TOP SECRET\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	bad := []string{
		"../../etc/passwd",
		"/etc/passwd",
		"..",
		"-rf",
		"",
		"a/../../escape",
	}

	for _, p := range bad {
		t.Run("read/"+p, func(t *testing.T) {
			resp, _ := getContent(t, hs.URL, "trav", p)
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("read %q: status = %d, want 400", p, resp.StatusCode)
			}
		})
		t.Run("write/"+p, func(t *testing.T) {
			resp := putContent(t, hs.URL, "trav", p, "PWNED\n")
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("write %q: status = %d, want 400", p, resp.StatusCode)
			}
		})
	}

	// Symlink escape: a link inside the worktree pointing at the outside
	// secret dir. ValidateRelPath passes (the rel path is local) but
	// safePathWithin's symlink resolution must still refuse it.
	if runtime.GOOS != "windows" {
		if err := os.Symlink(outside, filepath.Join(dir, "link")); err != nil {
			t.Fatalf("symlink: %v", err)
		}
		resp, _ := getContent(t, hs.URL, "trav", "link/secret.txt")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("symlink read: status = %d, want 400", resp.StatusCode)
		}
		// And a write through the symlink must not land on the secret.
		wresp := putContent(t, hs.URL, "trav", "link/secret.txt", "PWNED\n")
		defer wresp.Body.Close()
		if wresp.StatusCode != http.StatusBadRequest {
			t.Fatalf("symlink write: status = %d, want 400", wresp.StatusCode)
		}
		if got, _ := os.ReadFile(secret); string(got) != "TOP SECRET\n" {
			t.Fatalf("secret was overwritten through symlink: %q", got)
		}
	}
}

func TestRunFileContent_NoWorkDir(t *testing.T) {
	srv, hs := newTestServer(t)
	seedRunWithWorkDir(t, srv, "no-wd", "", false)

	resp, _ := getContent(t, hs.URL, "no-wd", "a.txt")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("read no-workdir: status = %d, want 409", resp.StatusCode)
	}
	wresp := putContent(t, hs.URL, "no-wd", "a.txt", "x\n")
	defer wresp.Body.Close()
	if wresp.StatusCode != http.StatusConflict {
		t.Fatalf("write no-workdir: status = %d, want 409", wresp.StatusCode)
	}
}

func TestRunFileContent_WorktreeGone(t *testing.T) {
	srv, hs := newTestServer(t)
	// Point at a path that does not exist on disk (worktree gc'd).
	seedRunWithWorkDir(t, srv, "gone", filepath.Join(t.TempDir(), "removed"), true)

	resp, _ := getContent(t, hs.URL, "gone", "a.txt")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("read gone: status = %d, want 409", resp.StatusCode)
	}
	wresp := putContent(t, hs.URL, "gone", "a.txt", "x\n")
	defer wresp.Body.Close()
	if wresp.StatusCode != http.StatusConflict {
		t.Fatalf("write gone: status = %d, want 409", wresp.StatusCode)
	}
}

func TestRunFileContent_RejectsCrossOrigin(t *testing.T) {
	srv, hs := newTestServer(t)
	dir := initRepo(t)
	seedRunWithWorkDir(t, srv, "xo", dir, true)

	body, _ := json.Marshal(saveRunFileRequest{Path: "a.txt", Content: "x\n"})
	req, _ := http.NewRequest(http.MethodPut, hs.URL+"/api/runs/xo/files/content", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://evil.example.com")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin write: status = %d, want 403", resp.StatusCode)
	}
	// The disallowed write must not have touched the file.
	if got, _ := os.ReadFile(filepath.Join(dir, "a.txt")); string(got) != "hello\n" {
		t.Fatalf("a.txt mutated by rejected cross-origin write: %q", got)
	}
}

func TestRunFileContent_RejectsCrossStoreWrite(t *testing.T) {
	srv, hs := newTestServer(t)
	dir := initRepo(t)
	seedRunWithWorkDir(t, srv, "xs", dir, true)

	body, _ := json.Marshal(saveRunFileRequest{Path: "a.txt", Content: "x\n"})
	req, _ := http.NewRequest(http.MethodPut,
		hs.URL+"/api/runs/xs/files/content?store="+url.QueryEscape("/some/other/store"),
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("cross-store write: status = %d, want 409", resp.StatusCode)
	}
}
