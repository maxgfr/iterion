// shortIssueId is the canonical short-form rendering of a tracker
// issue id used wherever the UI needs a compact fallback (e.g. when
// the full title isn't available yet). The native-tracker scheme
// prefixes opaque uuids with `native:`; we drop that and keep the
// leading 8 chars, matching the form the operator already sees on
// the board cards.

export function shortIssueId(id: string): string {
  return id.replace(/^native:/, "").slice(0, 8);
}
