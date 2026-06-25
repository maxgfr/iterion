package model

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/SocialGouv/iterion/pkg/backend/delegate"
	"github.com/SocialGouv/iterion/pkg/backend/permission"
)

// bashTool is a stub Bash tool that records whether it executed.
func bashTool(executed *bool) map[string]*GenerationTool {
	return map[string]*GenerationTool{
		"Bash": {
			Name: "Bash",
			Execute: func(_ context.Context, _ json.RawMessage) (string, error) {
				*executed = true
				return "ran", nil
			},
		},
	}
}

func bashCall(cmd string) []toolUseBlock {
	in, _ := json.Marshal(map[string]any{"command": cmd})
	return []toolUseBlock{{ID: "tu_1", Name: "Bash", PartialJSON: string(in)}}
}

func TestExecuteToolsDirect_GateDenyBlocksExecution(t *testing.T) {
	pol, err := permission.NewPolicy(permission.ModeAsk, []string{"Bash(go test:*)"}, nil, []string{"Bash(rm:*)"})
	if err != nil {
		t.Fatal(err)
	}
	var executed bool
	results, gErr := executeToolsDirect(context.Background(), bashCall("rm -rf /"),
		bashTool(&executed), nil, nil, nil, nil, pol)
	if gErr != nil {
		t.Fatalf("unexpected error: %v", gErr)
	}
	if executed {
		t.Error("denied tool must NOT execute")
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	// The deny result must be an error tool_result so the model adapts.
	raw, _ := json.Marshal(results[0])
	if !strings.Contains(string(raw), "denied") {
		t.Errorf("deny result should mention denial: %s", raw)
	}
}

func TestExecuteToolsDirect_GateAllowExecutes(t *testing.T) {
	pol, _ := permission.NewPolicy(permission.ModeAsk, []string{"Bash(go test:*)"}, nil, nil)
	var executed bool
	_, gErr := executeToolsDirect(context.Background(), bashCall("go test ./..."),
		bashTool(&executed), nil, nil, nil, nil, pol)
	if gErr != nil {
		t.Fatalf("unexpected error: %v", gErr)
	}
	if !executed {
		t.Error("allow-listed tool must execute")
	}
}

func TestExecuteToolsDirect_GateAskSuspends(t *testing.T) {
	// ask mode, no rule matches → Ask → loop aborts with ErrAskUser so the
	// run pauses for human approval. The tool must NOT execute.
	pol, _ := permission.NewPolicy(permission.ModeAsk, nil, nil, nil)
	var executed bool
	_, gErr := executeToolsDirect(context.Background(), bashCall("curl http://x"),
		bashTool(&executed), nil, nil, nil, nil, pol)
	var askErr *delegate.ErrAskUser
	if !errors.As(gErr, &askErr) {
		t.Fatalf("ask decision must return *delegate.ErrAskUser, got %v", gErr)
	}
	if askErr.PendingToolUseID != "tu_1" {
		t.Errorf("pending tool_use id = %q, want tu_1", askErr.PendingToolUseID)
	}
	if executed {
		t.Error("ask-gated tool must NOT execute before approval")
	}
}

func TestExecuteToolsDirect_NilPolicyNoGate(t *testing.T) {
	var executed bool
	_, gErr := executeToolsDirect(context.Background(), bashCall("rm -rf /"),
		bashTool(&executed), nil, nil, nil, nil, nil)
	if gErr != nil {
		t.Fatalf("unexpected error: %v", gErr)
	}
	if !executed {
		t.Error("nil policy = no gate, tool should execute")
	}
}
