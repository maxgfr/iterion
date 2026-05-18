import { useState, useEffect, type ReactNode } from "react";

import { Button } from "./Button";
import { EmptyState } from "./EmptyState";

interface Props {
  // Short feature label used in the notice copy.
  // e.g. "the workflow editor", "the Launch form".
  feature: string;
  // What the user can do instead. Default is generic.
  hint?: ReactNode;
  // The fallback to render once the user has chosen "Continue anyway"
  // OR once the viewport is wide enough. Pass the desktop UI here.
  children: ReactNode;
  // Persistence key for the "Continue anyway" opt-out. Stored in
  // localStorage so the choice survives reloads on this device.
  // Omit to keep the opt-out session-only.
  lsKey?: string;
}

// DesktopOnlyNotice gates an authoring-heavy view behind a viewport
// check + a per-feature opt-out. It renders nothing above the `sm`
// breakpoint (640px) — at that width and beyond, the children take
// over. Below `sm` it shows a centered "open on desktop for the best
// experience" message with a Continue-anyway button.
//
// Discipline: this is for views that can't be reflowed cleanly onto a
// phone (canvas drag-drop, multi-panel run console). Read-only flows
// should be fully responsive instead of hiding behind this primitive.
// See studio/docs/design-system.md § Responsive scope.
export function DesktopOnlyNotice({ feature, hint, children, lsKey }: Props) {
  // We can't read window.matchMedia before mount; assume desktop on
  // first paint so the desktop branch hydrates cleanly. The effect
  // below corrects to the real viewport size right after mount.
  const [isNarrow, setIsNarrow] = useState(false);
  const [override, setOverride] = useState<boolean>(() => {
    if (!lsKey || typeof window === "undefined") return false;
    try {
      return window.localStorage.getItem(lsKey) === "1";
    } catch {
      return false;
    }
  });

  useEffect(() => {
    if (typeof window === "undefined") return;
    const mql = window.matchMedia("(max-width: 639px)");
    const update = () => setIsNarrow(mql.matches);
    update();
    mql.addEventListener("change", update);
    return () => mql.removeEventListener("change", update);
  }, []);

  if (!isNarrow || override) {
    return <>{children}</>;
  }

  const onContinue = () => {
    setOverride(true);
    if (lsKey && typeof window !== "undefined") {
      try {
        window.localStorage.setItem(lsKey, "1");
      } catch {
        // Ignore persistence failures — the session-level state still
        // does the right thing in the current tab.
      }
    }
  };

  return (
    <EmptyState
      message={
        <div className="space-y-2 max-w-xs">
          <div className="text-fg-default text-sm font-medium">
            Open {feature} on a larger screen
          </div>
          <div className="text-fg-subtle">
            {hint ??
              "This view is built for desktop interaction (drag, multi-select, side panels). Phone layouts are intentionally read-only."}
          </div>
        </div>
      }
      action={
        <Button size="sm" variant="secondary" onClick={onContinue}>
          Continue anyway
        </Button>
      }
    />
  );
}
