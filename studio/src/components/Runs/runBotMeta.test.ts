import { describe, expect, it } from "vitest";

import type { RunSummary, RunStatus } from "@/api/runs";

import { availableBots, botEmoji, botLabel } from "./runBotMeta";

function mkRun(partial: Partial<RunSummary>): RunSummary {
  return {
    id: partial.id ?? "run_x",
    workflow_name: partial.workflow_name ?? "wf",
    bundle_name: partial.bundle_name,
    bundle_display_name: partial.bundle_display_name,
    status: (partial.status ?? "finished") as RunStatus,
    created_at: "2026-05-18T12:00:00Z",
    updated_at: "2026-05-18T12:00:00Z",
    active: false,
  };
}

describe("botLabel", () => {
  it("prefers persona, then bundle_name, then workflow_name", () => {
    expect(
      botLabel(mkRun({ bundle_display_name: "Featurly", bundle_name: "feature-dev" })),
    ).toBe("Featurly");
    expect(botLabel(mkRun({ bundle_name: "feature-dev" }))).toBe("feature-dev");
    expect(botLabel(mkRun({ workflow_name: "plain-wf" }))).toBe("plain-wf");
  });
});

describe("botEmoji", () => {
  it("resolves a known persona glyph", () => {
    // feature-dev is in the persona identity map.
    expect(botEmoji(mkRun({ bundle_name: "feature-dev" }))).not.toBe("🤖");
  });
  it("falls back to the robot glyph for unknown bots", () => {
    expect(botEmoji(mkRun({ bundle_name: "totally-unknown-bot" }))).toBe("🤖");
  });
});

describe("availableBots", () => {
  it("returns distinct bots sorted by label (case-insensitive)", () => {
    const runs = [
      mkRun({ bundle_name: "review-pr", bundle_display_name: "Revi" }),
      mkRun({ bundle_name: "feature-dev", bundle_display_name: "Featurly" }),
      mkRun({ bundle_name: "feature-dev", bundle_display_name: "Featurly" }), // dup key
    ];
    const out = availableBots(runs);
    expect(out.map((b) => b.key)).toEqual(["feature-dev", "review-pr"]);
    expect(out.map((b) => b.label)).toEqual(["Featurly", "Revi"]);
    expect(out.map((b) => b.count)).toEqual([2, 1]); // count folded into one pass
  });

  it("skips runs with no bot key", () => {
    const out = availableBots([mkRun({ workflow_name: "" })]);
    expect(out).toHaveLength(0);
  });
});
