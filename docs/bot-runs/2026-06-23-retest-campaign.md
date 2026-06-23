# 2026-06-23 — Catalog re-test campaign + Verified Action delivery

Goal (operator): re-test **every** catalog bot until each runs reliably on the
first-class lane (Anthropic opus / OpenAI gpt-5.5 forfait), fixing on failure;
and, separately, have Featurly build the ADR-044 "Verified Action" pattern.

This is the consolidated record; the headline runs also have per-bot entries
([feature-dev.md](feature-dev.md), [sec-audit-source.md](sec-audit-source.md)).

## Headline deliverable — Verified Action pattern (ADR-044)

Featurly built the entire ADR-044 synthesis in one supervised run (019ef38d):
DSL (parser/AST/IR/unparse/EBNF) + runtime `executor_verified_action.go` (342-line
escalation ladder) + unit & e2e tests + docs + a demo on a commit node. Branch
`iterion/run/019ef38d` @ `79f9111ed`, **builds + all VA tests green** (verified
independently). NOT yet integrated to main (overlaps the same-day Renovacy
commit-node fixes; needs review + overlap resolution). See feature-dev.md.

## Re-test scoreboard (first-class lane)

| Bot | Result |
|---|---|
| Doki (docs-refresh) | ✅ ×2 clean (convergence fix validated) |
| Seki (sec-audit-source) | ✅ clean end-to-end on opus (run 019ef389) |
| Nexie (whats-next) | ✅ clean to its priority-elicitation human gate |
| Revi (review-pr) | ✅ clean; caught a real regression in my own work |
| Evoly (evolve) | ✅ clean to its human gate |
| ReArchi (adr-rechallenge) | ✅ clean to human_decision (needs `--var workspace_dir`) |
| Bmady (bmady) | ✅ clean to its first BMAD gate |
| Fini (feature-gap-fill) | ✅ full plan→implement→review→commit; builds (real CLI fix) |
| Devy (devbox-setup) | ✅ clean |
| Willy (whole-improve-loop) | ✅ clean, **isolated** (worktree:auto default made it safe) |
| Renovacy (secured-renovacy) | ✅ commit-path validated via 27 clean commits (run cancelled for churn, not failure) |
| Billy (branch-improve-loop) | ✅ machinery validated with `--var chunk_dir=/tmp` (see finding) |
| Depsy (sec-audit-deps) | ⚠ blocked by the shared ~/.iterion-scratch finding (low-value SCA scaffold) |

revi-converse excluded (needs a live forge MR).

## Fixes committed this campaign

- **secured-renovacy** — the commit path was a minefield (ADR-044's living proof),
  fixed 5× then hardened: `:!`→`:(exclude)` pathspec; gitignore-filtered explicit
  add; plain `git add -A` (robust primitive) over brittle exclude-pathspecs;
  dropped `:(exclude)` from a Node `execSync` (the `(` broke `sh -c`); and a
  cache-unstage guard after `git add -A` (Revi finding).
- **sec-audit-source** — triage/voters defaulted to glm-5.2-first (GLM
  structured-output reliability gap); re-defaulted to **opus first-class**, glm
  chain now opt-in via `ITERION_SEC_AUDIT_PROVIDER_CHAIN` (ADR-043). `40a61ce97`.
- **adr-cartograph** — review-loop convergence hardened (Doki-style streak +
  steady-state escape) to stop opus oscillation into the fail backstop. `b5fc5be45`.
- **ENGINE: docker E2BIG** — oversized `sh`/`bash -c` commands now stream via
  stdin instead of the argv (`faf11a872` + `836e21094`). Unblocked Seki's
  `majority_verdict`; hardens every sandboxed tool node with large inter-node input.

## Findings (open)

1. **Shared `~/.iterion` scratch/cache failure (Depsy + Billy).** Both write
   scratch/cache under the host-mounted `~/.iterion/projects/<key>/…` from
   **inside the sandbox**, where the container runs as a non-host user that
   cannot write the jo-owned (700/755) host store → `PermissionError` /
   `Permission denied`. `--sandbox-host-state=none` is NOT a workaround (it strips
   the `~/.codex` OAuth mount → claw loses gpt-5.5). Fix: default these paths to a
   sandbox-writable location (worktree `${PROJECT_DIR}/…` or `/tmp`), per the
   repo-agnosticism rule (no caching under `~/.iterion`). Billy unblocked live with
   `--var chunk_dir=/tmp`; Depsy needs the same for its `cache_dir`.
2. **ReArchi `${PROJECT_DIR}` in a var-default** (`workspace_dir: string =
   "${PROJECT_DIR}"`) is not substituted when it arrives via the var into a
   command — `load_adr` got a literal `${PROJECT_DIR}`. Works with explicit
   `--var workspace_dir=…`. ReArchi-specific (Devy, same idiom, ran clean).
3. **Renovacy `max_packages_per_run` doesn't bound total commits** — a cap=15 run
   produced 27 commits across 60+ candidate selections / 224 upgrade attempts (the
   patch/family/solo paths appear counted separately). Efficiency/cap fix.

## Operational notes

- Two Anthropic account switches mid-campaign; every interrupted run was
  `failed_resumable` and resumed cleanly from checkpoint (zero lost work).
- LESSON: never `for p in $(pgrep …); do kill` in the same shell driving the
  campaign — it self-TERMs (hit twice). Kill by captured PID or leave orphans.
- LESSON: no backticks inside a `#` shell comment in a `.bot` `command:` backtick
  block — they close the DSL string (broke Renovacy validate once; amended).

## State

Worktree clean; main is local/unpushed (≈47 commits ahead of origin). Builds +
sandbox/model/dsl tests green. Pending operator steer: integrate Verified Action +
Fini branches; triage the Seki SSRF-TOCTOU finding; 2nd-clean confirms for the
heavily-fixed bots; fix findings 1–3; push.
