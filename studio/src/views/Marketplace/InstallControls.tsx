import { CheckIcon } from "@radix-ui/react-icons";

import { Button } from "@/components/ui/Button";
import type { InstalledState } from "./installState";

interface Props {
  state: InstalledState;
  installing: boolean;
  onInstall: () => void;
  onUpdate: () => void;
  onUninstall: () => void;
}

/** InstallControls renders the tri-state action set shared by the card
 *  and the detail drawer:
 *   - absent    → Install
 *   - installed → "Installed ✓" badge + Uninstall
 *   - update    → Update (accent) + Uninstall
 *  Click handlers stop propagation so the card's open-on-click doesn't
 *  also fire. */
export function InstallControls({
  state,
  installing,
  onInstall,
  onUpdate,
  onUninstall,
}: Props) {
  const stop = (fn: () => void) => (e: React.MouseEvent) => {
    e.stopPropagation();
    fn();
  };

  if (state === "absent") {
    return (
      <Button
        variant="success"
        size="sm"
        onClick={stop(onInstall)}
        disabled={installing}
        loading={installing}
        className="shrink-0"
      >
        {installing ? "Installing…" : "Install"}
      </Button>
    );
  }

  return (
    <div className="flex shrink-0 items-center gap-1.5">
      {state === "update" ? (
        <Button
          variant="primary"
          size="sm"
          onClick={stop(onUpdate)}
          disabled={installing}
          loading={installing}
        >
          {installing ? "Updating…" : "Update"}
        </Button>
      ) : (
        <span className="inline-flex items-center gap-1 rounded bg-surface-1 px-1.5 py-0.5 text-caption text-success-fg">
          <CheckIcon className="h-3 w-3" /> Installed
        </span>
      )}
      <Button
        variant="ghost"
        size="sm"
        onClick={stop(onUninstall)}
        disabled={installing}
      >
        Uninstall
      </Button>
    </div>
  );
}
