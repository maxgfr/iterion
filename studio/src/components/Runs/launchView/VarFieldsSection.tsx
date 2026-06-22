// Extracted from LaunchView.tsx to keep that file focused.
// VarFieldsSection renders the "Inputs" form — one row per declared
// var, with two layouts (prompt-like vars get a vertical label/textarea,
// scalar vars get a 160px label + control grid). The empty-state copy
// (no vars + no attachments) is also rendered here so the section owns
// its full presentation. State (values, submit) is owned by LaunchView.

import type { AttachmentField, VarField } from "@/api/types";

import VarFieldInput from "@/components/shared/VarFieldInput";
import { isPromptLikeVar } from "@/lib/promptVarHeuristics";
import { isVarRequired, RequiredPill } from "@/lib/varValidation";

export interface VarFieldsSectionProps {
  fields: VarField[];
  attachmentFields: AttachmentField[];
  values: Record<string, string>;
  submitting: boolean;
  onValueChange: (name: string, value: string) => void;
  onSubmit: () => void;
}

export default function VarFieldsSection({
  fields,
  attachmentFields,
  values,
  submitting,
  onValueChange,
  onSubmit,
}: VarFieldsSectionProps) {
  if (fields.length === 0) {
    if (attachmentFields.length !== 0) return null;
    return (
      <div className="space-y-1">
        <p className="text-xs text-fg-subtle">
          This workflow declares no input vars. You can launch it as-is.
        </p>
        <p className="text-caption text-fg-subtle">
          The workflow&apos;s prompts will read directly from{" "}
          <code>vars:</code> defaults.
        </p>
      </div>
    );
  }
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        if (!submitting) onSubmit();
      }}
    >
      <h2 className="text-xs font-medium text-fg-muted mb-2">Inputs</h2>
      <div className="space-y-4">
        {fields.map((f) => {
          const promptLike = isPromptLikeVar(f);
          const required = isVarRequired(f);
          const value = values[f.name] ?? "";
          const invalid = required && value.trim().length === 0;
          if (promptLike) {
            return (
              <div key={f.name} className="flex flex-col gap-1.5">
                <label htmlFor={`var-${f.name}`} className="flex items-baseline gap-2">
                  <span className="text-xs font-medium font-mono text-fg-default">{f.name}</span>
                  <span className="text-caption text-fg-subtle">{f.type}</span>
                  {required && <RequiredPill />}
                </label>
                <VarFieldInput
                  field={f}
                  id={`var-${f.name}`}
                  value={value}
                  onChange={(v) => onValueChange(f.name, v)}
                  required={required}
                  invalid={invalid}
                />
              </div>
            );
          }
          return (
            <div key={f.name} className="grid grid-cols-[160px_1fr] gap-3 items-start">
              <label htmlFor={`var-${f.name}`} className="pt-1">
                <div className="flex items-baseline gap-2">
                  <span className="text-xs font-medium font-mono">{f.name}</span>
                  {required && <RequiredPill />}
                </div>
                <div className="text-caption text-fg-subtle">{f.type}</div>
              </label>
              <VarFieldInput
                field={f}
                id={`var-${f.name}`}
                value={value}
                onChange={(v) => onValueChange(f.name, v)}
                required={required}
                invalid={invalid}
              />
            </div>
          );
        })}
      </div>
    </form>
  );
}
