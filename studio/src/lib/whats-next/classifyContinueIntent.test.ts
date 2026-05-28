import { describe, expect, it } from "vitest";

import { classifyContinueIntent } from "./classifyContinueIntent";

describe("classifyContinueIntent", () => {
  it("detects done from closing phrases", () => {
    for (const t of ["done", "I'm done", "that's all", "c'est fini", "stop"]) {
      const r = classifyContinueIntent(t);
      expect(r.action).toBe("done");
      expect(r.detail).toBe("");
    }
  });

  it("detects dispatch and extracts the filter as detail", () => {
    const r = classifyContinueIntent("dispatch all the feature_dev tickets");
    expect(r.action).toBe("dispatch_more");
    expect(r.detail).toBe("all the feature_dev tickets");
  });

  it("detects add_ticket and strips the verb from detail", () => {
    const r = classifyContinueIntent("add a ticket for the flaky sandbox boot");
    expect(r.action).toBe("add_ticket");
    expect(r.detail).toBe("the flaky sandbox boot");
  });

  it("detects modify_ticket from mutation verbs", () => {
    const r = classifyContinueIntent("close ticket abc12345");
    expect(r.action).toBe("modify_ticket");
    expect(r.detail).toContain("abc12345");
    expect(r.confidence).toBeGreaterThan(0.5);
  });

  it("falls back to modify_ticket with low confidence on ambiguous text", () => {
    const r = classifyContinueIntent("the doc-align thing from earlier");
    expect(r.action).toBe("modify_ticket");
    expect(r.detail).toBe("the doc-align thing from earlier");
    expect(r.confidence).toBeLessThan(0.5);
  });

  it("handles empty input", () => {
    const r = classifyContinueIntent("   ");
    expect(r.detail).toBe("");
    expect(r.confidence).toBe(0);
  });

  it("prioritises done over other verbs when the line opens with it", () => {
    const r = classifyContinueIntent("done, nothing else to add");
    expect(r.action).toBe("done");
  });
});
