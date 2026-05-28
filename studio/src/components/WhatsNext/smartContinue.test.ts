import { describe, expect, it } from "vitest";

import {
  previousContinueAction,
  smartContinueDefault,
} from "./WhatsNextView";
import type { WhatsNextMessage } from "@/lib/whats-next/messages";

function answeredContinue(action: string): WhatsNextMessage {
  return {
    kind: "human-question",
    id: `ask_continue:${action}`,
    nodeId: "ask_continue",
    prompt: "What's next?",
    status: "answered",
    outcome: { action, detail: "" },
  } as WhatsNextMessage;
}

describe("previousContinueAction", () => {
  it("returns '' when no prior ask_continue turn exists", () => {
    expect(previousContinueAction([])).toBe("");
  });

  it("reads the action from the most recent answered ask_continue", () => {
    const msgs: WhatsNextMessage[] = [
      answeredContinue("add_ticket"),
      answeredContinue("dispatch_more"),
    ];
    expect(previousContinueAction(msgs)).toBe("dispatch_more");
  });

  it("ignores pending ask_continue turns", () => {
    const pending = {
      kind: "human-question",
      id: "ask_continue:pending",
      nodeId: "ask_continue",
      prompt: "What's next?",
      status: "pending",
    } as WhatsNextMessage;
    const msgs: WhatsNextMessage[] = [answeredContinue("add_ticket"), pending];
    expect(previousContinueAction(msgs)).toBe("add_ticket");
  });
});

describe("smartContinueDefault", () => {
  it("defaults to add_ticket after add_ticket (batch creation)", () => {
    expect(smartContinueDefault("add_ticket")).toBe("add_ticket");
  });

  it("defaults to add_ticket after modify_ticket", () => {
    expect(smartContinueDefault("modify_ticket")).toBe("add_ticket");
  });

  it("never auto-defaults after a dispatch (accidental session-end guard)", () => {
    expect(smartContinueDefault("dispatch_more")).toBeUndefined();
    expect(smartContinueDefault("dispatch_just_created")).toBeUndefined();
  });

  it("returns undefined for done or unknown prior actions", () => {
    expect(smartContinueDefault("done")).toBeUndefined();
    expect(smartContinueDefault("")).toBeUndefined();
  });
});
