# Iterion Studio — Design System

This is a working reference for studio contributors. The goal is **discipline of adoption**, not novelty: the primitives below already exist; reach for them before writing ad-hoc styling.

## Tokens

Single source of truth: [`studio/src/app.css`](../src/app.css). Everything below is generated from `@theme` custom properties — change a value here and every consumer (canvas, inspector, run console, board, dispatcher) follows.

| Family | Tokens | When to use |
|---|---|---|
| Surfaces | `surface-0` (deepest bg) → `surface-3` (elevated/hover) | Backgrounds, panels, popovers |
| Foreground | `fg-default`, `fg-muted`, `fg-subtle`, `fg-onAccent` | Text contrast tiers |
| Borders | `border-default`, `border-strong`, `border-subtle` | Dividers, card outlines |
| Accent | `accent`, `accent-hover`, `accent-soft`, `accent-fg` | Primary interactive surfaces |
| Severity | `danger`, `warning`, `success`, `info` (+ `-soft` and `-fg` variants) | Status, validation, badges |
| Node-kind | `node-agent`, `node-judge`, `node-router`, `node-human`, `node-tool`, `node-compute`, `node-done`, `node-fail`, `node-start`, `node-join`, `node-group` | Canvas borders, form headers, library cards |
| Layer | `layer-schemas`, `layer-prompts`, `layer-vars` | Layer overlay + sub-node palette |
| Selection | `selected`, `sub-tool` | Selected highlights, sub-node tool kind |
| Library | `library-pattern` | "Pattern" library category (no node-kind equivalent) |
| Radii | `radius-sm`/`md`/`lg`/`xl` | Component corner radius |
| Motion | `motion-fast` (120ms) / `motion-base` (180ms) / `motion-slow` (280ms), `motion-ease` | Transitions, animations |

### TypeScript-side mirrors

`lib/constants.ts` re-exposes the node/layer/selection palette as `var(--...)` strings:

```ts
import { NODE_COLORS, LAYER_COLORS, SUB_COLORS, SELECTED_BORDER, SELECTED_GLOW, softColor } from "@/lib/constants";

// NODE_COLORS.agent === "var(--color-node-agent)" — works in inline styles,
// xyflow markerEnd.color, SVG fills, anywhere CSS values are accepted.
```

**Never** write a raw hex literal in a canvas, form, or run-console component. If you reach for one, ask: which token does this mean? Add it to `app.css` first, then consume.

For semi-transparent overlays (the legacy `${hex}22` pattern), use the `softColor` helper:

```ts
style={{ backgroundColor: softColor(NODE_COLORS.tool) }}        // 13% (default — matches old "22")
style={{ background: softColor(color, 10) }}                    // 10% (matches old "1A")
```

It compiles to `color-mix(in srgb, var(--color-node-tool) 13%, transparent)`. Supported in all modern browsers (Chromium 111+, Firefox 113+, Safari 16.4+).

## Primitives

Use these first. If the use-case doesn't fit, **extend the primitive** rather than rolling your own.

### Buttons & icon buttons

[`ui/Button.tsx`](../src/components/ui/Button.tsx) — `variant: primary | secondary | ghost | danger`, `size: sm | md`, `loading` prop, leading/trailing icons. Spinner is wired automatically when `loading={true}`.

[`ui/IconButton.tsx`](../src/components/ui/IconButton.tsx) — square icon-only variant for toolbars.

### Confirmation dialogs

**Never** call `window.confirm()`. It bypasses the theme, breaks focus management, and looks alien in a Tailwind UI.

Use the [`useConfirm`](../src/hooks/useConfirm.tsx) hook:

```tsx
const { confirm, dialog } = useConfirm();

const handleDestructive = async () => {
  const ok = await confirm({
    title: "Discard unsaved changes?",
    message: "You have unsaved changes that will be lost.",
    confirmLabel: "Discard",
    confirmVariant: "danger",
  });
  if (!ok) return;
  // ... do the thing
};

return <>... {dialog}</>;
```

