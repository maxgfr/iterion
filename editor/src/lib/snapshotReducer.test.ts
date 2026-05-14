import { describe, expect, it } from "vitest";

import type { RunEvent } from "@/api/runs";
import { buildExecutionsAt } from "./snapshotReducer";

function nodeStarted(seq: number, node: string, ts = "2026-05-14T12:00:00Z"): RunEvent {
  return { seq, timestamp: ts, type: "node_started", run_id: "r1", branch_id: "main", node_id: node };
}

describe("snapshotReducer terminal run events", () => {
  it("run_cancelled closes every still-running exec", () => {
    const events: RunEvent[] = [
      nodeStarted(1, "a"),
      nodeStarted(2, "b"),
      {
        seq: 3,
        timestamp: "2026-05-14T12:01:00Z",
        type: "run_cancelled",
        run_id: "r1",
        branch_id: "main",
        data: { reason: "ctrl-c" },
      },
    ];
    const out = buildExecutionsAt(events, 999);
    expect(out).toHaveLength(2);
    for (const e of out) {
      expect(e.status).toBe("failed");
      expect(e.finished_at).toBe("2026-05-14T12:01:00Z");
      expect(e.error).toBe("ctrl-c");
    }
  });

  it("run_finished closes every still-running exec as finished", () => {
    const events: RunEvent[] = [
      nodeStarted(1, "a"),
      nodeStarted(2, "b"),
      {
        seq: 3,
        timestamp: "2026-05-14T12:02:00Z",
        type: "run_finished",
        run_id: "r1",
        branch_id: "main",
      },
    ];
    const out = buildExecutionsAt(events, 999);
    expect(out).toHaveLength(2);
    for (const e of out) {
      expect(e.status).toBe("finished");
    }
  });

  it("run_failed closes parallel-sibling running execs too, not just the current one", () => {
    const events: RunEvent[] = [
      nodeStarted(1, "a"),
      nodeStarted(2, "b"),
      {
        seq: 3,
        timestamp: "2026-05-14T12:03:00Z",
        type: "run_failed",
        run_id: "r1",
        branch_id: "main",
        node_id: "a",
        data: { error: "boom" },
      },
    ];
    const out = buildExecutionsAt(events, 999);
    expect(out).toHaveLength(2);
    const byNode = Object.fromEntries(out.map((e) => [e.ir_node_id, e]));
    expect(byNode.a?.status).toBe("failed");
    expect(byNode.a?.error).toBe("boom");
    expect(byNode.b?.status).toBe("failed");
    expect(byNode.b?.error).toBe("boom");
  });

  it("does not touch already-terminal executions", () => {
    const events: RunEvent[] = [
      nodeStarted(1, "a"),
      {
        seq: 2,
        timestamp: "2026-05-14T12:00:30Z",
        type: "node_finished",
        run_id: "r1",
        branch_id: "main",
        node_id: "a",
      },
      nodeStarted(3, "b"),
      {
        seq: 4,
        timestamp: "2026-05-14T12:01:00Z",
        type: "run_cancelled",
        run_id: "r1",
        branch_id: "main",
      },
    ];
    const out = buildExecutionsAt(events, 999);
    const byNode = Object.fromEntries(out.map((e) => [e.ir_node_id, e]));
    expect(byNode.a?.status).toBe("finished");
    expect(byNode.a?.finished_at).toBe("2026-05-14T12:00:30Z");
    expect(byNode.b?.status).toBe("failed");
  });
});
