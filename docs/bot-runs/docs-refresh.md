[← Bot runs](README.md)

# docs-refresh (Doki) — bilans

Documentation refresh bot. Detects mismatches between project docs
(README, docs/**/*.md, CLAUDE.md, bundled skills, Go comments) and the
actual code, fixes the DOCS only (never code logic), and auto-commits on
convergence. Alternating claude_code (opus-4-8) / claw (gpt-5.5)
reviewers, deterministic `streak_check` (two cross-family approvals), a
`scan_docs` footprint enumerator + `build_manifest` anchor verifier so
agents can't truncate the audit set. Runs on ANY repo; iterion is the
reference self-host case.

## 2026-06-14 — first dogfood + md_link scanner improvement (runs 019ec675, 019ec69f)

First recorded dogfood, on the real board ticket `c4043495` ("Align the
.bot/.iter documentation boundary"). Run in an isolated git worktree
(`--merge-into none`), store pointed at the operator's `.iterion` so the
run was visible in studio. Bot launched via standalone `iterion run` (not
the watchexec studio backend) and the install was a fresh static binary at
HEAD — both per the CLAUDE.md dogfood discipline.

- Status: **validated (with one real coverage finding, since fixed)**.
- Versions: bot 0.13.1 → **0.14.0** (this session) · iterion `e9148046`.
- Method: claude_code `claude-opus-4-8` + claw `openai/gpt-5.5`; isolated
  worktree; `--var doc_globs=CLAUDE.md,README.md,docs/**/*.md,pkg/cli/templates/*.bot,*.iter`
  `--var scope_notes="resolve .bot/.iter tension"` `--var bundle_self_path=bots/docs-refresh`.
- Result: **converged in 4 review iterations**, `$7.68`, ~126k tokens,
  ~27 min. Commit `e9520f11` on `dogfood/docs-refresh-boundary`.
  `.md`-only contract held; `prepare_commit` re-verified every code ref
  before committing (anti-façade discipline working).

### Value produced
- Caught + fixed **real drift**: `docs/secrets-reference.md` linked a dead
  path `pkg/auth/auth.go:GenerateRandomToken` — the function actually lives
  at `pkg/auth/password.go:118` (auth.go does not exist). Fixed, verified.
- `docs/bot-runs/whats-next.md` — clarified a local run-artifact path that
  read as a committed repo path.

### Finding (bot coverage gap) → FIXED this session
The bot **converged without resolving the ticket's headline item**:
`CLAUDE.md:3` still claimed "`.iter` / `.bot` — identical semantics" and
linked a **dead anchor** `README.md#iter-vs-bot` (the README heading was
removed; the CLI now rejects `.iter` outright — `unsupported workflow
extension`). The reviewers verify doc→**code** refs (symbols, CLI surface,
file paths under known roots) but nothing systematically audited
doc→**doc** internal links / `#heading-anchors`. `FILE_RE` in
`build_manifest` only matches paths under known roots (so bare `README.md`
slipped through) and never captured the `#anchor` fragment. The
`dead_link` taxonomy existed but had no deterministic candidate feeder.

**Fix (v0.14.0, `build_manifest`):** added an `md_link` anchor kind that
extracts `[text](path#anchor)` links and verifies BOTH the target file's
existence AND, for `.md` targets, the `#heading-anchor` (GitHub-slug
match: lowercase, strip non-`[\w\s-]`, spaces→hyphens, strip leading/
trailing hyphens to handle emoji headings; line anchors `#Lnn` skipped).
Drifted `md_link`s flow through the existing candidate pipeline at high
priority; `doc-mismatch-taxonomy.md` now points `md_link` → `dead_link`
(`anchor_kind: external`). Validated standalone over the full 153-doc tree
(**764 verified / 16 drifted, 0 false positives** after the slug fix), and
in a real scoped re-run (019ec69f) `build_manifest` flagged exactly the two
dead anchors (`CLAUDE.md:3` + `docs/examples.md:12` → `README.md#iter-vs-bot`,
`drifted_anchors: 2` of 288, zero FP). The scanner is generic — dead
internal links/anchors are a universal doc-drift class, not iterion-specific.

### Engine hardening
- Filed board ticket **`d8e8dde1`**: every claude_code node with schema +
  tools emits `tool_error: No such tool available: StructuredOutput` — the
  agent (behaving natively, as the adaptivity work intends) reaches for the
  SDK's `StructuredOutput` tool, which iterion never registers, wasting its
  Pass-1 final turn (`raw_output_len: 0`) before the unconditional Pass-2
  formatting round-trip. Cosmetic + one wasted LLM round-trip per such node
  across all loop bots. Fix sketch in the ticket: register a capturing
  `StructuredOutput` tool via the same `WithHook(HookPreToolUse)` +
  short-circuit pattern iterion already uses for `ask_user`
  ([claude_code.go:364](../../pkg/backend/delegate/claude_code.go)) — would
  eliminate Pass-2. Deferred (core machinery, needs its own tested change).
- Side: closed a **stale "ready" board ticket** (`native:21065752`, Revi
  "scan_shards.go:458 blocks until shard timeout") — already fixed on HEAD
  by `59cfedcc` + covered by `TestAwaitTerminal_PreDispatchFailureDoesNotHang`
  (passes). A dispatch would have wasted a run on an already-fixed bug.

### Lessons for next run
- **Cost**: `$7.68` to fix 2 lines of incidental drift is high — the 80%
  coverage gate over a **114-file** footprint makes every reviewer pass
  heavy. For a focused ticket, scope `doc_globs` tightly (a 3-file scope
  re-run cost a fraction). `scope_notes` is only a HINT; the mandatory
  full-footprint coverage dominates, so a reviewer can converge on
  incidental drift while leaving the operator's stated focus untouched.
  Consider weighting `scope_notes`-named files into the coverage gate.
- The `md_link` scanner now closes the dead-anchor class; re-run the
  original `c4043495` scope to land the CLAUDE.md:3 / examples.md fixes.

## 2026-06-14 — synthetic clone-validation + 2nd real-bot C082 proof (run 019ec58a)

A second, independent dogfood from the **C082 board-emit** session (parallel
to the real-board run above). Purpose was narrower: confirm Doki's machinery +
convergence on a clean iterion **clone** and, incidentally, exercise the C082
sandboxed-board fix end-to-end on a real catalog bot. (Lower-value target than
the real-board run above — kept for the C082 proof + the gitignore finding.)

- Status: **validated.** Converged with **zero** false fixes (the audited docs
  were accurate).
- Method: dedicated worktree studio :4899 (C082 worktree binary, forfait env),
  clean iterion clone; `doc_globs=docs/resume.md,docs/routers.md`,
  `code_scope_globs=pkg/store/**/*.go,pkg/runtime/**/*.go,pkg/dsl/ir/**/*.go`,
  `merge_into=none`. claude_code/opus + claw `gpt-5.5` forfait.
- Result: **converged in ~2 rounds to a cross-family double-approval**, no
  oscillation. reviewer_gpt audited 18 symbol refs in `docs/resume.md`,
  confirmed them in the Go code, and concluded "No documentation changes
  needed" → `commit_changes` a correct no-op. Correct verification, no
  hallucinated drift.
- **C082: confirmed on a 2nd real catalog bot.** `prepare_commit` (sandboxed
  claude_code, board.create cap) invoked `mcp__iterion_board__create_issue`
  twice through the C082-fixed HTTP transport → board 0→2, real native ids.
  Independent of the minimal C082 validation bot — proves the fix works in a
  real bot.
- Findings:
  1. **`.claude/skills/` runtime mirror not gitignored — FIXED** (`.gitignore`
     `.claude/skills/` + `.docs-refresh-cache.json`). The engine mirrors
     `<bundle>/skills/*.md` into `<workspace>/.claude/skills/` at run start; it
     was uncovered, so it shows as `?? .claude/` and can be swept into a code
     bot's commit (later confirmed live: Bmady's commit included the mirror on
     a clone without this fix). Broader runtime-level exclusion is tracked.
  2. **Empty `code_scope_globs` → every symbol "unverifiable"** (a first
     attempt with `doc_globs` only marked all 22 symbols unverifiable). Always
     pass `code_scope_globs`; an empty default arguably should mean "scan the
     workspace."
  3. Same `StructuredOutput` tool-error as ticket `d8e8dde1` above (non-fatal).
- Lessons: pass `code_scope_globs`; the bot is safe (no false fixes) +
  doctrine-compliant (neutral cache path, flags code issues to the board).
