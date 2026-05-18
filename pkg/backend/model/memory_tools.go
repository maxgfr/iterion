package model

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/SocialGouv/claw-code-go/pkg/api"

	"github.com/SocialGouv/iterion/pkg/backend/delegate"
	"github.com/SocialGouv/iterion/pkg/memory"
)

// Tool names surfaced to the LLM. Tracked as constants so prompts,
// skills, and validators can refer to them without drifting.
const (
	MemoryReadToolName  = "memory_read"
	MemoryWriteToolName = "memory_write"
	MemoryListToolName  = "memory_list"
)

// installWorkspaceMemory wires the per-node memory index + autoload
// + tools + pre-compact hook into opts.
func installWorkspaceMemory(opts *GenerationOptions, workDir string, spec *delegate.MemorySpec) error {
	scope, err := memory.OpenScope(workDir, spec.Scope)
	if err != nil {
		return err
	}

	index, err := scope.BuildIndex()
	if err != nil {
		return fmt.Errorf("auto-index: %w", err)
	}
	if block := renderIndexBlock(scope.Root(), index); block != "" {
		opts.SystemBlocks = append(opts.SystemBlocks, api.ContentBlock{Type: "text", Text: block})
	}

	entries, err := scope.Autoload(spec.Autoload)
	if err != nil {
		return fmt.Errorf("autoload: %w", err)
	}
	if block := renderAutoloadBlock(scope.Root(), entries); block != "" {
		opts.SystemBlocks = append(opts.SystemBlocks, api.ContentBlock{Type: "text", Text: block})
	}

	// Extend the prompt cache to cover the memory blocks too: without
	// this, the system prompt's existing breakpoint stops at block 0
	// and the (often-stable, often-large) memory blocks re-bill on
	// every tool turn.
	if n := len(opts.SystemBlocks); n > 0 {
		opts.SystemBlocks[n-1].CacheControl = api.EphemeralCacheControl()
	}

	if spec.Read {
		opts.Tools = append(opts.Tools, memoryReadTool(scope), memoryListTool(scope))
	}
	if spec.Write {
		opts.Tools = append(opts.Tools, memoryWriteTool(scope))
	}
	if spec.PreCompactInject {
		opts.OnBeforeCompact = memoryPreCompactInjector(scope, spec.Autoload)
	}
	return nil
}

// renderIndexBlock formats the auto-generated index as a single
// XML-tagged block. Empty index → empty string (no block emitted).
func renderIndexBlock(scopeRoot string, entries []memory.IndexEntry) string {
	if len(entries) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<workspace_memory_index scope_root=%q>\n", scopeRoot)
	for _, e := range entries {
		b.WriteString("  <file")
		fmt.Fprintf(&b, " path=%q", e.Path)
		if e.Title != "" {
			fmt.Fprintf(&b, " title=%q", e.Title)
		}
		if len(e.Tags) > 0 {
			fmt.Fprintf(&b, " tags=%q", strings.Join(e.Tags, ","))
		}
		if e.Description != "" {
			fmt.Fprintf(&b, ">%s</file>\n", e.Description)
			continue
		}
		b.WriteString("/>\n")
	}
	b.WriteString("</workspace_memory_index>")
	return b.String()
}

// renderAutoloadBlock formats the full-content autoload set as a
// single XML-tagged block. Empty set → empty string.
func renderAutoloadBlock(scopeRoot string, entries []memory.AutoloadEntry) string {
	if len(entries) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<workspace_memory scope_root=%q>\n", scopeRoot)
	for _, e := range entries {
		fmt.Fprintf(&b, "<file path=%q>\n%s\n</file>\n", e.Path, string(e.Content))
	}
	b.WriteString("</workspace_memory>")
	return b.String()
}

func memoryReadTool(scope *memory.Scope) GenerationTool {
	return GenerationTool{
		Name:        MemoryReadToolName,
		Description: "Read a Markdown file from this node's workspace memory scope. Paths are relative to the scope root.",
		InputSchema: json.RawMessage(`{
            "type": "object",
            "properties": {"path": {"type": "string"}},
            "required": ["path"]
        }`),
		Execute: func(_ context.Context, input json.RawMessage) (string, error) {
			var args struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal(input, &args); err != nil {
				return "", fmt.Errorf("memory_read: decode args: %w", err)
			}
			data, err := scope.Read(args.Path)
			if err != nil {
				return "", fmt.Errorf("memory_read %q: %w", args.Path, err)
			}
			return string(data), nil
		},
	}
}

func memoryWriteTool(scope *memory.Scope) GenerationTool {
	return GenerationTool{
		Name:        MemoryWriteToolName,
		Description: "Write Markdown content to a file in this node's workspace memory scope. Overwrites on each call. Paths are relative to the scope root.",
		InputSchema: json.RawMessage(`{
            "type": "object",
            "properties": {
                "path": {"type": "string"},
                "content": {"type": "string"}
            },
            "required": ["path", "content"]
        }`),
		Execute: func(_ context.Context, input json.RawMessage) (string, error) {
			var args struct {
				Path    string `json:"path"`
				Content string `json:"content"`
			}
			if err := json.Unmarshal(input, &args); err != nil {
				return "", fmt.Errorf("memory_write: decode args: %w", err)
			}
			if err := scope.Write(args.Path, []byte(args.Content)); err != nil {
				return "", fmt.Errorf("memory_write %q: %w", args.Path, err)
			}
			abs, _ := scope.Resolve(args.Path)
			return fmt.Sprintf("checkpoint written: %s (%d bytes)", abs, len(args.Content)), nil
		},
	}
}

func memoryListTool(scope *memory.Scope) GenerationTool {
	return GenerationTool{
		Name:        MemoryListToolName,
		Description: "List files in a subdirectory of this node's workspace memory scope. Pass an empty path for the scope root.",
		InputSchema: json.RawMessage(`{
            "type": "object",
            "properties": {"path": {"type": "string"}},
            "required": []
        }`),
		Execute: func(_ context.Context, input json.RawMessage) (string, error) {
			var args struct {
				Path string `json:"path"`
			}
			if len(input) > 0 {
				if err := json.Unmarshal(input, &args); err != nil {
					return "", fmt.Errorf("memory_list: decode args: %w", err)
				}
			}
			files, err := scope.List(args.Path)
			if err != nil {
				return "", fmt.Errorf("memory_list %q: %w", args.Path, err)
			}
			if len(files) == 0 {
				return "(empty)", nil
			}
			return strings.Join(files, "\n"), nil
		},
	}
}

// memoryPreCompactInjector re-reads the auto-index + autoload set
// and prepends them as a synthetic user turn so claw's summariser
// folds them into the surviving summary. Returns nil (no-op) when
// the scope is empty.
func memoryPreCompactInjector(scope *memory.Scope, autoload []string) func(messages []api.Message) []api.Message {
	return func(messages []api.Message) []api.Message {
		index, err := scope.BuildIndex()
		if err != nil {
			return nil
		}
		entries, err := scope.Autoload(autoload)
		if err != nil {
			return nil
		}
		indexText := renderIndexBlock(scope.Root(), index)
		autoloadText := renderAutoloadBlock(scope.Root(), entries)
		if indexText == "" && autoloadText == "" {
			return nil
		}
		text := indexText
		if autoloadText != "" {
			if text != "" {
				text += "\n"
			}
			text += autoloadText
		}
		injected := api.Message{
			Role:       "user",
			IsInjected: true,
			Content:    []api.ContentBlock{{Type: "text", Text: text}},
		}
		out := make([]api.Message, 0, len(messages)+1)
		out = append(out, injected)
		out = append(out, messages...)
		return out
	}
}
