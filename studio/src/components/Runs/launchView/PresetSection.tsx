// Extracted from LaunchView.tsx to keep that file focused.
// PresetSection renders the named-preset picker + the selected preset's
// description/prompt/skills card. The applyPreset callback owns the
// overlay logic in LaunchView; this component is purely presentational.

import type { Preset } from "@/api/types";

import { Select } from "@/components/ui/Select";

export interface PresetSectionProps {
  presets: Preset[];
  selectedPreset: string;
  selectedPresetMeta: Preset | undefined;
  filePath: string;
  submitting: boolean;
  onApply: (name: string) => void;
  onEditInEditor: () => void;
}

export default function PresetSection({
  presets,
  selectedPreset,
  selectedPresetMeta,
  filePath,
  submitting,
  onApply,
  onEditInEditor,
}: PresetSectionProps) {
  if (presets.length === 0) return null;
  return (
    <section className="mb-6">
      <div className="flex items-center justify-between mb-2">
        <h2 className="text-xs font-medium text-fg-muted">Preset</h2>
        {filePath && (
          <button
            type="button"
            onClick={onEditInEditor}
            className="text-caption text-fg-subtle hover:text-fg-default underline"
            title="Edit presets in the workflow editor"
          >
            edit in editor →
          </button>
        )}
      </div>
      <Select
        value={selectedPreset}
        onChange={(e) => onApply(e.target.value)}
        disabled={submitting}
      >
        <option value="">— none —</option>
        {presets.map((p) => (
          <option key={p.name} value={p.name}>
            {p.display_name ?? p.name}
          </option>
        ))}
      </Select>
      {selectedPresetMeta &&
        !!(
          selectedPresetMeta.description ||
          selectedPresetMeta.prompt ||
          selectedPresetMeta.skills?.length
        ) && (
          <div className="mt-2 rounded bg-surface-2 border border-border-default px-2 py-1.5 text-micro text-fg-muted">
            {selectedPresetMeta.description && (
              <p className="text-fg-default">
                {selectedPresetMeta.description}
              </p>
            )}
            {selectedPresetMeta.prompt && (
              <p className="mt-1 max-h-24 overflow-y-auto whitespace-pre-wrap">
                {selectedPresetMeta.prompt}
              </p>
            )}
            {selectedPresetMeta.skills &&
              selectedPresetMeta.skills.length > 0 && (
                <p className="mt-1 text-fg-subtle">
                  Skills: {selectedPresetMeta.skills.join(", ")}
                </p>
              )}
          </div>
        )}
      <p className="mt-1 text-caption text-fg-subtle">
        Selecting a preset overlays its values onto the inputs
        below and biases every step (its “## Focus”). Any further
        edits override the preset; the engine applies the same
        precedence (preset &lt; vars).
      </p>
    </section>
  );
}
