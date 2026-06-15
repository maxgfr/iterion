package e2e

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/SocialGouv/iterion/pkg/dsl/ir"
	"github.com/SocialGouv/iterion/pkg/runtime"
	"github.com/SocialGouv/iterion/pkg/store"
)

// wilToInt coerces an edge-relayed numeric (which template substitution may
// deliver as an int, float64, or stringified number) to an int, defaulting
// to 0 for absent/unparseable values — matching how the real snapshot_chunk
// python seeds its state from an empty/literal STATE_IN.
func wilToInt(v interface{}) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case string:
		s := strings.TrimSpace(x)
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return int(f)
		}
	}
	return 0
}

// stubSnapshotChunk registers a stateful stub for the deterministic
// snapshot_chunk tool node. The e2e executor cannot run that node's real
// embedded-python chunker, so this models the one property the loop's
// convergence math depends on: the cross-pass pass-through of the rotation
// cursor and clean_streak. It echoes the edge-fed incoming_* values back as
// the snapshot's persisted_clean_streak / cursor outputs — exactly as the
// real tool seeds them from .whole_improve_loop.state — and reports a SINGLE
// chunk (num_chunks=1) so the streak threshold collapses to the original
// "two consecutive cross-family approvals" (the pre-chunking semantics these
// scenarios assert on). Without this stub the snapshot outputs are nil and
// streak_check's `persisted_clean_streak + 1` / `cursor + 1` exprs fail.
func stubSnapshotChunk(exec *scenarioExecutor) {
	exec.on("snapshot_chunk", func(in map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{
			"chunk_content":          "// stub chunk source",
			"files":                  "stub.go",
			"chunk_label":            "stub",
			"chunk_index":            0,
			"num_chunks":             1,
			"loop_max":               3, // small fixed bound for the test; real node emits num_chunks+max_passes
			"chunked":                false,
			"file_count":             1,
			"chunk_tokens":           10,
			"total_files":            1,
			"total_tokens":           10,
			"skipped_oversize":       0,
			"persisted_clean_streak": wilToInt(in["incoming_clean_streak"]),
			"cursor":                 wilToInt(in["incoming_cursor"]),
			"_tokens":                1,
		}, nil
	})
}

// stubVerifyGate registers stubs for the deterministic build/test gate
// the loop runs AFTER streak_check fires `stop` and BEFORE committing
// (streak_check -> verify_build -> verify_run -> commit_changes -> done).
// The e2e executor cannot run the real verify nodes (verify_build adapts
// to the repo's tooling; verify_run executes .whole_improve_loop.verify.sh),
// so this models a GREEN gate: verify_run reports passed=true so the run
// routes verify_run -> commit_changes when passed -> done. Without it the
// unstubbed verify_run returns no `passed` field, the `when passed` edge is
// never taken, verify_loop(3) exhausts and the run routes to `fail`. Tests
// that never converge (loop-exhaustion scenarios) never reach this gate and
// don't need it.
func stubVerifyGate(exec *scenarioExecutor) {
	exec.on("verify_run", func(_ map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{
			"passed":   true,
			"log_tail": "",
			"_tokens":  1,
		}, nil
	})
}

