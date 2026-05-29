import { describe, expect, it } from "vitest";

import {
  mergeActionReady,
  smartDefaultFilesMode,
  type RunStatus,
} from "./runs";

// These two pure helpers encode the FilesPanel's reactive scope default:
// "combined" while a run is in flight, flipping to "branch" exactly when the
// Squash & merge action becomes available (terminal state + storage branch).
// Locking them here keeps the lifecycle contract honest without mounting the
// panel (which needs a query client + Monaco).

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

describe("smartDefaultFilesMode", () => {
  it("defaults to combined in flight, branch once merge-ready", () => {
    expect(smartDefaultFilesMode(false)).toBe("combined");
    expect(smartDefaultFilesMode(true)).toBe("branch");
  });
});
