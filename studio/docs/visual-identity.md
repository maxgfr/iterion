# Iterion Studio — Visual identity brief

This is a working brief, not a brand book. It captures the design
intent behind the token palette so future contributors can answer
"why this color, not that color?" without re-litigating from scratch.

## The product mental model

Iterion is a workflow orchestration engine for developers. Operators
sit in front of the studio to:

- Edit `.iter` workflows in a dense node-graph canvas.
- Launch runs and watch them stream through an event log + IR
  visualisation.
- Triage failures, resume from checkpoints, judge cost vs progress.

This is **engineering tooling**, not consumer SaaS. The visual posture
should read **calm, technical, dense, deliberate** — closer in feel to
a debugger or a tracing UI than to a marketing dashboard. Avoid:

- Soft pastel gradients.
- Round, friendly illustrations.
- Hero typography that dominates content.
- Playful empty-state mascots.

Prefer:

- Clear surface hierarchy (4 tiers — see `design-system.md` § Surface
  hierarchy).
- High-contrast typography on neutral backgrounds.
- Status colour used semantically, not decoratively.
- Density over breathing room — every pixel earns its place.

## Primary accent

**Decision (2026-05-18): keep `--color-accent = #2563eb` (Tailwind
`blue-600`) for now.**

A "move off generic Tailwind blue" was on the table for Phase 4 of the
UX refresh. We deferred for these reasons:

1. The current blue already reads as a deliberate, restrained accent
   on both themes. No live evidence that it competes with the node-
   kind palette (the agent-node is `blue-500`, accent is `blue-600` —
   close but the role separation is clear in practice).
2. Picking a new identity hue without a design conversation is exactly
   the kind of decision that should not be made unilaterally by an
   implementation agent.
3. The accent shows up in ~200+ callers via `bg-accent`, `text-accent`,
   `border-accent`, `accent-soft`. Token-level swap is a 1-line edit
   in `app.css` and everything follows; the harder work is choosing
   the right hue.

**Migration path** when a direction emerges:

- Update `--color-accent`, `--color-accent-hover`, `--color-accent-fg`,
  `--color-accent-soft` in both `:root[data-theme="dark"]` and
  `[data-theme="light"]` blocks of `studio/src/app.css`.
- Verify WCAG AA contrast on `bg-accent text-fg-onAccent` and
  `bg-accent-soft text-accent` in both themes.
- Sweep `AppHeader.tsx` brand wordmark for any hue-specific styling.

## Secondary accent: "live"

Added in Phase 4 (this commit). `--color-live` is the dedicated token
for **"this is currently running"** — a separate semantic from the
informational `--color-info` (cyan-600).

| Token | Dark | Light | Use for |
|---|---|---|---|
| `info` | `cyan-600` (`#0891b2`) | `cyan-700` (`#0e7490`) | Informational toasts, neutral status |
| `live` | `cyan-400` (`#22d3ee`) | `cyan-600` (`#0891b2`) | Pulse on active runs, BackendStatusPill detecting, RunHeader active LiveDot |

Why same family: `live` reads as a "fresher, more-active" cyan against
`info`'s calmer tone. The hue family stays consistent so the eye does
not pivot when transitioning from steady-state to active.

`LiveDot` accepts `tone="live"` for this purpose; existing
`tone="info"` callers that semantically meant "in flight" migrate to
`live` (the RunHeader active dot and similar).

## Severity palette

Kept as-is (red/amber/green/cyan + their soft and fg variants). The
node-kind palette and iteration palette are independent dimensions —
they do not need to harmonise with the severity palette. See
`design-system.md` § Tokens for the full table.

**Reservation: red is for `node-fail` and diagnostic errors only.**
Don't introduce red anywhere else.

## Brand mark

`AppHeader` currently renders the wordmark "ITERION" in tracked-out
small-caps. No icon, no logotype. This stays minimal for now — a more
deliberate mark is downstream of a fuller identity conversation.

## Third-party design libraries

**Recommendation: don't add one.**

The studio is already a Radix + Tailwind v4 + tokens system. Adding
shadcn/ui would duplicate primitives we already have. Mantine and
Park UI would conflict with token discipline. `react-aria-components`
would overlap with Radix. Tremor is interesting if we ever ship a
metrics dashboard but is overkill until then. See `design-system.md`
§ Don'ts for the formal guard.

## Future work

- Light-mode contrast sweep with axe browser extension on the canvas
  (`softColor(color, 10)` backgrounds still need verification).
- Identity exploration when there's a design conversation: candidates
  to explore are indigo-600 (more saturated/distinctive) or a custom
  brand hue. Test against the agent-node blue-500 to ensure separation.
- Iteration palette ↔ live-accent alignment: currently
  `--color-iteration-0` is `#06b6d4` (cyan-500), one step from `live`'s
  `#22d3ee`. They render as adjacent in flight; consider lifting
  iteration-0 to match `live` exactly so an active iteration ring and
  the LiveDot read as the same accent.
