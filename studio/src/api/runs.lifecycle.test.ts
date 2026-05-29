import { describe, expect, it } from "vitest";

import { mergeActionReady, type RunStatus } from "./runs";

// mergeActionReady gates the "Squash & merge" action (terminal state +
// storage branch). The FilesPanel always defaults to the "combined" (All
// changes) view, so there is no longer a lifecycle-reactive scope default to
// lock down here. Locking this helper keeps the merge-gate contract honest
// without mounting the panel (which needs a query client + Monaco).

describe("mergeActionReady", () => {
  it("is false while the run is still in progress or non-mergeable", () => {
    const notReady: RunStatus[] = [
      "running",
      "paused_waiting_human",
      "paused_operator",
      "failed",
      "failed_resumable",
      "queued",
    ];
    for (const status of notReady) {
      expect(
        mergeActionReady({ status, final_branch: "iterion/run/x" }),
      ).toBe(false);
    }
  });

  it("is false for a terminal run with no storage branch", () => {
    expect(
      mergeActionReady({ status: "finished", final_branch: undefined }),
    ).toBe(false);
    expect(mergeActionReady({ status: "cancelled", final_branch: "" })).toBe(
      false,
    );
  });

  it("is true once terminal with a storage branch (merge button shown)", () => {
    expect(
      mergeActionReady({ status: "finished", final_branch: "iterion/run/x" }),
    ).toBe(true);
    expect(
      mergeActionReady({ status: "cancelled", final_branch: "iterion/run/x" }),
    ).toBe(true);
  });

  it("is false for a missing run", () => {
    expect(mergeActionReady(null)).toBe(false);
    expect(mergeActionReady(undefined)).toBe(false);
  });
});
