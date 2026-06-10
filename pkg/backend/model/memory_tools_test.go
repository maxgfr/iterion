package model

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/SocialGouv/claw-code-go/pkg/api"

	"github.com/SocialGouv/iterion/pkg/backend/delegate"
	"github.com/SocialGouv/iterion/pkg/knowledge"
	"github.com/SocialGouv/iterion/pkg/memory"
)

// memTest sets up an isolated ITERION_HOME and returns the FS store +
// the legacy bot SpaceRef the tools run against, plus a direct Scope
// pointing at the same on-disk location for read/write assertions.
func memTest(t *testing.T, scope string) (*memory.FSStore, knowledge.SpaceRef, *memory.Scope) {
	t.Helper()
	t.Setenv("ITERION_HOME", t.TempDir())
	s, err := memory.OpenScope("/tmp/wn", scope)
	if err != nil {
		t.Fatalf("OpenScope: %v", err)
	}
	return memory.DefaultFSStore(), memory.LegacyBotRef("/tmp/wn", scope), s
}

func TestMemoryWriteTool_RoundtripsToScope(t *testing.T) {
	store, ref, s := memTest(t, "session-continuity")
	tool := memoryWriteTool(store, ref)
	if tool.Name != MemoryWriteToolName {
		t.Fatalf("name: %q", tool.Name)
	}
	in, _ := json.Marshal(map[string]string{"path": "CONTEXT_BRIEF.md", "content": "# foo"})
	out, err := tool.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "checkpoint written") {
		t.Fatalf("result: %q", out)
	}
	data, err := s.Read("CONTEXT_BRIEF.md")
	if err != nil || string(data) != "# foo" {
		t.Fatalf("readback: %q err=%v", string(data), err)
	}
}

func TestMemoryReadTool_ReturnsContent(t *testing.T) {
	store, ref, s := memTest(t, "session-continuity")
	_ = s.Write("brief.md", []byte("hello"))
	tool := memoryReadTool(store, ref)
	in, _ := json.Marshal(map[string]string{"path": "brief.md"})
	out, err := tool.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out != "hello" {
		t.Fatalf("got %q", out)
	}
}

func TestMemoryListTool_EmptyAndPopulated(t *testing.T) {
	store, ref, s := memTest(t, "session-continuity")
	tool := memoryListTool(store, ref)
	out, err := tool.Execute(context.Background(), nil)
	if err != nil || out != "(empty)" {
		t.Fatalf("empty: out=%q err=%v", out, err)
	}
	_ = s.Write("a.md", []byte("x"))
	out, err = tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "a.md") {
		t.Fatalf("populated: %q", out)
	}
}

func TestMemoryPreCompactInjector_PrependsAutoload(t *testing.T) {
	store, ref, s := memTest(t, "session-continuity")
	_ = s.Write("CONTEXT_BRIEF.md", []byte("brief body"))

	inject := memoryPreCompactInjector(context.Background(), store, ref, []string{"CONTEXT_BRIEF.md"})
	original := []api.Message{{Role: "user", Content: []api.ContentBlock{{Type: "text", Text: "orig"}}}}
	got := inject(original)
	if len(got) != 2 {
		t.Fatalf("len=%d", len(got))
	}
	if !got[0].IsInjected || got[0].Role != "user" {
		t.Fatalf("first msg shape: %+v", got[0])
	}
	if !strings.Contains(got[0].Content[0].Text, "brief body") {
		t.Fatalf("missing content: %q", got[0].Content[0].Text)
	}
	if !strings.Contains(got[0].Content[0].Text, "<workspace_memory") {
		t.Fatalf("missing tag wrapper: %q", got[0].Content[0].Text)
	}
}

func TestMemoryPreCompactInjector_MissingScopeNoOp(t *testing.T) {
	store, ref, _ := memTest(t, "session-continuity")
	inject := memoryPreCompactInjector(context.Background(), store, ref, nil) // no autoload, empty scope
	got := inject([]api.Message{{Role: "user"}})
	if got != nil {
		t.Fatalf("expected nil (no-op), got %d msgs", len(got))
	}
}

