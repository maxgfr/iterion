[← Bot runs](README.md)

# adr-rechallenge (ReArchi) — bilans

Human-in-the-loop ADR re-challenger. Loads one ADR + the current code, frames
fresh arguments (changed assumptions, matured alternatives, dependency/code
drift, triggered consequences), and asks the operator: **keep / change /
addendum**. `change` files a backlog ticket; `addendum` appends a dated note
to the ADR, then a second commit-or-skip gate (the note is optional). Uses
`interaction: human` (not the heavier `interaction: review`); claude_code only
(no forfait dependency).

## 2026-06-14 — first dogfood, re-challenge ADR-008 (run 019ec5bc)

- **Status: validated** — full human-in-the-loop cycle exercised end-to-end;
  one commit-message bug found + fixed.
- **Versions:** bot 0.1.0 · iterion @ `92ccd136` (main)
- **Method:** `iterion run bots/adr-rechallenge/main.bot
  --var adr_path=docs/adr/008-bot-golden-replay-framework.md
  --var workspace_dir=<worktree>` (contained to the dogfood worktree, not
  main, since ReArchi has no `worktree: auto` and edits the live tree).
  claude_code (host OAuth) only — no claw/forfait nodes, so no token risk.
  Driven interactively: launch → pause → `iterion resume --answers-file`.
- **Result: full cycle, all branches reachable.** load_adr → survey_code →
  frame_arguments → **pause at human_decision** (operator chose *addendum*) →
  derive_decision → write_addendum → **pause at human_commit_gate** (operator
  chose *commit*) → derive_commit → commit_changes → done. The two human gates
  rendered + routed correctly; the launch→pause→resume→pause→resume flow
  worked via `--answers-file`.
- **Value: high — honest, evidence-based re-challenge.** Survey verified the
  ADR's claims against the *current* code with file:line evidence: the
  `api.APIClient` injection seam still doesn't exist (`executor.go:377-524`),
  so rejected Alternative 1 stays infeasible exactly as the ADR said; the
  assignee-coupling consequence hasn't fired (`set_bot` is only the MCP routing
  tool). It found **no change driver** and **correctly refused to manufacture a
  change case** (the argument-framing skill's anti-fabrication contract held).
  It surfaced two real record-keeping divergences (stale pre-rename bot names
  `c9996d98`/`8784d677`; four golden fixtures still original 2026-05-29 seeds
  never regenerated) and wrote a crisp, correctly-formatted addendum. The
  addendum was landed on main's ADR-008 (`a9e65e50`).

### Findings
| # | Finding | Severity | State |
|---|---|---|---|
| 1 | commit_changes committed the addendum with an unresolved/malformed message (`docs(adr): addendum to ADR-{{outputs.load_adr.num}} re-challenge\n\n…`) — embedded refs in the `with{}` edge string don't resolve (only PURE refs do; `files: "{{outputs.load_adr.path_list}}"` worked), and DSL `\n` stayed literal | real | **FIXED** `3a639f27` — write_addendum now emits a complete `full_message`, the edge passes it as a pure ref (matches the docs-refresh/feature-dev pattern). Live-validation deferred (static validate OK). |

### Lessons for next run
- **Never build a multi-part value with embedded refs inside an edge `with{}`
  string** — iterion resolves a pure ref (`"{{x}}"`) there but not refs
  embedded in a longer string, and DSL `\n` is literal. Build the value in a
  node (agent/compute) and pass it as a pure ref. (Tool `command:` templates DO
  resolve embedded refs — that's why `git -C {{input.workspace_dir}}` works —
  so the asymmetry is edge-mappings vs command bodies.)
- Run contained (`--var workspace_dir=<scratch>` or a throwaway clone) since
  ReArchi has no `worktree: auto` and commits to the live tree on the addendum
  path — otherwise it edits + commits the operator's checked-out ADRs directly.
- The `change` branch (file a backlog ticket) was not exercised this run —
  ReArchi correctly judged ADR-008 had no change driver. A future dogfood on an
  ADR with a genuine maturing alternative would cover it.
