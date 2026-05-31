import { afterEach, describe, expect, it, vi } from "vitest";

import { showRunAlertNotification } from "./desktopNotify";

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("showRunAlertNotification", () => {
  it("no-ops when the Notification API is unavailable", () => {
    vi.stubGlobal("Notification", undefined);
    // Should not throw.
    expect(() =>
      showRunAlertNotification({ title: "x", reason: "y" }),
    ).not.toThrow();
  });

  it("constructs a notification immediately when permission is granted", () => {
    const ctor = vi.fn();
    const NotificationMock = Object.assign(ctor, { permission: "granted" });
    vi.stubGlobal("Notification", NotificationMock);

    showRunAlertNotification({
      title: "Run stalled: demo",
      reason: "no activity for 5m",
      run_id: "run_1",
    });

    expect(ctor).toHaveBeenCalledTimes(1);
    expect(ctor).toHaveBeenCalledWith("Run stalled: demo", {
      body: "no activity for 5m",
      tag: "run_1",
    });
  });

  it("falls back to a default title and empty body when fields are missing", () => {
    const ctor = vi.fn();
    vi.stubGlobal("Notification", Object.assign(ctor, { permission: "granted" }));

    showRunAlertNotification({ run_id: "run_2" });

    expect(ctor).toHaveBeenCalledWith("Run alert", { body: "", tag: "run_2" });
  });

  it("requests permission (and shows on grant) when permission is default", async () => {
    const ctor = vi.fn();
    const requestPermission = vi.fn().mockResolvedValue("granted");
    vi.stubGlobal(
      "Notification",
      Object.assign(ctor, { permission: "default", requestPermission }),
    );

    showRunAlertNotification({ title: "Budget warning: demo", run_id: "run_3" });

    expect(requestPermission).toHaveBeenCalledTimes(1);
    // Let the resolved promise microtask run.
    await Promise.resolve();
    expect(ctor).toHaveBeenCalledTimes(1);
  });

  it("does not show or request when permission is denied", () => {
    const ctor = vi.fn();
    const requestPermission = vi.fn();
    vi.stubGlobal(
      "Notification",
      Object.assign(ctor, { permission: "denied", requestPermission }),
    );

    showRunAlertNotification({ title: "x", run_id: "run_4" });

    expect(ctor).not.toHaveBeenCalled();
    expect(requestPermission).not.toHaveBeenCalled();
  });
});
