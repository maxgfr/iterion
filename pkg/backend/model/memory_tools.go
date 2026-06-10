package model

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/SocialGouv/claw-code-go/pkg/api"

	"github.com/SocialGouv/iterion/pkg/backend/delegate"
	"github.com/SocialGouv/iterion/pkg/knowledge"
)

// Tool names surfaced to the LLM. Tracked as constants so prompts,
// skills, and validators can refer to them without drifting. They are
// stable across the FS/cloud MemoryStore backends.
const (
	MemoryReadToolName  = "memory_read"
	MemoryWriteToolName = "memory_write"
	MemoryListToolName  = "memory_list"
)

// installWorkspaceMemory wires the per-node memory index + autoload
// + tools + pre-compact hook into opts, routed through a
// knowledge.MemoryStore + resolved SpaceRef. The store abstracts the
// local-filesystem vs cloud backend; the tool names and prompt blocks
// are identical either way.
func installWorkspaceMemory(ctx context.Context, opts *GenerationOptions, store knowledge.MemoryStore, ref knowledge.SpaceRef, spec *delegate.MemorySpec) error {
	root, err := store.Root(ref)
	if err != nil {
		return err
	}

	index, err := store.BuildIndex(ctx, ref)
	if err != nil {
		return fmt.Errorf("auto-index: %w", err)
	}
	if block := renderIndexBlock(root, index); block != "" {
		opts.SystemBlocks = append(opts.SystemBlocks, api.ContentBlock{Type: "text", Text: block})
	}

	entries, err := store.Autoload(ctx, ref, spec.Autoload)
	if err != nil {
		return fmt.Errorf("autoload: %w", err)
	}
	if block := renderAutoloadBlock(root, entries); block != "" {
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
		opts.Tools = append(opts.Tools, memoryReadTool(store, ref), memoryListTool(store, ref))
	}
	if spec.Write {
		opts.Tools = append(opts.Tools, memoryWriteTool(store, ref))
	}
	if spec.PreCompactInject {
		opts.OnBeforeCompact = memoryPreCompactInjector(ctx, store, ref, spec.Autoload)
	}
	return nil
}

// renderIndexBlock formats the auto-generated index as a single
// XML-tagged block. Empty index → empty string (no block emitted).
func renderIndexBlock(scopeRoot string, entries []knowledge.IndexEntry) string {
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
func renderAutoloadBlock(scopeRoot string, entries []knowledge.AutoloadEntry) string {
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

func memoryReadTool(store knowledge.MemoryStore, ref knowledge.SpaceRef) GenerationTool {
	return GenerationTool{
		Name:        MemoryReadToolName,
		Description: "Read a Markdown file from this node's workspace memory scope. Paths are relative to the scope root.",
		InputSchema: json.RawMessage(`{
            "type": "object",
            "properties": {"path": {"type": "string"}},
            "required": ["path"]
        }`),
		Execute: func(ctx context.Context, input json.RawMessage) (string, error) {
			var args struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal(input, &args); err != nil {
				return "", fmt.Errorf("memory_read: decode args: %w", err)
			}
			doc, err := store.ReadDocument(ctx, ref, args.Path)
			if err != nil {
				return "", fmt.Errorf("memory_read %q: %w", args.Path, err)
			}
			return string(doc.Content), nil
		},
	}
}

func memoryWriteTool(store knowledge.MemoryStore, ref knowledge.SpaceRef) GenerationTool {
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
		Execute: func(ctx context.Context, input json.RawMessage) (string, error) {
			var args struct {
				Path    string `json:"path"`
				Content string `json:"content"`
			}
			if err := json.Unmarshal(input, &args); err != nil {
				return "", fmt.Errorf("memory_write: decode args: %w", err)
			}
			meta, err := store.WriteDocument(ctx, ref, knowledge.DocumentInput{Path: args.Path, Content: []byte(args.Content)})
			if err != nil {
				return "", fmt.Errorf("memory_write %q: %w", args.Path, err)
			}
			loc := meta.Path
			if root, rerr := store.Root(ref); rerr == nil && root != "" {
				loc = strings.TrimRight(root, "/") + "/" + meta.Path
			}
			return fmt.Sprintf("checkpoint written: %s (%d bytes)", loc, meta.Size), nil
		},
	}
}

func memoryListTool(store knowledge.MemoryStore, ref knowledge.SpaceRef) GenerationTool {
	return GenerationTool{
		Name:        MemoryListToolName,
		Description: "List files in a subdirectory of this node's workspace memory scope. Pass an empty path for the scope root.",
		InputSchema: json.RawMessage(`{
            "type": "object",
            "properties": {"path": {"type": "string"}},
            "required": []
        }`),
		Execute: func(ctx context.Context, input json.RawMessage) (string, error) {
			var args struct {
				Path string `json:"path"`
			}
			if len(input) > 0 {
				if err := json.Unmarshal(input, &args); err != nil {
					return "", fmt.Errorf("memory_list: decode args: %w", err)
				}
			}
			docs, err := store.ListDocuments(ctx, ref, args.Path)
			if err != nil {
				return "", fmt.Errorf("memory_list %q: %w", args.Path, err)
			}
			if len(docs) == 0 {
				return "(empty)", nil
			}
			paths := make([]string, len(docs))
			for i, d := range docs {
				paths[i] = d.Path
			}
			return strings.Join(paths, "\n"), nil
		},
	}
}

// memoryPreCompactInjector re-reads the auto-index + autoload set
// and prepends them as a synthetic user turn so claw's summariser
// folds them into the surviving summary. Returns nil (no-op) when
// the scope is empty.
func memoryPreCompactInjector(ctx context.Context, store knowledge.MemoryStore, ref knowledge.SpaceRef, autoload []string) func(messages []api.Message) []api.Message {
	return func(messages []api.Message) []api.Message {
		index, err := store.BuildIndex(ctx, ref)
		if err != nil {
			return nil
		}
		entries, err := store.Autoload(ctx, ref, autoload)
		if err != nil {
			return nil
		}
		root, _ := store.Root(ref)
		indexText := renderIndexBlock(root, index)
		autoloadText := renderAutoloadBlock(root, entries)
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
