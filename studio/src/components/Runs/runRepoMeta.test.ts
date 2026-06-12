import { describe, expect, it } from "vitest";

import type { RunSummary, RunStatus } from "@/api/runs";

import {
  availableRepos,
  repoAxisLabel,
  repoKey,
  repoLabel,
} from "./runRepoMeta";

function mkRun(partial: Partial<RunSummary>): RunSummary {
  return {
    id: partial.id ?? "run_x",
    workflow_name: partial.workflow_name ?? "wf",
    work_dir: partial.work_dir,
    repo_root: partial.repo_root,
    project_path: partial.project_path,
    status: (partial.status ?? "finished") as RunStatus,
    created_at: "2026-05-18T12:00:00Z",
    updated_at: "2026-05-18T12:00:00Z",
    active: false,
  };
}

describe("repoAxisLabel", () => {
  it("is 'Repo' in cloud, 'Folder' locally", () => {
    expect(repoAxisLabel("cloud")).toBe("Repo");
    expect(repoAxisLabel("local")).toBe("Folder");
  });
});

describe("repoKey", () => {
  it("prefers project_path (cloud), then repo_root, then work_dir", () => {
    expect(repoKey(mkRun({ project_path: "acme/widgets" }))).toBe("acme/widgets");
    expect(repoKey(mkRun({ repo_root: "/a/b" }))).toBe("/a/b");
    expect(repoKey(mkRun({ work_dir: "/a/wt" }))).toBe("/a/wt");
    // project_path wins over a folder (a cloud run may carry both).
    expect(
      repoKey(mkRun({ project_path: "acme/x", repo_root: "/a/b" })),
    ).toBe("acme/x");
    // repo_root wins over work_dir.
    expect(repoKey(mkRun({ repo_root: "/a/b", work_dir: "/a/wt" }))).toBe("/a/b");
  });
  it("is empty when the run carries none", () => {
    expect(repoKey(mkRun({}))).toBe("");
  });
});

describe("repoLabel", () => {
  it("a cloud slug shows verbatim", () => {
    expect(repoLabel(mkRun({ project_path: "acme/sub/widgets" }))).toBe(
      "acme/sub/widgets",
    );
  });
  it("a folder shows its basename (POSIX + trailing slash + Windows)", () => {
    expect(repoLabel(mkRun({ repo_root: "/home/jo/iterion" }))).toBe("iterion");
    expect(repoLabel(mkRun({ repo_root: "/home/jo/iterion/" }))).toBe("iterion");
    expect(repoLabel(mkRun({ repo_root: "C:\\dev\\iterion" }))).toBe("iterion");
  });
});

describe("availableRepos", () => {
  it("counts per key, busiest first, then label asc", () => {
    const runs = [
      mkRun({ project_path: "acme/widgets" }),
      mkRun({ project_path: "acme/widgets" }),
      mkRun({ project_path: "acme/gadgets" }),
      mkRun({ project_path: "" }), // skipped
    ];
    const out = availableRepos(runs);
    expect(out.map((c) => [c.key, c.count])).toEqual([
      ["acme/widgets", 2],
      ["acme/gadgets", 1],
    ]);
  });

  it("buckets folders with basename labels", () => {
    const runs = [
      mkRun({ repo_root: "/home/jo/iterion" }),
      mkRun({ repo_root: "/home/jo/iterion" }),
    ];
    const out = availableRepos(runs);
    expect(out).toHaveLength(1);
    expect(out[0]).toMatchObject({
      key: "/home/jo/iterion",
      label: "iterion",
      count: 2,
    });
  });
});
