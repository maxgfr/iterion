import { useState } from "react";

import type { BotEntryWithSchema, BotPatch } from "@/api/bots";
import { CheckboxField, TagListField, TextField } from "@/components/Panels/forms/FormField";
import { useBotsStore } from "@/store/bots";
import { useUIStore } from "@/store/ui";

interface Draft {
  display_name: string;
  description: string;
  when_to_use: string;
  triggers: string[];
  author: string;
  version: string;
  enabled: boolean; // edits the MANIFEST default (manifest_enabled)
}

function toDraft(b: BotEntryWithSchema): Draft {
  return {
    display_name: b.display_name ?? "",
    description: b.description ?? "",
    when_to_use: b.when_to_use ?? "",
    triggers: b.triggers ?? [],
    author: b.author ?? "",
    version: b.version ?? "",
    enabled: b.manifest_enabled !== false,
  };
}

/**
 * BotMetadataForm is the Inspector "Bot" tab — edits a bundle's
 * manifest.yaml (persona, description, the Nexie-facing "when to use",
 * triggers, author/version, and the catalog default). Mounted with
 * `key={bot.name}` so switching files re-seeds the draft; dirty state is
 * computed against the live entry, so a successful save (which refreshes
 * the bots store) clears it.
 *
 * The catalog checkbox writes the manifest DEFAULT; a workspace overlay
 * (set via the Catalog manager) can override it locally — surfaced as a
 * note when the resolved `enabled` diverges from the manifest default.
 */
export default function BotMetadataForm({ bot }: { bot: BotEntryWithSchema }) {
  const saveBot = useBotsStore((s) => s.saveBot);
  const addToast = useUIStore((s) => s.addToast);
  const [draft, setDraft] = useState<Draft>(() => toDraft(bot));
  const [saving, setSaving] = useState(false);

  const baseline = toDraft(bot);
  const dirty = JSON.stringify(draft) !== JSON.stringify(baseline);
  const overlayDiffers = bot.enabled !== bot.manifest_enabled;

  const update = <K extends keyof Draft>(k: K, v: Draft[K]) =>
    setDraft((d) => ({ ...d, [k]: v }));

  const onSave = async () => {
    setSaving(true);
    const patch: BotPatch = {
      display_name: draft.display_name.trim(),
      description: draft.description,
      when_to_use: draft.when_to_use,
      triggers: draft.triggers,
      author: draft.author.trim(),
      version: draft.version.trim(),
      enabled: draft.enabled,
    };
    try {
      const updated = await saveBot(bot.name, patch);
      setDraft(toDraft(updated));
      addToast(`Saved ${updated.display_name?.trim() || updated.name}`, "success");
    } catch (e) {
      addToast(e instanceof Error ? e.message : "Failed to save bot metadata", "error");
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="h-full overflow-y-auto p-3">
      <div className="mb-3">
        <div className="mb-1 text-xs text-fg-subtle">Bot (technical name — immutable)</div>
        <div className="font-mono text-sm text-fg-default">{bot.name}</div>
      </div>

      <TextField
        label="Persona name (display_name)"
        value={draft.display_name}
        onChange={(v) => update("display_name", v)}
        placeholder="e.g. Nexie"
      />
      <TextField
        label="Description"
        value={draft.description}
        onChange={(v) => update("description", v)}
        multiline
        rows={4}
      />
      <TextField
        label="When to use (shown to Nexie)"
        help="Nexie reads this to decide whether to route a task to this bot — like a skill's “when to use it”."
        value={draft.when_to_use}
        onChange={(v) => update("when_to_use", v)}
        multiline
        rows={3}
        placeholder="Use when…"
      />
      <TagListField
        label="Triggers"
        values={draft.triggers}
        onChange={(v) => update("triggers", v)}
        placeholder="Add trigger…"
      />
      <div className="grid grid-cols-2 gap-2">
        <TextField label="Author" value={draft.author} onChange={(v) => update("author", v)} />
        <TextField label="Version" value={draft.version} onChange={(v) => update("version", v)} />
      </div>

      <div className="mt-2 border-t border-border-default pt-2">
        <CheckboxField
          label="Active in catalog (exposed to Nexie)"
          checked={draft.enabled}
          onChange={(v) => update("enabled", v)}
          help="When on, Nexie can route tasks to this bot and it shows in the board bot picker. This sets the bot's manifest default; the Catalog manager can override it per-workspace."
        />
        {overlayDiffers && (
          <p className="mt-0.5 text-[10px] text-warning">
            Locally overridden: this workspace currently treats it as{" "}
            {bot.enabled ? "enabled" : "disabled"} (via the Catalog manager),
            regardless of the manifest default above.
          </p>
        )}
      </div>

      <div className="mt-3 flex items-center gap-2">
        <button
          type="button"
          disabled={!dirty || saving}
          onClick={onSave}
          className="rounded bg-accent px-3 py-1 text-xs text-fg-default disabled:cursor-not-allowed disabled:opacity-50"
        >
          {saving ? "Saving…" : "Save changes"}
        </button>
        {dirty && !saving && <span className="text-[10px] text-warning">Unsaved changes</span>}
      </div>
    </div>
  );
}
