import { beforeEach, describe, expect, it } from "vitest";
import { useRunStore } from "./run";
import type { RunSnapshot, RunEvent, RunHeader, ExecutionState } from "@/api/runs";

const baseRun: RunHeader = {
  id: "run_test",
  name: "test",
  workflow: "wf",
  status: "running",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
} as unknown as RunHeader;

function exec(node: string, status: ExecutionState["status"], iter = 0, seq = 0): ExecutionState {
  return {
    execution_id: `exec:main:${node}:${iter}`,
    ir_node_id: node,
    branch_id: "main",
    loop_iteration: iter,
    status,
    current_event_seq: seq,
    first_seq: seq,
    last_seq: seq,
  } as ExecutionState;
}

function snap(executions: ExecutionState[], last_seq: number): RunSnapshot {
  return { run: baseRun, executions, last_seq };
}

function nodeStarted(node: string, seq: number, iter = 0): RunEvent {
  return {
    seq,
    timestamp: "2026-01-01T00:00:01Z",
    type: "node_started",
    run_id: "run_test",
    branch_id: "main",
    node_id: node,
    // Pass iteration explicitly so the reducer doesn't auto-bump it
    // via nextIteration() — the bug fix targets duplicate events for
    // the SAME iter, which is what WS history replay produces. The
    // nextIteration fallback (which legitimately bumps for recovery
    // retries) is a different code path that already creates a new
    // exec id.
    data: { kind: "agent", iteration: iter },
  };
}

function nodeFinished(node: string, seq: number): RunEvent {
  return {
    seq,
    timestamp: "2026-01-01T00:00:02Z",
    type: "node_finished",
    run_id: "run_test",
    branch_id: "main",
    node_id: node,
  };
}

beforeEach(() => {
  // Reset to a known empty state. Cast to never to bypass the
  // partial-shape requirement since we want a full wipe per test.
  useRunStore.setState({
    snapshot: null,
    executionsById: new Map(),
    events: [],
    pendingHumanInput: null,
  } as never);
});

describe("applySnapshot", () => {
  it("populates executions and snapshot on first call", () => {
    const s = snap([exec("detect_stack", "running", 0, 1)], 1);
    useRunStore.getState().applySnapshot(s);
    const st = useRunStore.getState();
    expect(st.snapshot?.last_seq).toBe(1);
    expect(st.executionsById.size).toBe(1);
    const e = Array.from(st.executionsById.values())[0]!;
    expect(e.ir_node_id).toBe("detect_stack");
    expect(e.status).toBe("running");
  });

  // Regression: REST and WS each push a snapshot. If the older one
  // arrives second, it must NOT overwrite the newer state — that was
  // the dominant root cause of "two nodes show as running" (the
  // finished node's transition was clobbered by stale snapshot data).
  it("ignores a stale snapshot whose last_seq is older than the current one", () => {
    const newer = snap([exec("detect_stack", "finished", 0, 3), exec("discover_outdated", "running", 0, 5)], 5);
    useRunStore.getState().applySnapshot(newer);

    const stale = snap([exec("detect_stack", "running", 0, 1)], 1);
    useRunStore.getState().applySnapshot(stale);

    const st = useRunStore.getState();
    expect(st.snapshot?.last_seq).toBe(5);
    expect(st.executionsById.size).toBe(2);
    const detect = Array.from(st.executionsById.values()).find((e) => e.ir_node_id === "detect_stack");
    expect(detect?.status).toBe("finished");
  });

  // Regression: events that arrived between the snapshot's last_seq
  // and the snapshot being applied must be re-applied on top of the
  // snapshot's base. Without this, the second-arriving newer event
  // (e.g. detect_stack node_finished) was dropped — leaving the UI
  // showing both detect_stack AND discover_outdated as running.
  it("re-applies events that are newer than the snapshot's last_seq", () => {
    // Simulate: WS pushes node_started(detect_stack)@1 and
    // node_finished(detect_stack)@2 and node_started(discover_outdated)@3
    // BEFORE the REST snapshot resolves (carrying state at seq=1).
    useRunStore.getState().applyEventsBatch([
      nodeStarted("detect_stack", 1),
      nodeFinished("detect_stack", 2),
      nodeStarted("discover_outdated", 3),
    ]);
    // Pre-condition: store has detect_stack=finished + discover_outdated=running.
    {
      const st = useRunStore.getState();
      const detect = Array.from(st.executionsById.values()).find((e) => e.ir_node_id === "detect_stack");
      const discover = Array.from(st.executionsById.values()).find((e) => e.ir_node_id === "discover_outdated");
      expect(detect?.status).toBe("finished");
      expect(discover?.status).toBe("running");
    }

    // Now an OLDER REST snapshot lands (server saw only seq=1, so
    // detect_stack appears "running" and discover_outdated is absent).
    const restSnapshot = snap([exec("detect_stack", "running", 0, 1)], 1);
    useRunStore.getState().applySnapshot(restSnapshot);

    // The newer events (seq=2, seq=3) must have been re-applied to
    // the snapshot's base. State must reflect the latest known
    // truth, not the stale snapshot.
    const st = useRunStore.getState();
    const detect = Array.from(st.executionsById.values()).find((e) => e.ir_node_id === "detect_stack");
    const discover = Array.from(st.executionsById.values()).find((e) => e.ir_node_id === "discover_outdated");
    expect(detect?.status).toBe(
      "finished",
    );
    expect(discover?.status).toBe(
      "running",
    );
  });
});

