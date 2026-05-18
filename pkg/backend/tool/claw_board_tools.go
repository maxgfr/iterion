package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/SocialGouv/iterion/pkg/dispatcher/native"
	"github.com/SocialGouv/iterion/pkg/dispatcher/native/boardops"
)

// BoardConfig configures in-process board access for the claw backend. It
// is intentionally tiny: the store handle is shared with the dispatcher (so
// changes are visible immediately) and the cap list is whatever the node
// was granted. Pass nil store to disable board tool registration.
type BoardConfig struct {
	Store        *native.Store
	Capabilities []string
}

// RegisterClawBoardTools registers the seven board operations as
// in-process claw tools, filtered by the granted capabilities. The
// `mcp__iterion_board__` FQN prefix mirrors the claude_code MCP path so a
// workflow swapping backends doesn't need to rename tool references in
// prompts.
//
// Each operation goes through pkg/dispatcher/native/boardops so the
// validation and event-emission semantics match the stdio + HTTP
// transports byte-for-byte.
//
// Returns nil when cfg or cfg.Store is nil (no-op).
func RegisterClawBoardTools(reg *Registry, cfg *BoardConfig) error {
	if cfg == nil || cfg.Store == nil {
		return nil
	}
	caps := boardops.Capabilities{}
	for _, c := range cfg.Capabilities {
		caps[c] = true
	}
	for _, t := range boardops.ToolsFor(caps) {
		t := t // capture for closure
		exec := func(ctx context.Context, input json.RawMessage) (string, error) {
			raw, err := boardops.Call(cfg.Store, caps, t.Name, input)
			if err != nil {
				return "", err
			}
			return string(raw), nil
		}
		if err := reg.RegisterMCP("iterion_board", t.Name, t.Description, t.InputSchema, exec); err != nil {
			return fmt.Errorf("register iterion_board/%s: %w", t.Name, err)
		}
	}
	return nil
}