func TestInstallWorkspaceMemory_WiresEverything(t *testing.T) {
	store, ref, _ := memTest(t, "session-continuity")
	opts := &GenerationOptions{}
	spec := &delegate.MemorySpec{
		Scope:            "session-continuity",
		Autoload:         nil,
		Read:             true,
		Write:            true,
		PreCompactInject: true,
	}
	if err := installWorkspaceMemory(context.Background(), opts, store, ref, spec); err != nil {
		t.Fatalf("install: %v", err)
	}
	names := make(map[string]bool)
	for _, tool := range opts.Tools {
		names[tool.Name] = true
	}
	for _, want := range []string{MemoryReadToolName, MemoryWriteToolName, MemoryListToolName} {
		if !names[want] {
			t.Fatalf("missing tool %q (got %v)", want, names)
		}
	}
	if opts.OnBeforeCompact == nil {
		t.Fatalf("PreCompactInject=true did not install OnBeforeCompact")
	}
}

func TestInstallWorkspaceMemory_EmitsAutoIndex(t *testing.T) {
	store, ref, scope := memTest(t, "whats-next")
	_ = scope.Write("brief.md", []byte("---\ntitle: Brief\ntags: [a, b]\n---\n"))
	_ = scope.Write("decisions/dropped-x.md", []byte("# Dropped X\n"))

	opts := &GenerationOptions{}
	err := installWorkspaceMemory(context.Background(), opts, store, ref, &delegate.MemorySpec{
		Scope: "whats-next",
		Read:  true,
	})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if len(opts.SystemBlocks) != 1 {
		t.Fatalf("expected 1 system block (auto-index, no autoload), got %d", len(opts.SystemBlocks))
	}
	got := opts.SystemBlocks[0].Text
	for _, want := range []string{"workspace_memory_index", "brief.md", "title=\"Brief\"", "tags=\"a,b\"", "decisions/dropped-x.md", "title=\"Dropped X\""} {
		if !strings.Contains(got, want) {
			t.Fatalf("auto-index missing %q\nfull:\n%s", want, got)
		}
	}
	if strings.Contains(got, "workspace_memory scope_root") {
		t.Fatalf("autoload block leaked into index-only run:\n%s", got)
	}
}

func TestInstallWorkspaceMemory_AutoIndexPlusAutoload(t *testing.T) {
	store, ref, scope := memTest(t, "whats-next")
	_ = scope.Write("CONTEXT_BRIEF.md", []byte("---\ntitle: Brief\n---\nFULL CONTENT\n"))
	_ = scope.Write("decisions/dropped-x.md", []byte("# Dropped X\n"))

	opts := &GenerationOptions{}
	err := installWorkspaceMemory(context.Background(), opts, store, ref, &delegate.MemorySpec{
		Scope:    "whats-next",
		Autoload: []string{"CONTEXT_BRIEF.md"},
		Read:     true,
	})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if len(opts.SystemBlocks) != 2 {
		t.Fatalf("expected 2 system blocks (index + autoload), got %d", len(opts.SystemBlocks))
	}
	if !strings.Contains(opts.SystemBlocks[0].Text, "workspace_memory_index") {
		t.Fatalf("first block should be the index, got:\n%s", opts.SystemBlocks[0].Text)
	}
	if !strings.Contains(opts.SystemBlocks[1].Text, "FULL CONTENT") {
		t.Fatalf("second block should carry autoloaded content, got:\n%s", opts.SystemBlocks[1].Text)
	}
}

func TestInstallWorkspaceMemory_ReadOnly(t *testing.T) {
	store, ref, _ := memTest(t, "session-continuity")
	opts := &GenerationOptions{}
	spec := &delegate.MemorySpec{
		Scope: "session-continuity",
		Read:  true,
		Write: false,
	}
	if err := installWorkspaceMemory(context.Background(), opts, store, ref, spec); err != nil {
		t.Fatalf("install: %v", err)
	}
	for _, tool := range opts.Tools {
		if tool.Name == MemoryWriteToolName {
			t.Fatalf("write tool leaked under Read=true, Write=false")
		}
	}
}

func TestInstallWorkspaceMemory_RejectsBadScope(t *testing.T) {
	t.Setenv("ITERION_HOME", t.TempDir())
	opts := &GenerationOptions{}
	ref := memory.LegacyBotRef("/tmp/wn", "../escape")
	if err := installWorkspaceMemory(context.Background(), opts, memory.DefaultFSStore(), ref, &delegate.MemorySpec{Scope: "../escape"}); err == nil {
		t.Fatalf("expected scope rejection")
	}
}
