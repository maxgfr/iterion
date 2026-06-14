package git

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// withSmallDiffCap shrinks diffPayloadCap for the duration of a test so the
// oversize fixtures stay a few bytes rather than 5 MiB, then restores it.
func withSmallDiffCap(t *testing.T, cap int64) {
	t.Helper()
	orig := diffPayloadCap
	diffPayloadCap = cap
	t.Cleanup(func() { diffPayloadCap = orig })
}

// assertOversized checks the canonical oversized payload state: the flag set
// and both sides blanked (the oversized side was never read).
func assertOversized(t *testing.T, p DiffPayload) {
	t.Helper()
	if !p.Oversized {
		t.Errorf("Oversized: want true, got false (payload=%+v)", p)
	}
	if p.Before != nil || p.After != nil {
		t.Errorf("oversized payload must blank both sides, got Before=%v After=%v", p.Before, p.After)
	}
}

// TestDiffOversizedWorktree covers the After (worktree) side exceeding the
// cap: the file is read via readWorktreeFile, whose size guard must trip
// before os.ReadFile and yield Oversized with both sides blanked.
func TestDiffOversizedWorktree(t *testing.T) {
	dir := gitRepo(t)
	withSmallDiffCap(t, 8)

	// a.txt is committed small (HEAD side under cap); overwrite the worktree
	// copy with content larger than the cap so only the After side is oversized.
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("this is well over eight bytes\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	payload, err := Diff(dir, "a.txt")
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	assertOversized(t, payload)
	if payload.Binary {
		t.Errorf("Binary: want false for an oversized text file, got true")
	}
}

// TestDiffOversizedHead covers the Before (HEAD blob) side exceeding the cap
// via showAt's git cat-file size pre-check.
func TestDiffOversizedHead(t *testing.T) {
	dir := gitRepo(t)

	// Commit a large blob, then delete it from the worktree so only the HEAD
	// side carries content. Set the cap small AFTER committing.
	commit(t, dir, "big.txt", string(bytes.Repeat([]byte("x"), 4096)), "add big")
	if err := os.Remove(filepath.Join(dir, "big.txt")); err != nil {
		t.Fatal(err)
	}
	withSmallDiffCap(t, 16)

	payload, err := Diff(dir, "big.txt")
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	assertOversized(t, payload)
}

// TestDiffUnderCapStillBinary is the AC#4 regression: a small file containing
// a NUL byte must still be detected as binary, with Oversized false.
func TestDiffUnderCapStillBinary(t *testing.T) {
	dir := gitRepo(t)
	withSmallDiffCap(t, 1<<20) // 1 MiB — well above the tiny fixture

	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("ab\x00cd\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	payload, err := Diff(dir, "a.txt")
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if !payload.Binary {
		t.Errorf("Binary: want true for a NUL-containing file under cap, got false")
	}
	if payload.Oversized {
		t.Errorf("Oversized: want false for an under-cap file, got true")
	}
	if payload.Before != nil || payload.After != nil {
		t.Errorf("binary payload must blank both sides, got Before=%v After=%v", payload.Before, payload.After)
	}
}

// TestDiffUnderCapNormal guards the happy path: a normal small text edit
// returns populated Before/After with neither flag set.
func TestDiffUnderCapNormal(t *testing.T) {
	dir := gitRepo(t)
	withSmallDiffCap(t, 1<<20)

	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello world\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	payload, err := Diff(dir, "a.txt")
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if payload.Oversized || payload.Binary {
		t.Errorf("flags: want both false, got Oversized=%v Binary=%v", payload.Oversized, payload.Binary)
	}
	if payload.Before == nil || *payload.Before != "hello\n" {
		t.Errorf("Before: want %q, got %v", "hello\n", payload.Before)
	}
	if payload.After == nil || *payload.After != "hello world\n" {
		t.Errorf("After: want %q, got %v", "hello world\n", payload.After)
	}
}

// TestDiffBetweenOversized covers DiffBetween's git-blob path: a blob larger
// than the cap at one ref trips Oversized via showAt's cat-file pre-check,
// without reading the blob.
func TestDiffBetweenOversized(t *testing.T) {
	dir := gitRepo(t)

	base := resolveSHA(t, dir, "HEAD")
	final := commit(t, dir, "a.txt", string(bytes.Repeat([]byte("y"), 4096)), "grow a")
	withSmallDiffCap(t, 32)

	payload, err := DiffBetween(dir, base, final, "a.txt")
	if err != nil {
		t.Fatalf("DiffBetween: %v", err)
	}
	assertOversized(t, payload)
}

// TestDiffBetweenUnderCap guards DiffBetween's happy path under a small cap.
func TestDiffBetweenUnderCap(t *testing.T) {
	dir := gitRepo(t)
	withSmallDiffCap(t, 1<<20)

	base := resolveSHA(t, dir, "HEAD")
	final := commit(t, dir, "a.txt", "v2\n", "edit a")

	payload, err := DiffBetween(dir, base, final, "a.txt")
	if err != nil {
		t.Fatalf("DiffBetween: %v", err)
	}
	if payload.Oversized || payload.Binary {
		t.Errorf("flags: want both false, got Oversized=%v Binary=%v", payload.Oversized, payload.Binary)
	}
	if payload.Before == nil || *payload.Before != "hello\n" {
		t.Errorf("Before: want %q, got %v", "hello\n", payload.Before)
	}
	if payload.After == nil || *payload.After != "v2\n" {
		t.Errorf("After: want %q, got %v", "v2\n", payload.After)
	}
}
