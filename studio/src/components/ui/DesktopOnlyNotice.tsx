import { useState, useEffect, type ReactNode } from "react";

import { readBooleanFlag, writeBooleanFlag } from "@/lib/localStorageFlag";

import { Button } from "./Button";
import { EmptyState } from "./EmptyState";

interface Props {
  feature: string;
  hint?: ReactNode;
  children: ReactNode;
  // Persistence key for the "Continue anyway" opt-out. Omit to keep
  // the opt-out session-only.
  lsKey?: string;
}

// For authoring surfaces that can't reflow cleanly onto a phone (canvas
// drag-drop, multi-panel run console). Read-only flows should reflow
// inline instead — see studio/docs/design-system.md § Responsive scope.
export function DesktopOnlyNotice({ feature, hint, children, lsKey }: Props) {
  // Assume desktop on first paint so the desktop branch hydrates
  // cleanly; the effect below corrects to the real viewport.
  const [isNarrow, setIsNarrow] = useState(false);
  const [override, setOverride] = useState<boolean>(() =>
    lsKey ? readBooleanFlag(lsKey) : false,
  );

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
    if (lsKey) writeBooleanFlag(lsKey, true);
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
