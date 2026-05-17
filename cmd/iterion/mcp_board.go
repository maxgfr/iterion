package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/SocialGouv/iterion/pkg/cli"
	"github.com/SocialGouv/iterion/pkg/conductor/native"
	"github.com/SocialGouv/iterion/pkg/conductor/native/boardops"
	"github.com/spf13/cobra"
)

// mcpBoardCmd runs the internal MCP stdio server that exposes capability-gated
// access to the native kanban board. claude_code and (in-process) claw bots
// invoke this server when they need to create/move/assign issues from inside a
// running workflow.
//
// Configuration via environment:
//   - ITERION_STORE_DIR : path to the conductor store root. If unset, falls
//     back to ./.iterion/conductor relative to the current working directory.
//   - ITERION_BOARD_CAPS: comma-separated list of granted capabilities.
//     Empty/unset = no tools exposed (tools/list returns []).
var mcpBoardCmd = &cobra.Command{
	Use:    "__mcp-board",
	Short:  "Internal: MCP stdio server exposing capability-gated board operations",
	Hidden: true,
	Args:   cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		store, err := openBoardStoreFromEnv()
		if err != nil {
			return err
		}
		caps := boardops.NewCapabilities(os.Getenv("ITERION_BOARD_CAPS"))
		return runMCPBoardServer(os.Stdin, os.Stdout, store, caps)
	},
}

func init() {
	rootCmd.AddCommand(mcpBoardCmd)
}

// openBoardStoreFromEnv resolves the conductor store root and opens it.
// On a first-time open, native.NewStore creates the board.json + issues
// dir + events.jsonl automatically, so a standalone `iterion run` can hit
// a fresh store without any prior `iterion issue board init`.
func openBoardStoreFromEnv() (*native.Store, error) {
	root := os.Getenv("ITERION_STORE_DIR")
	if root == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("resolve cwd: %w", err)
		}
		root = filepath.Join(cwd, ".iterion", "conductor")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("create store dir: %w", err)
	}
	return native.NewStore(root)
}

func runMCPBoardServer(in io.Reader, out io.Writer, store *native.Store, caps boardops.Capabilities) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	enc := json.NewEncoder(out)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var req mcpRequest
		if err := json.Unmarshal(line, &req); err != nil {
			_ = enc.Encode(mcpResponse{
				JSONRPC: "2.0",
				ID:      nil,
				Error:   &mcpError{Code: -32700, Message: fmt.Sprintf("parse error: %s", err)},
			})
			continue
		}
		if req.ID == nil {
			continue // notification — no response
		}
		resp := dispatchMCPBoard(req, store, caps)
		if err := enc.Encode(resp); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func dispatchMCPBoard(req mcpRequest, store *native.Store, caps boardops.Capabilities) mcpResponse {
	resp := mcpResponse{JSONRPC: "2.0", ID: req.ID}

	switch req.Method {
	case "initialize":
		resp.Result = map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo": map[string]any{
				"name":    "iterion-board",
				"version": cli.Version(),
			},
		}
	case "tools/list":
		tools := boardops.ToolsFor(caps)
		entries := make([]map[string]any, 0, len(tools))
		for _, t := range tools {
			entries = append(entries, map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"inputSchema": t.InputSchema,
			})
		}
		resp.Result = map[string]any{"tools": entries}
	case "tools/call":
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			resp.Error = &mcpError{Code: -32602, Message: fmt.Sprintf("invalid params: %s", err)}
			return resp
		}
		raw, err := boardops.Call(store, caps, params.Name, params.Arguments)
		if err != nil {
			if errors.Is(err, boardops.ErrCapabilityDenied) {
				resp.Error = &mcpError{Code: -32601, Message: err.Error()}
				return resp
			}
			// Other errors (unknown state, validation, etc) are returned as
			// tool errors (isError:true) so the LLM can recover.
			resp.Result = map[string]any{
				"content": []map[string]any{{"type": "text", "text": err.Error()}},
				"isError": true,
			}
			return resp
		}
		resp.Result = map[string]any{
			"content": []map[string]any{{"type": "text", "text": string(raw)}},
			"isError": false,
		}
	default:
		resp.Error = &mcpError{Code: -32601, Message: fmt.Sprintf("method not found: %s", req.Method)}
	}
	return resp
}

