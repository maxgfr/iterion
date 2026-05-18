// Shared micro-copy. Centralised here so identical strings actually
// stay identical across the studio — and so a future i18n pass has a
// single seam to translate. Keep this file small: a constant earns a
// home here when it appears verbatim in 2+ places, NOT for every
// label or button in the app.

import type { ConfirmOptions } from "@/hooks/useConfirm";

/**
 * "Discard unsaved changes?" prompt used by the Editor toolbar, the
 * home recents panel, and the deep-link handler in EditorView.
 * Three callers, identical wording → factored out so any future
 * tweak (translation, softer phrasing) propagates everywhere.
 */
export const DISCARD_CHANGES_PROMPT: ConfirmOptions = {
  title: "Discard unsaved changes?",
  message: "You have unsaved changes that will be lost.",
  confirmLabel: "Discard",
  confirmVariant: "danger",
};
