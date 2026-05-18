package main

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/SocialGouv/iterion/pkg/dispatcher/native"
	"github.com/SocialGouv/iterion/pkg/dispatcher/native/boardops"
)

// drive feeds a list of JSON-RPC requests to runMCPBoardServer and returns
// the decoded responses in order.
func drive(t *testing.T, store *native.Store, caps boardops.Capabilities, lines []string) []map[string]any {
	t.Helper()
	in := strings.NewReader(strings.Join(lines, "\n") + "\n")
	out := &bytes.Buffer{}
	if err := runMCPBoardServer(in, out, store, caps); err != nil && err != io.EOF {
		t.Fatalf("runMCPBoardServer: %v", err)
	}
	dec := json.NewDecoder(out)
	var resps []map[string]any
	for {
		var m map[string]any
		if err := dec.Decode(&m); err != nil {
			if err == io.EOF {
				break
			}
			t.Fatalf("decode: %v", err)
		}
		resps = append(resps, m)
	}
	return resps
}

func TestMCPBoard_Initialize(t *testing.T) {
	s, err := native.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	resps := drive(t, s, nil, []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize"}`,
	})
	if len(resps) != 1 {
		t.Fatalf("want 1 response, got %d", len(resps))
	}
	result, _ := resps[0]["result"].(map[string]any)
	srv, _ := result["serverInfo"].(map[string]any)
	if srv["name"] != "iterion-board" {
		t.Fatalf("serverInfo.name=%v", srv["name"])
	}
}

func TestMCPBoard_ToolsList_EmptyCapsReturnsEmpty(t *testing.T) {
	s, _ := native.NewStore(t.TempDir())
	resps := drive(t, s, boardops.NewCapabilities(""), []string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`,
	})
	tools := resps[0]["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 0 {
		t.Fatalf("expected zero tools, got %d", len(tools))
	}
}

func TestMCPBoard_ToolsList_FilteredByCaps(t *testing.T) {
	s, _ := native.NewStore(t.TempDir())
	resps := drive(t, s, boardops.NewCapabilities("board.create,board.read"), []string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`,
	})
	tools := resps[0]["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 3 {
		t.Fatalf("expected 3 tools (create, list, get), got %d", len(tools))
	}
	names := []string{
		tools[0].(map[string]any)["name"].(string),
		tools[1].(map[string]any)["name"].(string),
		tools[2].(map[string]any)["name"].(string),
	}
	if strings.Join(names, ",") != "create_issue,get_issue,list_issues" {
		t.Fatalf("unexpected tool order: %v", names)
	}
}

func TestMCPBoard_ToolsCall_CreateAndTransition(t *testing.T) {
	s, _ := native.NewStore(t.TempDir())
	caps := boardops.NewCapabilities("board.create,board.move,board.read")
	resps := drive(t, s, caps, []string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"create_issue","arguments":{"title":"Refactor X"}}}`,
	})
	if len(resps) != 1 {
		t.Fatalf("want 1 response, got %d", len(resps))
	}
	result := resps[0]["result"].(map[string]any)
	if result["isError"] == true {
		t.Fatalf("unexpected isError=true: %+v", result)
	}
	content := result["content"].([]any)
	text := content[0].(map[string]any)["text"].(string)
	var iss native.Issue
	if err := json.Unmarshal([]byte(text), &iss); err != nil {
		t.Fatalf("decode issue: %v", err)
	}
	if iss.Title != "Refactor X" {
		t.Fatalf("title=%q", iss.Title)
	}

	// Transition the issue.
	call := map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "tools/call",
		"params": map[string]any{
			"name":      "transition_issue",
			"arguments": map[string]any{"id": iss.ID, "to": "ready"},
		},
	}
	raw, _ := json.Marshal(call)
	resps = drive(t, s, caps, []string{string(raw)})
	got, _ := s.Get(iss.ID)
	if got.State != "ready" {
		t.Fatalf("state=%s", got.State)
	}
}

func TestMCPBoard_ToolsCall_CapabilityDenied(t *testing.T) {
	s, _ := native.NewStore(t.TempDir())
	caps := boardops.NewCapabilities("board.read") // no create
	resps := drive(t, s, caps, []string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"create_issue","arguments":{"title":"x"}}}`,
	})
	errObj := resps[0]["error"].(map[string]any)
	if int(errObj["code"].(float64)) != -32601 {
		t.Fatalf("expected -32601, got %v", errObj["code"])
	}
	if !strings.Contains(errObj["message"].(string), "capability denied") {
		t.Fatalf("expected capability denied error, got %v", errObj["message"])
	}
}

func TestMCPBoard_MethodNotFound(t *testing.T) {
	s, _ := native.NewStore(t.TempDir())
	resps := drive(t, s, nil, []string{
		`{"jsonrpc":"2.0","id":1,"method":"random/method"}`,
	})
	if int(resps[0]["error"].(map[string]any)["code"].(float64)) != -32601 {
		t.Fatalf("expected -32601")
	}
}
