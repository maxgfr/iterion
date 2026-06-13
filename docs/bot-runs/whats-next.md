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
1. **`set_bot`/`list_labels` truly absent at runtime — STALE INSTALLED BINARY (medium
   — dev-infra, ROOT-CAUSED).** Both `emit_action` and `triage_board` routed the bot
   via the human `assignee` field and reported *"this board build registers no
   set_bot/list_labels MCP tools"*. **The agent was correct, not confabulating.** The
   studio under `task studio:dev` runs via `go run`, whose `os.Executable()` is a
   volatile build path, so `proc.LocateIterionBinary()` skips it and falls back to the
   **installed `/usr/bin/iterion`** to serve the `__mcp-board` stdio MCP. That installed
   binary was **stale (commit 62aac3cc, pre-dating `set_bot`/`list_labels`)** and its
   `tools/list` advertises only **7 tools** (`assign_issue, close_issue, create_issue,
   get_issue, list_issues, set_labels, transition_issue`) — no `set_bot`, no
   `list_labels`. Proof: `ITERION_BOARD_CAPS=<6 caps> /usr/bin/iterion __mcp-board`
   tools/list → 7 tools; the **freshly-built** binary (current code) → all 9. So the
   bot prompt (`emit_action_system` l.709-713 already maps `item.assignee → set_bot`)
   and the `iterion-board` skill are **correct**; the agent faithfully used what the
   stale board server offered (assignee fallback, which the dispatcher honours). **Fix
   is operational, not a bot/code change:** refresh the installed binary
   (`sudo cp ./iterion /usr/bin/iterion`) or run the studio with
   `ITERION_BIN=<fresh>` so delegated subprocesses match the running code. **This skew
   affects EVERY delegated capability** (board MCP, the sandboxed `__claw-runner`, the
   `__mcp-ask-user` server) — see the CLAUDE.md note added under the live-dogfood
   section. (The chained findings #2–#5 below are downstream of routing via `assignee`
   and may differ once the binary is fresh and `set_bot` is used.)
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
- **Dev-mode delegated-binary skew (finding #1):** real, root-caused — the studio
  under `go run` serves `__mcp-board` (and the claw runner / ask-user MCP) from the
  **stale installed `/usr/bin/iterion`**, which silently lacks capabilities added since
  the last install (`set_bot`/`list_labels` here). Code is correct; refresh the binary
  or set `ITERION_BIN`. Documented in CLAUDE.md so it doesn't re-bite the campaign.
- **`pkg/botregistry` cross-root dedupe (finding #7): FIXED** — `discoverBots` now
  dedupes by normalized bundle name across roots (precedence `bots/` > `.botz/`), with
  a regression test (`TestList_DedupesSameBotAcrossRoots`). So a stray packed `.botz/`
  copy can no longer duplicate a catalog card.

### Lessons for next run
- Apply the finding-1 fix to the bot before the next Nexie run, then confirm the
  `bot` field (not `assignee`) is set on materialised issues.
- At the dispatch picker, type explicit IDs or `"all"`; do **not** submit empty
  (ambiguous + creates the phantom `[]` watch).
- A stale paused Nexie session from a prior repo layout will mislead — "Abandon &
  restart" for a clean survey against the current tree.