For dialogs with custom bodies or non-binary outcomes, drop down to [`shared/ConfirmDialog`](../src/components/shared/ConfirmDialog.tsx) directly (it supports a third "secondary action" button).

For non-confirmation modals (file picker, forms, multi-step wizards), use [`ui/Dialog`](../src/components/ui/Dialog.tsx) (Radix-backed).

### Empty / loading / error states

Use [`ui/EmptyState`](../src/components/ui/EmptyState.tsx) for **every list, tree, or panel that may have no data**. Three branches, one component:

```tsx
{error ? <EmptyState message={<span className="text-danger">{error}</span>} />
 : loading ? <EmptyState message="Loading…" />
 : items.length === 0 ? <EmptyState message="No commits yet" />
 : <List items={items} />}
```

For "shimmer while data loads" rather than a centered message, use [`ui/Skeleton`](../src/components/ui/Skeleton.tsx).

### Spinner

[`ui/Spinner.tsx`](../src/components/ui/Spinner.tsx) — `size: xs | sm | md`. Use for **pure indeterminate loading**:

```tsx
<Spinner size="sm" /> Loading…
```

Note: the existing pattern of rotating an icon (e.g. `<ReloadIcon className="animate-spin" />`) is a **different design choice** and intentionally kept where it shows that a specific operation (reload, refresh) is in flight. Don't blanket-replace those with `Spinner` — the icon carries semantics.

### Toasts

`useUIStore.addToast(message, level)` with four levels: `info`, `warning`, `error`, `success`. Optional `{ persistent: true }`. Use for **transient asynchronous feedback** (save complete, reload failed, etc.). Don't swallow API errors — toast them.

### Tabs, Inputs, Selects, Badges

See [`ui/index.ts`](../src/components/ui/index.ts) for the full export list. All wired with proper tokens, focus rings, and disabled states.

## Patterns

### Disabled controls

Always pair `disabled:opacity-*` (or color change) with `disabled:cursor-not-allowed`. Opacity alone makes the control look interactive while being inert — that's a a11y faux-positive.

The Button primitive uses the canonical pattern:

```css
disabled:bg-surface-2 disabled:text-fg-subtle disabled:cursor-not-allowed
```

For text/icon-only buttons where a background swap is too heavy, use:

```css
disabled:opacity-50 disabled:cursor-not-allowed disabled:hover:text-fg-subtle
```

### Form headers (inspector)

Each node-kind form (`AgentForm`, `RouterForm`, `ToolForm`, etc.) renders a colored header so the user knows what they're editing at a glance. The header color **must** equal the node-kind token — that's how the canvas → inspector visual link works.

```tsx
const headerColor = NODE_COLORS[kind]; // "var(--color-node-agent)"
<div
  className="flex items-center gap-2 px-2 py-1.5 rounded mb-2 -mx-1"
  style={{
    backgroundColor: softColor(headerColor),
    borderLeft: `3px solid ${headerColor}`,
  }}
>
  <span style={{ color: headerColor }}>{headerLabel}</span>
</div>
```

### Status colors

Stick to the severity palette. Red is **reserved** for `node-fail` and diagnostic errors — don't use red for anything else. Amber/`warning` is the second-tier "needs attention" token; reuse it for entry markers, loop edges, schema mismatches.

### Canvas edges (xyflow)

`markerEnd.color` accepts CSS variables — pass `"var(--color-warning)"` rather than `"#F59E0B"`. xyflow renders the marker as SVG `<polygon fill={color}>`, which resolves the variable against `:root`.

```ts
markerEnd: { type: MarkerType.ArrowClosed, color: "var(--color-fg-subtle)", width: 16, height: 16 }
```

### Themes

