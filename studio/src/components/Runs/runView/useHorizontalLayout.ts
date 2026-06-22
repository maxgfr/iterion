import { useLayoutPersistence } from "@/hooks/useLayoutPersistence";

// Owns the four horizontal-layout persistence handles for the canvas
// row (canvas / detail / optional Browser / optional Chat) and derives
// the active handle + per-Panel size baselines based on which docks are
// currently mounted. Lifted out of RunView so the host doesn't have to
// repeat the 4-way ternary at every {defaultLayout, onLayoutChanged,
// defaultSize} site.
//
// Separate layout keys for each dock combo so the right-dock split
// (canvas / detail / browser) doesn't collide with the canvas/detail-
// only layout when the user toggles the dock.
export interface HorizontalLayoutResult {
  active: ReturnType<typeof useLayoutPersistence>;
  canvasSize: number;
  detailSize: number;
  browserRightSize: number;
  chatPanelSize: number;
  // Reset all four persistence handles so a single host-level "Reset
  // layout" command snaps every horizontal Group back to its default,
  // regardless of which dock combo is currently active.
  resetAll: () => void;
}

export function useHorizontalLayout({
  browserRightDocked,
  chatDockedRight,
}: {
  browserRightDocked: boolean;
  chatDockedRight: boolean;
}): HorizontalLayoutResult {
  const horizontalLayout = useLayoutPersistence(
    "run-console-v2.horizontal",
    { canvas: 70, detail: 30 },
  );
  // Separate layout key so the right-dock split (canvas / detail /
  // browser) doesn't collide with the canvas/detail-only layout when
  // the user toggles the dock.
  const horizontalLayoutWithBrowser = useLayoutPersistence(
    "run-console-v2.horizontal-with-browser",
    { canvas: 50, detail: 25, browserRight: 25 },
  );
  // Layout key for when the chat panel is docked to the right (3rd
  // resizable column). Includes the chat slot at the end.
  const horizontalLayoutWithChat = useLayoutPersistence(
    "run-console-v2.horizontal-with-chat",
    { canvas: 50, detail: 20, chat: 30 },
  );
  // Layout key for the rare case where both browser AND chat dock to
  // the right at the same time (4 horizontal columns).
  const horizontalLayoutWithBrowserAndChat = useLayoutPersistence(
    "run-console-v2.horizontal-full-right",
    { canvas: 40, detail: 20, browserRight: 20, chat: 20 },
  );

  // Pick the layout persistence handle once, indexed by the active
  // column set so we don't repeat the same 4-way ternary at every
  // {defaultLayout, onLayoutChanged, defaultSize}. Stays inline — the
  // four sources don't share an identity and memoising would capture
  // stale onChange callbacks.
  const active = browserRightDocked && chatDockedRight
    ? horizontalLayoutWithBrowserAndChat
    : browserRightDocked
    ? horizontalLayoutWithBrowser
    : chatDockedRight
    ? horizontalLayoutWithChat
    : horizontalLayout;
  const canvasSize = browserRightDocked && chatDockedRight
    ? 40
    : browserRightDocked || chatDockedRight
    ? 50
    : 70;
  const detailSize = browserRightDocked || chatDockedRight ? 22 : 30;
  const browserRightSize = chatDockedRight ? 18 : 25;
  const chatPanelSize = browserRightDocked ? 20 : 30;

  const resetAll = () => {
    // Each layout's reset() bumps its own groupKey, remounting the Groups
    // so they re-read the just-reset defaultLayout — see
    // useLayoutPersistence.
    horizontalLayout.reset();
    horizontalLayoutWithBrowser.reset();
    horizontalLayoutWithChat.reset();
    horizontalLayoutWithBrowserAndChat.reset();
  };

  return { active, canvasSize, detailSize, browserRightSize, chatPanelSize, resetAll };
}
