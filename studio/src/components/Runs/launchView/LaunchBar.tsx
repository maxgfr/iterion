// Extracted from LaunchView.tsx to keep that file focused.
// LaunchBar is the bottom row of the launch form: the primary Launch
// button + the SandboxBadge + the CostPreviewChip + the missing-required
// caption + the "Run ID is generated automatically" hint. All state is
// owned by LaunchView; this component is pure presentation.

import { Button } from "@/components/ui/Button";

import CostPreviewChip from "../CostPreviewChip";

import SandboxBadge from "./SandboxBadge";

export interface LaunchBarProps {
  docReady: boolean;
  submitting: boolean;
  missingRequired: boolean;
  missingTitle: string | undefined;
  attemptedLaunch: boolean;
  sandboxMode: string;
  filePath: string;
  currentSource: string | null;
  onSubmit: () => void;
}

export default function LaunchBar({
  docReady,
  submitting,
  missingRequired,
  missingTitle,
  attemptedLaunch,
  sandboxMode,
  filePath,
  currentSource,
  onSubmit,
}: LaunchBarProps) {
  return (
    <div className="mt-6 flex items-center gap-2 flex-wrap">
      <Button
        variant="primary"
        onClick={onSubmit}
        loading={submitting}
        disabled={!docReady}
        title={missingTitle}
      >
        Launch
      </Button>
      <SandboxBadge mode={sandboxMode} />
      <CostPreviewChip filePath={filePath} source={currentSource || undefined} />
      {missingRequired && (
        <span
          className="text-caption text-warning-fg"
          role={attemptedLaunch ? "alert" : "status"}
        >
          {missingTitle}
        </span>
      )}
      <span className="text-caption text-fg-subtle">
        Run ID is generated automatically.
      </span>
    </div>
  );
}
