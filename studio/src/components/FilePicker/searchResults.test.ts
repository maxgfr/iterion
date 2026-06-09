import { describe, expect, it } from "vitest";
import type { FileEntry } from "@/api/types";
import { buildSearchResults } from "./searchResults";

const file = (name: string, size = 0): FileEntry => ({ name, size });

describe("buildSearchResults", () => {
  it("returns [] for empty / whitespace-only query", () => {
    expect(buildSearchResults("", ["a.bot"], [file("b.bot")], ["c.bot"])).toEqual([]);
    expect(buildSearchResults("   ", ["a.bot"], [file("b.bot")], ["c.bot"])).toEqual([]);
  });

  it("matches across all three sources — the bug scenario", () => {
    // User has Recents active, types a substring that only exists in
    // Examples. Old code would return nothing; new code finds it.
    const results = buildSearchResults(
      "kanban",
      ["plan.bot"],
      [file("review.bot")],
      ["kanban_review.bot"],
    );
    expect(results).toEqual([{ kind: "example", name: "kanban_review.bot" }]);
  });

  it("aggregates matches across recents, files, and examples", () => {
    const results = buildSearchResults(
      "review",
      ["session_review.bot", "plan.bot"],
      [file("review.bot"), file("kanban.bot")],
      ["full_review_example.bot", "minimal.bot"],
    );
    expect(results).toEqual([
      { kind: "recent", path: "session_review.bot" },
      { kind: "file", path: "review.bot" },
      { kind: "example", name: "full_review_example.bot" },
    ]);
  });

  it("dedups: a path in both recents and files appears once as 'recent'", () => {
    const results = buildSearchResults(
      "plan",
      ["plan.bot"],
      [file("plan.bot"), file("planner.bot")],
      [],
    );
    expect(results).toEqual([
      { kind: "recent", path: "plan.bot" },
      { kind: "file", path: "planner.bot" },
    ]);
    // Sanity: "plan.bot" is not also reported as a "file" hit.
    expect(results.filter((r) => r.kind === "file" && r.path === "plan.bot")).toHaveLength(0);
  });

  it("matches case-insensitively", () => {
    const results = buildSearchResults(
      "REVIEW",
      ["Session_Review.bot"],
      [file("REVIEW.bot")],
      ["full_review.bot"],
    );
    expect(results.map((r) => (r.kind === "example" ? r.name : r.path))).toEqual([
      "Session_Review.bot",
      "REVIEW.bot",
      "full_review.bot",
    ]);
  });

  it("preserves source order: recents first, then files, then examples", () => {
    const results = buildSearchResults(
      "x",
      ["x_recent_2.bot", "x_recent_1.bot"], // recents are most-recent-first
      [file("x_file_a.bot"), file("x_file_b.bot")],
      ["x_example_a.bot", "x_example_b.bot"],
    );
    expect(results).toEqual([
      { kind: "recent", path: "x_recent_2.bot" },
      { kind: "recent", path: "x_recent_1.bot" },
      { kind: "file", path: "x_file_a.bot" },
      { kind: "file", path: "x_file_b.bot" },
      { kind: "example", name: "x_example_a.bot" },
      { kind: "example", name: "x_example_b.bot" },
    ]);
  });

  it("returns nothing when nothing matches", () => {
    const results = buildSearchResults(
      "zzz",
      ["plan.bot"],
      [file("review.bot")],
      ["minimal.bot"],
    );
    expect(results).toEqual([]);
  });

  it("supports paths with directory components", () => {
    const results = buildSearchResults(
      "subdir",
      ["subdir/a.bot"],
      [file("other/subdir/b.bot")],
      [],
    );
    expect(results).toEqual([
      { kind: "recent", path: "subdir/a.bot" },
      { kind: "file", path: "other/subdir/b.bot" },
    ]);
  });
});
