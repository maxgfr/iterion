// Extracted from LaunchView.tsx to keep that file focused.
// RunSettingsSection renders the "Run settings" block: per-run backend
// override and rtk compression override. Both selects are presentational —
// LaunchView owns the override state and feeds it to createRun().

import type { BackendDetectReport } from "@/api/backends";

import { Select } from "@/components/ui/Select";

export interface RunSettingsSectionProps {
  backendOverride: string;
  rtkOverride: string;
  backendReport: BackendDetectReport | null;
  onBackendChange: (value: string) => void;
  onRtkChange: (value: string) => void;
}

export default function RunSettingsSection({
  backendOverride,
  rtkOverride,
  backendReport,
  onBackendChange,
  onRtkChange,
}: RunSettingsSectionProps) {
  return (
    <section className="mt-6 border-t border-border-default pt-4 mb-6">
      <h2 className="text-xs font-medium text-fg-muted mb-3">Run settings</h2>
      <div className="space-y-4">
        <div className="grid grid-cols-[160px_1fr] gap-3 items-start">
          <div>
            <div className="text-xs font-medium font-mono">backend</div>
            <div className="text-caption text-fg-subtle">override for this run</div>
          </div>
          <div>
            <Select
              value={backendOverride}
              onChange={(e) => onBackendChange(e.currentTarget.value)}
            >
              <option value="">
                auto{backendReport?.resolved_default
                  ? ` — currently ${backendReport.resolved_default}`
                  : ""}
              </option>
              {(backendReport?.backends ?? []).map((b) => (
                <option
                  key={b.name}
                  value={b.name}
                  disabled={!b.available}
                >
                  {b.name}
                  {b.available
                    ? b.auth !== "none"
                      ? ` (${b.auth})`
                      : ""
                    : " — no credential"}
                </option>
              ))}
            </Select>
            <div className="mt-1 text-caption text-fg-subtle">
              Overrides the workflow&apos;s default. Nodes that pin a specific{" "}
              <code>backend:</code> keep their pin.
            </div>
          </div>
        </div>
        <div className="grid grid-cols-[160px_1fr] gap-3 items-start">
          <div>
            <div className="text-xs font-medium font-mono">rtk</div>
            <div className="text-caption text-fg-subtle">output compression</div>
          </div>
          <div>
            <Select
              value={rtkOverride}
              onChange={(e) => onRtkChange(e.currentTarget.value)}
            >
              <option value="">inherit (workflow / ITERION_RTK)</option>
              <option value="on">on — compress shell output</option>
              <option value="ultra">ultra — densest output</option>
              <option value="off">off — disable for this run</option>
            </Select>
            <div className="mt-1 text-caption text-fg-subtle">
              Rewrites agent shell commands via{" "}
              <a
                href="https://github.com/rtk-ai/rtk"
                target="_blank"
                rel="noreferrer"
                className="underline"
              >
                rtk
              </a>{" "}
              to save 60–90% of command-output tokens. Needs the{" "}
              <code>rtk</code> binary on the host PATH.
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}