describe("applyEventsBatch — monotonic status", () => {
  // Regression for the "finished node continues to show running"
  // glitch. A duplicate node_started for an exec id that already
  // reached node_finished must NOT downgrade the status back to
  // running, and must NOT clear finished_at. The runtime can re-emit
  // node_started in legit cases (WS history replay on reconnect,
  // recovery retry that reuses the same iteration) — the reducer is
  // responsible for treating terminal statuses as immutable in this
  // direction.
  it("does not downgrade finished -> running on duplicate node_started", () => {
    useRunStore.getState().applyEventsBatch([
      nodeStarted("detect_stack", 1),
      nodeFinished("detect_stack", 2),
    ]);
    {
      const st = useRunStore.getState();
      const e = Array.from(st.executionsById.values())[0]!;
      expect(e.status).toBe("finished");
      expect(e.finished_at).toBeDefined();
    }

    // Duplicate node_started arrives (history replay or server
    // re-emission). Higher seq than the prior finished — the
    // dedupe filter at the top of the reducer doesn't drop it.
    useRunStore.getState().applyEventsBatch([nodeStarted("detect_stack", 3)]);

    const st = useRunStore.getState();
    const e = Array.from(st.executionsById.values())[0]!;
    expect(e.status).toBe("finished");
    expect(e.finished_at).toBeDefined();
    expect(e.last_seq).toBe(3); // seq markers still advance
  });

  it("does not downgrade failed -> running on duplicate node_started", () => {
    // Build the failed state through the public API: node_started ->
    // run_failed. run_failed flips the current exec to status=failed.
    useRunStore.getState().applyEventsBatch([
      nodeStarted("validate", 1),
      {
        seq: 2,
        timestamp: "2026-01-01T00:00:02Z",
        type: "run_failed",
        run_id: "run_test",
        branch_id: "main",
        node_id: "validate",
        data: { error: "boom" },
      } as RunEvent,
    ]);
    {
      const st = useRunStore.getState();
      const e = Array.from(st.executionsById.values())[0]!;
      expect(e.status).toBe("failed");
    }

    useRunStore.getState().applyEventsBatch([nodeStarted("validate", 3)]);
    const st = useRunStore.getState();
    const e = Array.from(st.executionsById.values())[0]!;
    expect(e.status).toBe("failed");
  });
});
