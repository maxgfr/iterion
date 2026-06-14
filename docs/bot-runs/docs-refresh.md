# docs-refresh (Doki) — dogfood bilan

Index + template: [README.md](README.md). Newest first.

## 2026-06-14 — first validated run + 2nd real-bot C082 proof (run 019ec58a)

- Status: **validated.** Machinery, convergence, real verification, and board
  emit all work; correctly made **zero** doc edits (the audited docs were
  already accurate — no false fixes).
- Versions: iterion branch `c082-board-emit` (C082 worktree binary) · docs-refresh
  current.
- Method: dedicated worktree studio :4899 (worktree binary, forfait env),
  scanning a clean iterion clone; `doc_globs=docs/resume.md,docs/routers.md`,
  `code_scope_globs=pkg/store/**/*.go,pkg/runtime/**/*.go,pkg/dsl/ir/**/*.go`,
  `merge_into=none`. Backends: claude_code/opus (reviewer_claude, prepare_commit)
  + claw `openai/gpt-5.5` forfait (reviewer_gpt).
- Result: **converged in ~2 rounds to a cross-family double-approval**
  (reviewer_claude + reviewer_gpt both `approved`), `Run finished`, no
  oscillation (asymptote held). reviewer_gpt audited 18 symbol refs in
  `docs/resume.md` (LoopCounters, RoundRobinCounters, FailNode, delegate.Task,
  WithForceResume, …), confirmed them against the live Go code, and concluded
  "No documentation changes needed" → `commit_changes` was a correct no-op (no
  new commit; clone HEAD unchanged).
- Value: (a) **correct verification with no false fixes** — the whole point of a
  docs bot is to NOT hallucinate drift; it didn't. (b) Emitted **2 board issues**
  via the C082 transport, one genuinely useful (see findings).
- C082: **confirmed on a 2nd real catalog bot.** `prepare_commit` (sandboxed
  claude_code, board.create cap) invoked `mcp__iterion_board__create_issue` twice
  through the fixed HTTP transport → board 0→2, real native ids. Independent of
  the minimal validation bot — proves the fix works in a real bot even though the
  Seki re-run couldn't reach its own `report_card`.
- Findings:
  1. **`.claude/skills/` runtime mirror not gitignored — FIXED.** The engine
     mirrors `<bundle>/skills/*.md` into `<workspace>/.claude/skills/` at run
     start; iterion's `.gitignore` covered `.claude/*.local.json` +
     `scheduled_tasks.lock` but not the mirror, so it shows as `?? .claude/` in
     any target's git status and can be swept into a code bot's commit. Added
     `.claude/skills/` (and Doki's neutral `.docs-refresh-cache.json` cache) to
     `.gitignore`. Broader runtime fix (gitignore/exclude the mirror in ANY
     target, not just iterion) is a tracked engine item.
  2. **Empty `code_scope_globs` → every symbol ref "unverifiable."** The first
     attempt (run 019ec589, doc_globs only) marked all 22 resume.md symbols
     `unverifiable` ("no match in code_scope_globs") → a fast but low-value
     converge-with-no-verification. Lesson: always pass `code_scope_globs` for
     real verification; an empty default arguably should mean "scan the
     workspace" (usability follow-up).
  3. Doki also flagged my uncommitted `bots/sec-audit-source/main.bot` edit in
     the clone as "orphaned in working tree" — an artifact of my test setup (I
     copied the fixed bot in without committing), not a repo bug, but it shows
     the bot is perceptive about working-tree hygiene.
- Engine hardening this run: `.gitignore` `.claude/skills/` + `.docs-refresh-cache.json`.
- Lessons for next run: pass `code_scope_globs`; the bot is safe (no false
  fixes) + doctrine-compliant (neutral cache path, flags code issues to the
  board instead of editing code).