// TestWholeImproveLoop_HappyPath simulates the canonical "two
// consecutive cross-family approvals" scenario:
//
//	iter1: claude approves   → streak_check.stop = false (no previous)
//	iter2: gpt approves      → streak_check.stop = true  → done
func TestWholeImproveLoop_HappyPath(t *testing.T) {
	wf := compileFixtureStubSafe(t, "whole-improve-loop/main.bot")
	exec := newScenarioExecutor()
	stubSnapshotChunk(exec)
	stubVerifyGate(exec)

	exec.on("reviewer_claude", func(_ map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{
			"approved":  true,
			"family":    "claude",
			"blockers":  []string{},
			"fix_plan":  "",
			"_tokens":   100,
			"_cost_usd": 0.01,
		}, nil
	})
	exec.on("reviewer_gpt", func(_ map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{
			"approved":  true,
			"family":    "gpt",
			"blockers":  []string{},
			"fix_plan":  "",
			"_tokens":   100,
			"_cost_usd": 0.01,
		}, nil
	})

	s := tmpStore(t)
	eng := runtime.New(wf, s, exec)
	if err := eng.Run(context.Background(), "run-vibe-happy", nil); err != nil {
		t.Fatalf("Run: %v", err)
	}

	run, err := s.LoadRun(context.Background(), "run-vibe-happy")
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run.Status != store.RunStatusFinished {
		t.Fatalf("status = %s, want %s", run.Status, store.RunStatusFinished)
	}
	if exec.callCount("reviewer_claude") != 1 {
		t.Errorf("expected reviewer_claude once, got %d", exec.callCount("reviewer_claude"))
	}
	if exec.callCount("reviewer_gpt") != 1 {
		t.Errorf("expected reviewer_gpt once, got %d", exec.callCount("reviewer_gpt"))
	}
	if exec.wasCalled("fix_claude") {
		t.Errorf("fix_claude should not have been called on happy path")
	}
}

// TestWholeImproveLoop_FixThenApprove simulates a scenario where
// the first reviewer rejects, fix runs, then two cross-family approvals
// trigger the stop:
//
//	iter1: claude rejects → fix_claude
//	iter2: gpt approves   → streak_check.stop = false (previous was a fix)
//	iter3: claude approves → streak_check.stop = true → done
//
// Note: iter1 sets loop.previous_output to claude's rejection. After
// fix_claude runs (no loop edge crossing), iter2 traverses the gpt
// reviewer→streak_check edge which snapshots gpt's verdict. Then iter3
// claude→streak_check sees previous=gpt's approval, current=claude's
// approval, families differ → stop.
func TestWholeImproveLoop_FixThenApprove(t *testing.T) {
	wf := compileFixtureStubSafe(t, "whole-improve-loop/main.bot")
	exec := newScenarioExecutor()
	stubSnapshotChunk(exec)
	stubVerifyGate(exec)

	claudeCalls := 0
	exec.on("reviewer_claude", func(_ map[string]interface{}) (map[string]interface{}, error) {
		claudeCalls++
		approved := claudeCalls > 1 // first call rejects, subsequent approve
		blockers := []string{"missing test"}
		if approved {
			blockers = []string{}
		}
		return map[string]interface{}{
			"approved": approved,
			"family":   "claude",
			"blockers": blockers,
			"fix_plan": "add tests",
			"_tokens":  100,
		}, nil
	})
	exec.on("reviewer_gpt", func(_ map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{
			"approved": true,
			"family":   "gpt",
			"blockers": []string{},
			"fix_plan": "",
			"_tokens":  100,
		}, nil
	})
	exec.on("fix_claude", func(_ map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{
			"applied": true,
			"summary": "added tests",
			"_tokens": 200,
		}, nil
	})

	s := tmpStore(t)
	eng := runtime.New(wf, s, exec)
	if err := eng.Run(context.Background(), "run-vibe-fix", nil); err != nil {
		t.Fatalf("Run: %v", err)
	}

	run, err := s.LoadRun(context.Background(), "run-vibe-fix")
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run.Status != store.RunStatusFinished {
		t.Fatalf("status = %s, want %s", run.Status, store.RunStatusFinished)
	}
	if exec.callCount("fix_claude") != 1 {
		t.Errorf("expected fix_claude once, got %d", exec.callCount("fix_claude"))
	}
	if exec.callCount("reviewer_claude") < 2 {
		t.Errorf("expected reviewer_claude at least twice, got %d", exec.callCount("reviewer_claude"))
	}
}

