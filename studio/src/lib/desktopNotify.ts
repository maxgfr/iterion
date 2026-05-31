// desktopNotify.ts — turns a run-health `run:alert` Wails event into a
// native OS notification. Used only in the desktop app (the listener is
// registered via onDesktopEvent, a no-op in plain browser mode). The
// in-page toast + notification dot are driven separately off the run WS
// `alert` event (see useRunWebSocket), so this adds the out-of-window
// affordance: an alert the operator sees even when the window is hidden.

// RunAlertPayload mirrors alert.Alert.AsEventData() on the Go side
// (pkg/alert/alert.go). Only the fields the notification needs are typed;
// the rest pass through untouched.
export interface RunAlertPayload {
  kind?: string;
  run_id?: string;
  run_name?: string;
  title?: string;
  reason?: string;
  link?: string;
}

// showRunAlertNotification renders a native notification for one alert.
// Safe to call anywhere: it no-ops when the Notification API is absent
// (older WebViews / SSR) or permission was denied, and requests
// permission lazily on first use otherwise. Failures are swallowed —
// the in-page toast remains the guaranteed delivery path.
export function showRunAlertNotification(payload: RunAlertPayload): void {
  if (typeof Notification === "undefined") return;

  const title = payload.title?.trim() || "Run alert";
  const body = typeof payload.reason === "string" ? payload.reason : "";

  const show = () => {
    try {
      // tag = run_id collapses repeat alerts for the same run into one
      // notification slot instead of stacking duplicates.
      const n = new Notification(title, { body, tag: payload.run_id });
      n.onclick = () => {
        try {
          window.focus();
        } catch {
          // focus can throw in restricted contexts — ignore.
        }
      };
    } catch {
      // Constructing a Notification can throw on some platforms when the
      // page isn't allowed to; the toast path still covers the user.
    }
  };

  if (Notification.permission === "granted") {
    show();
  } else if (Notification.permission === "default") {
    void Notification.requestPermission().then((perm) => {
      if (perm === "granted") show();
    });
  }
}