The studio supports dark and light themes via `[data-theme="..."]` on `<html>` (managed by [`store/theme.ts`](../src/store/theme.ts)). The full token palette — including the **canvas tokens** (`node-*`, `layer-*`, `selected`, `sub-tool`, `library-pattern`) — has light-mode overrides using the Tailwind-700 family for contrast against the lighter surface.

When adding a new color token to `@theme`, **always** add the matching `[data-theme="light"]` override. If you forget, light mode falls back to the dark default and prints poorly on white surfaces.

## Accessibility

What's already wired:
- `:focus-visible` global outline ([app.css:148](../src/app.css#L148)).
- All `disabled:` controls pair their visual change with `disabled:cursor-not-allowed` (Q4).
- `ConfirmDialog` traps focus and exits on Escape.
- `useConfirm` returns a Promise so call-sites stay synchronous-shaped.
- `Skeleton` renders `aria-hidden` so screen readers skip the shimmer.
- Toast component is announced via the toast bus.
- FormField inputs wire `aria-describedby` to their help icon and error message via the `FieldRow` wrapper. Set the `error` prop to render `<p role="alert">` and add `aria-invalid` on the input.

Open items (need human / axe-core verification):
- Light mode contrast on status badges (severity tokens currently desaturated for light — sample with WCAG checker).
- Canvas node text contrast on `softColor(color, 10)` backgrounds, both themes.
- Skip-link from the page chrome to the main work surface (editor canvas, run view, etc.). Not yet present.
- Full keyboard reachability of the canvas — recent commits added aux + sub-node keyboard nav, but cycling between root nodes via `Tab` alone is not yet exhaustive.

Run an axe-core sweep (e.g. `axe-core` browser extension) on `/`, `/editor`, `/runs/:id`, `/board`, `/dispatcher`, `/settings` to surface anything missed.

## Don'ts

| Pattern | Why | Use instead |
|---|---|---|
| `window.confirm("…")` | Bypasses theme, no focus mgmt, alien UX | `useConfirm()` |
| `style={{ color: "#3B82F6" }}` | Drifts from tokens, breaks theme switch | `style={{ color: NODE_COLORS.agent }}` or `text-node-agent` utility |
| `${hex}22` for soft bg | Doesn't work with `var()` strings | `softColor(token)` |
| `<div>Loading…</div>` ad-hoc | Visual drift across panels | `<EmptyState message="Loading…" />` |
| `<span className="animate-spin border-2 …" />` ad-hoc | Reinvents Button's spinner | `<Spinner size="sm" />` |
| `disabled:opacity-50` alone on a button | a11y faux-positive (still looks clickable) | Add `disabled:cursor-not-allowed` |
| `text-error`, `border-error` | These classes don't exist (no `--color-error` token) | `text-danger`, `border-danger` |
| New CSS-in-JS hex outside `app.css` / `lib/constants.ts` | Bypasses palette, breaks future theme variants | Add a token to `app.css`, expose via constants |

## When to extend vs roll your own

Before adding a new primitive, ask:

1. **Is there already a primitive that almost fits?** Extend its props (add a `variant`, a size) instead of forking.
2. **Is this a one-off vs reusable?** A one-off form layout is fine inline; a pattern repeated 3+ times deserves a primitive.
3. **Where does this fit in the tokens?** If you can't name the design tokens you'd use, the design isn't finished yet — pause and define them.

## Open items

- **Status pulse semantics** — `animate-pulse` is currently used for 5 different meanings (WS live, run running, diagnostic urgent, node running, info). Worth a separate audit to assign clear visual vocabulary.
- **WCAG AA contrast audit on every theme** — light mode overrides shipped, but no automated pass yet. Run axe-core (browser extension) on `/`, `/editor`, `/runs/:id`, `/board`, `/dispatcher`, `/settings`.
- **Skip-link** for keyboard users from the page header to the main work surface.
- **Mobile / responsive** — explicitly out of scope until a use-case demands it.