// TestWholeImproveLoop_LoopExhausted simulates a run where the families
// never assemble a clean streak (claude always approves, gpt always
// rejects with a blocker, so the streak resets every gpt pass). The
// review_loop bound kicks in and execution falls through to the
// `reviewer -> fail` terminal: exhausting the loop WITHOUT streak_check
// ever firing `stop` is a non-convergence, which the bot reports as
// `failed` (not a silent `finished`) so a dispatcher re-runs rather than
// marking the ticket clean. See main.bot "Loop exhaustion fallbacks".
func TestWholeImproveLoop_LoopExhausted(t *testing.T) {
	wf := compileFixtureStubSafe(t, "whole-improve-loop/main.bot")
	exec := newScenarioExecutor()
	stubSnapshotChunk(exec)

	exec.on("reviewer_claude", func(_ map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{
			"approved": true, "family": "claude",
			"blockers": []string{}, "fix_plan": "", "_tokens": 50,
		}, nil
	})
	exec.on("reviewer_gpt", func(_ map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{
			"approved": false, "family": "gpt",
			"blockers": []string{"flaky test"}, "fix_plan": "stabilize", "_tokens": 50,
		}, nil
	})
	exec.on("fix_claude", func(_ map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{
			"applied": true, "summary": "tried", "_tokens": 100,
		}, nil
	})

	s := tmpStore(t)
	eng := runtime.New(wf, s, exec)
	// Reaching the `fail` terminal surfaces as a Run error — that IS the
	// non-convergence signal, not a test failure.
	if err := eng.Run(context.Background(), "run-vibe-exhausted", nil); err == nil {
		t.Fatalf("expected Run to error on review_loop exhaustion (non-convergence), got nil")
	}

	run, err := s.LoadRun(context.Background(), "run-vibe-exhausted")
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run.Status != store.RunStatusFailed {
		t.Fatalf("status = %s, want %s (review_loop exhaustion routes to fail — no silent success)",
			run.Status, store.RunStatusFailed)
	}
}

// TestWholeImproveLoop_EventTrace establishes the event-coherence
// baseline: a happy-path run must persist a complete trace covering
// node lifecycle, edge selection, and round-robin reviewer dispatch.
// This is the regression net for any future refactor of the engine's
// event emission — a missing event type surfaces here first.
func TestWholeImproveLoop_EventTrace(t *testing.T) {
	wf := compileFixtureStubSafe(t, "whole-improve-loop/main.bot")
	exec := newScenarioExecutor()
	stubSnapshotChunk(exec)
	stubVerifyGate(exec)
	exec.on("reviewer_claude", func(_ map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{
			"approved": true, "family": "claude",
			"blockers": []interface{}{}, "fix_plan": "", "_tokens": 100,
		}, nil
	})
	exec.on("reviewer_gpt", func(_ map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{
			"approved": true, "family": "gpt",
			"blockers": []interface{}{}, "fix_plan": "", "_tokens": 100,
		}, nil
	})

	s := tmpStore(t)
	eng := runtime.New(wf, s, exec)
	if err := eng.Run(context.Background(), "run-vibe-events", nil); err != nil {
		t.Fatalf("Run: %v", err)
	}

	events, err := s.LoadEvents(context.Background(), "run-vibe-events")
	if err != nil {
		t.Fatalf("LoadEvents: %v", err)
	}

	if !hasEvent(events, store.EventRunStarted) {
		t.Errorf("missing run_started event")
	}
	if !hasEvent(events, store.EventRunFinished) {
		t.Errorf("missing run_finished event")
	}
	if countEventType(events, store.EventNodeStarted) < 3 {
		t.Errorf("expected ≥3 node_started events (reviewer_claude + reviewer_gpt + streak_check), got %d",
			countEventType(events, store.EventNodeStarted))
	}
	if countEventType(events, store.EventEdgeSelected) < 2 {
		t.Errorf("expected ≥2 edge_selected events (round-robin dispatch creates one per reviewer), got %d",
			countEventType(events, store.EventEdgeSelected))
	}
	finishedIDs := eventNodeIDs(events, store.EventNodeFinished)
	finishedSet := make(map[string]bool, len(finishedIDs))
	for _, id := range finishedIDs {
		finishedSet[id] = true
	}
	for _, want := range []string{"reviewer_claude", "reviewer_gpt"} {
		if !finishedSet[want] {
			t.Errorf("expected node_finished event for %q, got %v", want, finishedIDs)
		}
	}
}

