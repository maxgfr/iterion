import { describe, expect, it } from "vitest";

import type { RunEvent } from "@/api/runs";
import {
  deriveWatchedIds,
  formatUpdatesAsChatMessage,
  type WatchUpdate,
} from "./useWatchList";

function dispatchEvent(ids: string[], seq = 1): RunEvent {
  return {
    seq,
    timestamp: "2026-05-28T10:00:00Z",
    type: "human_answers_recorded",
    run_id: "run-1",
    node_id: "ask_which_to_process",
    data: { answers: { selected_issue_ids: ids } },
  };
}

describe("deriveWatchedIds", () => {
  it("falls back to event-derived ids for legacy runs (server undefined)", () => {
    expect(deriveWatchedIds(undefined, [dispatchEvent(["native:a", "native:b"])])).toEqual([
      "native:a",
      "native:b",
    ]);
  });

  it("uses the server list as the primary, reload-durable source", () => {
    expect(deriveWatchedIds(["native:x", "native:y"], [])).toEqual([
      "native:x",
      "native:y",
    ]);
  });

  it("unions server + event ids, server first, deduped", () => {
    const got = deriveWatchedIds(
      ["native:x", "native:a"],
      [dispatchEvent(["native:a", "native:b"])],
    );
    expect(got).toEqual(["native:x", "native:a", "native:b"]);
  });

  it("drops empty/whitespace ids", () => {
    expect(deriveWatchedIds(["native:x", ""], [])).toEqual(["native:x"]);
  });

  it("returns empty when neither source has ids", () => {
    expect(deriveWatchedIds(undefined, [])).toEqual([]);
    expect(deriveWatchedIds([], [])).toEqual([]);
  });
});

describe("formatUpdatesAsChatMessage", () => {
  it("returns empty for no updates", () => {
    expect(formatUpdatesAsChatMessage([])).toBe("");
  });

  it("formats a single transition", () => {
    const upd: WatchUpdate = {
      issueId: "native:abc",
      title: "Fix sandbox doctor",
      prevState: "ready",
      newState: "in_progress",
      at: "2026-05-27T17:00:00Z",
    };
    expect(formatUpdatesAsChatMessage([upd])).toBe(
      [
        "Board updates since last check:",
        "- Fix sandbox doctor: ready → in_progress",
      ].join("\n"),
    );
  });

  it("collapses multi-hop chains per issue to first prev + latest next", () => {
    const issueId = "native:abc";
    const updates: WatchUpdate[] = [
      {
        issueId,
        title: "Doc-align fix",
        prevState: "backlog",
        newState: "ready",
        at: "2026-05-27T17:00:00Z",
      },
      {
        issueId,
        title: "Doc-align fix",
        prevState: "ready",
        newState: "in_progress",
        at: "2026-05-27T17:05:00Z",
      },
      {
        issueId,
        title: "Doc-align fix",
        prevState: "in_progress",
        newState: "review",
        at: "2026-05-27T17:30:00Z",
      },
    ];
    expect(formatUpdatesAsChatMessage(updates)).toBe(
      [
        "Board updates since last check:",
        "- Doc-align fix: backlog → review",
      ].join("\n"),
    );
  });

  it("keeps one entry per issue when multiple issues transition", () => {
    const updates: WatchUpdate[] = [
      {
        issueId: "native:a",
        title: "Issue A",
        prevState: "ready",
        newState: "in_progress",
        at: "2026-05-27T17:00:00Z",
      },
      {
        issueId: "native:b",
        title: "Issue B",
        prevState: "in_progress",
        newState: "done",
        at: "2026-05-27T17:05:00Z",
      },
      {
        issueId: "native:a",
        title: "Issue A",
        prevState: "in_progress",
        newState: "review",
        at: "2026-05-27T17:30:00Z",
      },
    ];
    expect(formatUpdatesAsChatMessage(updates)).toBe(
      [
        "Board updates since last check:",
        "- Issue A: ready → review",
        "- Issue B: in_progress → done",
      ].join("\n"),
    );
  });
});
