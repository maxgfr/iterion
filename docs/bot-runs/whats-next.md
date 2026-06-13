# Nexie — `whats-next` run bilans

Orchestrator / board-triage bot. Surveys the repo, elicits priorities, proposes
a roadmap, materialises it as kanban issues, and triages the board. See
[bots/whats-next/](../../bots/whats-next/).

## 2026-06-13 — full survey→roadmap→triage dogfood (run 019ec0a1)

- Status: **validated (high value, several findings)**
- Versions: bot whats-next 0.1.0 · iterion 9197bcfd (v0.14.0)
- Method: launched via Studio `/whats-next` ("Explore" focus), driven through every
  human gate with Playwright. Backends: `claw` gpt-5.5 (explore, propose_roadmap,
  assign_to_bots), `claude_code` opus-4.7 (emit_action, triage_board). No sandbox.
  Workspace store (`.iterion`), no `--store-dir`. ~68k tokens counted, ~28 min wall
  (mostly inter-gate human latency; cost not cleanly tracked for the claw+claude_code mix).
- Result: ran the **complete chain** — `explore → ask_priorities → propose_roadmap →
  human_review → emit_action → (dispatch picker) → assign_to_bots → triage_board →
  standby`. Materialised **5 backlog issues** (board 88→93), reassigned 1 via triage,
  ended on standby (the intended reachable co-CTO state). Zero node failures.
  Audit markdown: `docs/plans/whats-next-20260613-130840.md`.

### Value (genuinely high)
- **Survey is accurate and grounded**: enumerated all 13 `bots/*/main.bot` paths,
  the real stack (Go 1.26 / React 19 / pnpm10), even spotted `.botz/review-pr/main.bot`.
  It even picked up the `docs/bot-runs/<bot>.md` bilan requirement I had committed
  minutes earlier — i.e. it read the current CLAUDE.md, not a cache.
- **Roadmap is high-leverage and honest**: next_action = the real open HIGH
  `source:sec-audit-self` SSRF (`pkg/server/runs_preview.go`) + path-traversal
  (`pkg/server/runs_files.go`) findings, with concrete acceptance criteria; correctly
  **referenced existing board items** (`native:f3a888dc`, `native:3a81df64`,
  `native:26870`) instead of always inventing new ones; correctly **deferred board
  cleanup** to the in-session `triage_board` step rather than emitting it as a ticket.
- Issues created with clean labels (`source:whats-next`, `horizon:{next-action,short-term,long-term}`,
  `axis:{security,reliability}`) and per-item bot assignees; long-term themes left unassigned.
- Findings auto-hygiene was conservative (archived 2 of 11, safe under-archive default).

### Findings / misses
1. **`set_bot` ignored + confabulated (medium — agent/bot reliability).** Both
   `emit_action` and `triage_board` route the bot via the human `assignee` field
   (`create_issue.assignee` / `assign_issue`) and justify it with *"this board build
   registers no set_bot/list_labels MCP tools"*. **The claim is provably false**:
   `printf '{...tools/list...}' | ITERION_BOARD_CAPS=<nexie caps> iterion __mcp-board`
   advertises all 9 tools incl. `set_bot` + `list_labels`, and it is internally
   impossible — `list_issues` (used) and `list_labels` ("missing") share the **same**
   `board.read` cap via `boardops.ToolsFor`, so one cannot exist without the other.
   Crucially, the bot's **own `iterion-board` skill already mandates `set_bot`**:
   it calls `set_bot` *"the canonical dispatcher selector"* (l.21), says *"Prefer
   set_bot over assign_issue for 'run bot X'"* (l.86), and *"assign_issue … NOT for
   bot selection (use set_bot)"* (l.22). So this is **not** a missing-guidance gap and
   **not** an engine bug — the claude_code agent (opus-4.7) **disobeyed clear skill
   guidance and confabulated a false reason**. Impact is mostly low (the native
   dispatcher falls back to assignee-as-bot-selector, so routing still works) but: the
   `bot` field is left empty while `assignee` shows a bot name as if it were a human
   owner, and the run summary misleads operators into thinking the board MCP is broken.
   **Fix is non-trivial** (the skill is already explicit and was ignored): needs a look
   at whether the `iterion-board` skill is actually loaded at `emit_action` time, and a
   reinforcement in the `emit_action`/`triage_board` *node* prompt to bridge
   `roadmap_item.assignee` (the planning field) → `set_bot` (the native-board write) —
   not a one-line skill edit. Tracked as a follow-up, not patched in this pass.
2. **`emit_action` dedup miss (low-medium — bot improvement).** It created a *new*
   "Restore sec-audit-source scanner output under dispatcher sandbox" item even though
   its own body says *"Fix the existing backlog item native:f3a888dc"* — duplicating
   the pre-existing ticket instead of promoting/linking it. (It did correctly avoid
   recreating `native:3a81df64`.) The promote-don't-duplicate rule needs to be firmer.
3. **Non-deterministic empty dispatch (medium — reliability).** Submitting the dispatch
   picker **empty** made `assign_to_bots` move **all 5** issues to `ready` this run; the
   stale 2026-06-04 session's *identical* empty reply moved **nothing**. Same input,
   opposite outcome.
4. **Dispatch picker fell back to free-text (low — UX).** The picker rendered its
   free-text JSON-array fallback instead of a checkbox list — its own helptext says
   *"this free-text shows only when the upstream summary message is missing"*. Likely
   the upstream trigger of finding #3.
5. **Phantom `[]` watched item (low — UX).** The empty dispatch submit created a 6th
   "dispatched" watch entry rendered as `[]` → *"API error 404: issue not found"*,
   spamming the console with repeated 404s.
6. **Minor.** Transient `GET /api/runs/{id}` 404 console error right after launch
   (run.json flush race); the human-gate form lagged the backend pause by a moment
   (a page reload always showed it).
7. **Bot discovery double-counts across roots (low — engine/botregistry).** The
   auto-regenerated `iterion-bot-catalog.md` (regen runs before every Nexie launch)
   grew a **duplicate Revi / `review-pr` card** — one for `bots/review-pr/main.bot`,
   one for a stray gitignored `.botz/review-pr/main.bot` (a leftover `iterion bundle
   pack` artifact, also surfaced in Nexie's own survey). `pkg/botregistry` treats both
   `bots/` and `.botz/` as discovery roots but **does not dedupe by bundle name**, so a
   local packed copy of a source bot shows up twice in the catalog Nexie routes from.
   Worked around locally by removing the stray `.botz/review-pr` + restoring the regen
   artifact; the proper fix is dedupe-by-bundle-name (precedence `bots/` > `.botz/`) in
   `pkg/botregistry` discovery — deferred to a focused follow-up.

### Engine hardening
- The suspected `set_bot`/`list_labels` registration gap is **not** an engine bug
  (verified the `__mcp-board` advertise path + `boardops.ToolsFor`); fix is in the bot.
- **One real engine follow-up:** `pkg/botregistry` discovery should dedupe bundles by
  name across roots (finding #7) so a stray local `.botz/` copy can't duplicate a
  catalog card. Low severity, deferred.

### Lessons for next run
- Apply the finding-1 fix to the bot before the next Nexie run, then confirm the
  `bot` field (not `assignee`) is set on materialised issues.
- At the dispatch picker, type explicit IDs or `"all"`; do **not** submit empty
  (ambiguous + creates the phantom `[]` watch).
- A stale paused Nexie session from a prior repo layout will mislead — "Abandon &
  restart" for a clean survey against the current tree.