// TestWholeImproveLoop_RecoveryLoopExhausted makes both reviewers reject
// with concrete blockers on every pass, so each iteration routes through a
// fix_X. The bounded loops terminate the cascade (review_loop(15) binds
// before recovery_loop(20), since review_loop is incremented every cycle)
// and execution falls through to a `fail` terminal — a non-convergence,
// reported as `failed`. Asserts the fixer ran many times before the cap.
func TestWholeImproveLoop_RecoveryLoopExhausted(t *testing.T) {
	wf := compileFixtureStubSafe(t, "whole-improve-loop/main.bot")
	exec := newScenarioExecutor()
	stubSnapshotChunk(exec)

	// Both reviewers always reject with concrete blockers so each
	// streak_check routes through fix_X.
	exec.on("reviewer_claude", func(_ map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{
			"approved": false, "family": "claude",
			"blockers": []interface{}{"claude found a blocker"},
			"fix_plan": "fix the claude blocker",
			"_tokens":  50,
		}, nil
	})
	exec.on("reviewer_gpt", func(_ map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{
			"approved": false, "family": "gpt",
			"blockers": []interface{}{"gpt found a blocker"},
			"fix_plan": "fix the gpt blocker",
			"_tokens":  50,
		}, nil
	})
	exec.on("fix_claude", func(_ map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{"applied": true, "summary": "claude fix", "_tokens": 100}, nil
	})
	exec.on("fix_gpt", func(_ map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{"applied": true, "summary": "gpt fix", "_tokens": 100}, nil
	})

	s := tmpStore(t)
	eng := runtime.New(wf, s, exec)
	// Never converges (every pass has a blocker) → loop exhaustion routes
	// to the `fail` terminal, which surfaces as a Run error.
	if err := eng.Run(context.Background(), "run-vibe-recovery-exhausted", nil); err == nil {
		t.Fatalf("expected Run to error on loop exhaustion (non-convergence), got nil")
	}

	run, err := s.LoadRun(context.Background(), "run-vibe-recovery-exhausted")
	if err != nil {
		t.Fatalf("LoadRun: %v", err)
	}
	if run.Status != store.RunStatusFailed {
		t.Fatalf("status = %s, want %s (loop exhaustion routes to fail — no silent success)",
			run.Status, store.RunStatusFailed)
	}
	// Bounded by review_loop(15) on the reviewer→streak_check edge —
	// the review cap kicks in before recovery_loop(20) does, since
	// review_loop is what's incremented every cycle. The exact cap
	// depends on round-robin starting family; assert "fixes ran each pass"
	// rather than a specific number. With num_chunks=1 the dynamic bound is
	// loop_max=3, so ~3 fix cycles run before review_loop exhausts.
	totalFixes := exec.callCount("fix_claude") + exec.callCount("fix_gpt")
	if totalFixes < 2 {
		t.Errorf("expected fixer activity across the bounded loop (≥2), got %d (claude=%d, gpt=%d)",
			totalFixes, exec.callCount("fix_claude"), exec.callCount("fix_gpt"))
	}
}

// TestWholeImproveLoop_SessionInheritStructural is a structural
// assertion on the bot's IR rather than a runtime trace: it confirms
// the fix_* agents are declared with `session: inherit` so the
// runtime can splice them into the same Claude/GPT conversation the
// reviewer was using. Drift on this property silently breaks
// prompt-cache hits and reviewer-context continuity — the live runs
// would still pass but cost more, so we pin it here.
func TestWholeImproveLoop_SessionInheritStructural(t *testing.T) {
	wf := compileFixtureStubSafe(t, "whole-improve-loop/main.bot")

	for _, id := range []string{"fix_claude", "fix_gpt"} {
		node, ok := wf.Nodes[id]
		if !ok {
			t.Fatalf("workflow missing expected node %q", id)
		}
		agent, ok := node.(*ir.AgentNode)
		if !ok {
			t.Fatalf("node %q is not an AgentNode (got %T)", id, node)
		}
		if agent.Session != ir.SessionInherit {
			t.Errorf("node %q session = %s, want %s (drift breaks reviewer→fix prompt-cache continuity)",
				id, agent.Session, ir.SessionInherit)
		}
	}
}

