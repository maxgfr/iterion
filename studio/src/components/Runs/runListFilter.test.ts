import { describe, expect, it } from "vitest";

import type { RunSummary, RunStatus } from "@/api/runs";

import { filterRuns, parseSince, sinceCutoff } from "./runListFilter";

function mkRun(partial: Partial<RunSummary>): RunSummary {
  const created = partial.created_at ?? "2026-05-18T12:00:00Z";
  return {
    id: partial.id ?? "run_x",
    workflow_name: partial.workflow_name ?? "workflow",
    name: partial.name,
    file_path: partial.file_path,
    status: (partial.status ?? "finished") as RunStatus,
    created_at: created,
    updated_at: partial.updated_at ?? created,
    finished_at: partial.finished_at,
    active: partial.active ?? false,
  };
}

describe("parseSince", () => {
  it("accepts known values", () => {
    expect(parseSince("today")).toBe("today");
    expect(parseSince("7d")).toBe("7d");
    expect(parseSince("30d")).toBe("30d");
  });
  it("falls back to 'all' for empty/unknown", () => {
    expect(parseSince(null)).toBe("all");
    expect(parseSince("")).toBe("all");
    expect(parseSince("yesterday")).toBe("all");
  });
});

describe("sinceCutoff", () => {
  const NOW = Date.parse("2026-05-18T15:00:00Z");

  it("returns null for 'all'", () => {
    expect(sinceCutoff("all", NOW)).toBeNull();
  });
  it("'today' anchors on local midnight", () => {
    const cut = sinceCutoff("today", NOW)!;
    const d = new Date(cut);
    expect(d.getHours()).toBe(0);
    expect(d.getMinutes()).toBe(0);
    expect(cut).toBeLessThanOrEqual(NOW);
  });
  it("'7d' is 7×24h ago", () => {
    expect(sinceCutoff("7d", NOW)).toBe(NOW - 7 * 24 * 60 * 60 * 1000);
  });
  it("'30d' is 30×24h ago", () => {
    expect(sinceCutoff("30d", NOW)).toBe(NOW - 30 * 24 * 60 * 60 * 1000);
  });
});

describe("filterRuns", () => {
  const NOW = Date.parse("2026-05-18T15:00:00Z");
  const runs: RunSummary[] = [
    mkRun({
      id: "run_aaa",
      name: "kanban refresh",
      workflow_name: "feature-dev",
      file_path: "examples/feature-dev.bot",
      created_at: "2026-05-18T10:00:00Z", // today
    }),
    mkRun({
      id: "run_bbb",
      workflow_name: "review",
      file_path: "examples/review.bot",
      created_at: "2026-05-15T10:00:00Z", // 3d ago
    }),
    mkRun({
      id: "run_ccc",
      workflow_name: "old-thing",
      file_path: "examples/old.bot",
      created_at: "2026-04-10T10:00:00Z", // 38d ago
    }),
  ];

  it("returns the input untouched when query is empty and since is 'all'", () => {
    expect(filterRuns(runs, { query: "", since: "all", now: NOW })).toEqual(runs);
    expect(filterRuns(runs, { query: "   ", since: "all", now: NOW })).toEqual(runs);
  });

  it("matches the run name (case-insensitive)", () => {
    const out = filterRuns(runs, { query: "Kanban", since: "all", now: NOW });
    expect(out.map((r) => r.id)).toEqual(["run_aaa"]);
  });

  it("matches the workflow_name", () => {
    const out = filterRuns(runs, { query: "review", since: "all", now: NOW });
    expect(out.map((r) => r.id)).toEqual(["run_bbb"]);
  });

  it("matches the file path", () => {
    const out = filterRuns(runs, { query: "old.bot", since: "all", now: NOW });
    expect(out.map((r) => r.id)).toEqual(["run_ccc"]);
  });

  it("matches the run id prefix", () => {
    const out = filterRuns(runs, { query: "ccc", since: "all", now: NOW });
    expect(out.map((r) => r.id)).toEqual(["run_ccc"]);
  });

  it("filters by 'today'", () => {
    const out = filterRuns(runs, { query: "", since: "today", now: NOW });
    expect(out.map((r) => r.id)).toEqual(["run_aaa"]);
  });

  it("filters by '7d'", () => {
    const out = filterRuns(runs, { query: "", since: "7d", now: NOW });
    expect(out.map((r) => r.id).sort()).toEqual(["run_aaa", "run_bbb"]);
  });

  it("filters by '30d'", () => {
    const out = filterRuns(runs, { query: "", since: "30d", now: NOW });
    expect(out.map((r) => r.id).sort()).toEqual(["run_aaa", "run_bbb"]);
  });

  it("combines query AND date filter (intersection)", () => {
    const out = filterRuns(runs, { query: "examples", since: "today", now: NOW });
    expect(out.map((r) => r.id)).toEqual(["run_aaa"]);
  });

  it("drops runs with unparseable created_at when a date filter is active", () => {
    const bad = [...runs, mkRun({ id: "run_bad", created_at: "not-a-date" })];
    const out = filterRuns(bad, { query: "", since: "7d", now: NOW });
    expect(out.map((r) => r.id)).not.toContain("run_bad");
  });
});
