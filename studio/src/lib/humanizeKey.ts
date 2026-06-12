// humanizeKey turns a snake_case / camelCase identifier into a title the
// operator reads as English: "selected_story_ids" → "Selected story ids",
// "nextAction" → "Next action". Shared by the run-console output cards
// (the values an agent emits) and the human-form labels (the fields those
// same schemas ask the operator to fill back) so the two surfaces render
// identical field names.
export function humanizeKey(k: string): string {
  const spaced = k.replace(/_/g, " ").replace(/([a-z])([A-Z])/g, "$1 $2");
  return spaced.charAt(0).toUpperCase() + spaced.slice(1);
}