// TestWholeImproveLoop_CursorThreadsAndStreakAccumulates pins the
// crash-safety fix (issue #12, ADR-011 → Corrections 2026-06-02c): the
// rotation cursor is advanced by streak_check (`next_cursor = cursor + 1`)
// and threaded back to snapshot_chunk on the loop-return edge TOGETHER with
// the clean_streak, so the two move as one verdict-coupled pair (the
// property that makes the persisted state crash-safe). It drives a 3-chunk
// snapshot stub (threshold = num_chunks + 1 = 4) with all-clean reviews and
// asserts:
//
//   - snapshot_chunk receives a cursor that advances 0,1,2,3 — one per pass,
//     NOT stuck at 0. If the next_cursor expr or any incoming_cursor edge
//     mapping is dropped, the cursor freezes at 0 and this fails — catching
//     the regression that would silently re-open the false-convergence hole.
//   - the clean_streak accumulates 0,1,2,3 across passes (cross-run base
//     pass-through), and convergence needs a FULL sweep + 1 (4 passes), so a
//     single clean chunk cannot terminate the loop.
func TestWholeImproveLoop_CursorThreadsAndStreakAccumulates(t *testing.T) {
	wf := compileFixtureStubSafe(t, "whole-improve-loop/main.bot")
	exec := newScenarioExecutor()

	var gotCursors, gotStreaks []int
	exec.on("snapshot_chunk", func(in map[string]interface{}) (map[string]interface{}, error) {
		cur := wilToInt(in["incoming_cursor"])
		streak := wilToInt(in["incoming_clean_streak"])
		gotCursors = append(gotCursors, cur)
		gotStreaks = append(gotStreaks, streak)
		return map[string]interface{}{
			"chunk_content": "// stub", "files": "stub.go", "chunk_label": "stub",
			"chunk_index": cur % 3, "num_chunks": 3, "loop_max": 5, "chunked": true,
			"file_count": 1, "chunk_tokens": 10, "total_files": 3, "total_tokens": 30,
			"skipped_oversize": 0, "persisted_clean_streak": streak, "cursor": cur, "_tokens": 1,
		}, nil
	})
	approve := func(fam string) func(map[string]interface{}) (map[string]interface{}, error) {
		return func(_ map[string]interface{}) (map[string]interface{}, error) {
			return map[string]interface{}{
				"approved": true, "family": fam,
				"blockers": []string{}, "fix_plan": "", "_tokens": 10,
			}, nil
		}
	}
	exec.on("reviewer_claude", approve("claude"))
	exec.on("reviewer_gpt", approve("gpt"))
	stubVerifyGate(exec)

	s := tmpStore(t)
	eng := runtime.New(wf, s, exec)
	if err := eng.Run(context.Background(), "run-vibe-cursor", nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	run, err := s.LoadRun(context.Background(), "run-vibe-cursor")
	if err != nil {
		t.Fatalf("LoadRun: %v", err)
	}
	if run.Status != store.RunStatusFinished {
		t.Fatalf("status = %s, want %s", run.Status, store.RunStatusFinished)
	}
	want := []int{0, 1, 2, 3} // 3 chunks → 4 clean passes to converge; snapshot runs once per pass
	if !wilEqualInts(gotCursors, want) {
		t.Errorf("snapshot incoming_cursor sequence = %v, want %v (cursor must advance one per pass; stuck-at-0 means the next_cursor/incoming_cursor wiring regressed → false-convergence risk)",
			gotCursors, want)
	}
	if !wilEqualInts(gotStreaks, want) {
		t.Errorf("snapshot incoming_clean_streak sequence = %v, want %v (streak must accumulate across passes)",
			gotStreaks, want)
	}
}

func wilEqualInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
