import { useMemo } from "react";

import type { BotEntry } from "@/api/bots";
import { Combobox, type ComboboxOption } from "@/components/ui/Combobox";
import { botIdentity } from "@/lib/personas";

interface Props {
  value: string;
  bots: BotEntry[];
  onChange: (botName: string) => void;
  disabled?: boolean;
  /** Pre-baked tags shown to the right of each row (default: triggers
   *  + capabilities, comma-joined and truncated). */
  metaFor?: (bot: BotEntry) => string;
}

const defaultMeta = (b: BotEntry): string => {
  const tags = [...(b.triggers ?? []), ...(b.capabilities ?? [])];
  if (tags.length === 0) return "";
  const joined = tags.join(", ");
  return joined.length > 32 ? joined.slice(0, 29) + "…" : joined;
};

/** BotPicker is the Board ticket form's bot selector. Empty value =
 *  "use dispatcher default" (the per-assignee / global workflow from
 *  the dispatcher config). A non-empty value overrides that choice at
 *  launch time (pkg/dispatcher/loop.go buildSpec).
 */
export function BotPicker({
  value,
  bots,
  onChange,
  disabled,
  metaFor = defaultMeta,
}: Props) {
  const options = useMemo<ComboboxOption<string>[]>(
    () =>
      // Disabled bots (catalog toggle off) aren't routable, so they don't
      // belong in the ticket assignment picker. The current value is kept
      // even if disabled (below) so an existing assignment still renders.
      bots
        .filter((b) => b.enabled !== false || b.name === value)
        .map((b) => {
        // The manifest persona (display_name) leads, with the emoji
        // avatar; the technical name moves into the description and stays
        // in the search haystack so operators can still type "feature_dev".
        const persona = b.display_name?.trim();
        const { emoji } = botIdentity(b.name);
        const label = persona ? `${emoji} ${persona}` : b.name;
        const description = persona
          ? `${b.name}${b.description ? ` — ${b.description}` : ""}`
          : b.description;
        return {
          value: b.name,
          label,
          description,
          meta: metaFor(b) || undefined,
          searchHaystack: `${persona ?? ""} ${b.name} ${b.description ?? ""} ${(b.triggers ?? []).join(" ")} ${(b.capabilities ?? []).join(" ")}`,
        };
      }),
    [bots, metaFor, value],
  );

  return (
    <Combobox<string>
      value={value}
      options={options}
      onChange={(v) => onChange(v)}
      emptyLabel="(dispatcher default)"
      emptyDescription="Falls back to the workflow configured under Dispatcher settings."
      placeholder="Search bots…"
      disabled={disabled}
    />
  );
}
