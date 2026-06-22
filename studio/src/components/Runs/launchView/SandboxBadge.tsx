// Extracted from LaunchView.tsx to keep that file focused.
// SandboxBadge surfaces the workflow's sandbox isolation level next to
// the Launch button so the operator never confirms a host-execution run
// by accident. Three states match pkg/dsl/ir/sandbox.go SandboxSpec:
//   auto / inline → green "sandboxed" pill
//   none          → red "host execution" pill
//   (no block)    → red "no sandbox" pill, same risk as `none`
// The badge title carries the long-form description so the chip itself
// stays compact in the Launch row.

import { CheckCircledIcon, ExclamationTriangleIcon } from "@radix-ui/react-icons";

export default function SandboxBadge({ mode }: { mode: string }) {
  const active = mode === "auto" || mode === "inline";
  const label = active
    ? `Sandbox: ${mode}`
    : mode === "none"
    ? "Sandbox: none"
    : "No sandbox";
  const cls = active
    ? "bg-success-soft text-success-fg border-success/40"
    : "bg-danger-soft text-danger-fg border-danger/40";
  const title = active
    ? "Workflow declares a sandbox block — tools run inside the container."
    : "Tools and shell commands will run on this host. Add `sandbox: auto` to the workflow file to opt into container isolation.";
  return (
    <span
      className={`inline-flex items-center gap-1 text-caption px-1.5 py-0.5 rounded border ${cls}`}
      title={title}
    >
      {active ? (
        <CheckCircledIcon className="w-3 h-3" aria-hidden="true" />
      ) : (
        <ExclamationTriangleIcon className="w-3 h-3" aria-hidden="true" />
      )}
      {label}
    </span>
  );
}
