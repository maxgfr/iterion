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

**Decision (2026-06-21): shift to an electric indigo/periwinkle and
*decouple* accent-background from accent-text.**

The previous accent (`#2563eb`, Tailwind `blue-600`, kept on 2026-05-18)
had two problems we resolved together:

1. **A11y defect.** As link text on a dark surface, `text-accent` landed
   at ~3.4:1 — below the WCAG AA 4.5:1 floor. Accent links are common in
   WhatsNext / Board / Settings.
2. **Identity.** The studio is tooling — neutral, modern, tech-oriented —
   with room for a *gentle* cyberpunk/hacker nod (see § Brand voice). A
   restrained electric indigo serves that better than generic Tailwind
   blue and separates cleanly from the canvas `node-agent` (`blue-500`).

A single accent token cannot be AA both as **white-on-accent** (button
backgrounds need the accent *dark enough*) and as **accent-on-surface**
(links need it *bright enough*). So the system now carries two tokens:

| Token | Dark | Light | Use for |
|---|---|---|---|
| `--color-accent` | `#4f46e5` (indigo-600) | `#4f46e5` | Button/brand **background**, focus ring, borders. White-on-accent = 5.8:1. |
| `--color-accent-text` | `#818cf8` (indigo-400) | `#4f46e5` | Accent-coloured **text / links / icons** on a surface. AA on dark (5.7:1 on surface-0). |

`--color-accent-fg` (white) is the text *on* an accent background;
`--color-accent-soft` is the translucent tint for chips/hover. The
`text-accent` utility was swept to `text-accent-text` across the app;
`bg-accent` / `border-accent` / `ring-accent` stay on `--color-accent`.
The `--accent-rgb` var (consumed by the canvas `.pulse-flash` ping) now
matches the accent. `theme-color` in `index.html` + `manifest.json` was
reconciled from a stray violet (`#7c3aed`) to `#4f46e5`.

**Migration path** if the hue changes again: update the accent +
accent-text quadruples in the `@theme`, `[data-theme="dark"]`, and
`[data-theme="light"]` blocks of `studio/src/app.css`, keep the contrast
assertions in `__tests__/a11y/contrast.test.ts` green (white-on-accent ≥
4.5, accent-text-on-surface ≥ 4.5 in both themes), and update
`theme-color`.

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

The wordmark lives in the **Sidebar** (`components/shared/Sidebar.tsx`),
rendered by the `BrandWordmark` primitive (`components/ui/BrandWordmark`):
tracked-out caps "ITERION" followed by a discreet accent caret (a static
terminal cursor). Pure text + a CSS bar — crisp at any size and
theme-perfect via `currentColor` / `text-accent-text`, replacing the
previous rasterised favicon + `dark:invert` crutch. The collapsed sidebar
shows the compact "I" monogram + caret. No icon, no logotype.

## Brand voice — a gentle cyberpunk/hacker nod

The studio stays **neutral, modern, tech-oriented** (it's tooling). The
one identity flavour layered on top is a *subtle, gentle* nod to
hacker/terminal culture — never the Roman-imperator marketing voice (that
stays in the README, never in-app). "Tone only":

- **Monospace for technical identifiers** — run-ids, commit SHAs,
  node-ids, branch names render in `font-mono`. Reads as a terminal and
  is genuinely more legible for fixed-width tokens.
- **The accent caret** in the wordmark (above) — one static terminal cursor.
- **The cyan `live` token** is the signature "alive" signal (§ Secondary
  accent). Reduced-motion-safe.
- **Gentle terminal microcopy** in a few empty/loading states (a blinking
  caret affordance), neutral-toned.

No neon glow, no scanlines, no grid texture, no mascots — the base
posture (calm / technical / dense) is unchanged.

## Third-party design libraries

**Recommendation: don't add one.**

The studio is already a Radix + Tailwind v4 + tokens system. Adding
shadcn/ui would duplicate primitives we already have. Mantine and
Park UI would conflict with token discipline. `react-aria-components`
would overlap with Radix. Tremor is interesting if we ever ship a
metrics dashboard but is overkill until then. See `design-system.md`
§ Don'ts for the formal guard.

## Future work

- Identity-hue exploration is **resolved** — see § Primary accent
  (2026-06-21: electric indigo/periwinkle, decoupled accent/accent-text).
- Light-mode contrast: now backed by deterministic assertions in
  `__tests__/a11y/contrast.test.ts`; a manual axe browser-extension pass
  on `/editor` in light mode is still worth doing before a release.
- Iteration palette ↔ live-accent alignment: currently
  `--color-iteration-0` is `#06b6d4` (cyan-500), one step from `live`'s
  `#22d3ee`. They render as adjacent in flight; consider lifting
  iteration-0 to match `live` exactly so an active iteration ring and
  the LiveDot read as the same accent.
