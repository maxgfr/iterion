[← Docs](../README.md)

# Bot runs — validation & knowledge base

This directory is the **committed knowledge base** for iterion's catalog bots.
Every time a bot is dogfooded with a real run, the operator distils the outcome
into a dated **bilan** in `docs/bot-runs/<bot>.md` (named by bot **directory**,
not persona). The next person to launch that bot reads its file first — what it
caught, what it missed, what to change, and which engine bugs the run surfaced.

A bilan is durable, reviewable in a PR, and shows up in `git log`. It is one of
**three complementary knowledge channels** — do not confuse them:

| Channel | Where | Scope | Lifetime |
|---|---|---|---|
| **Workspace memory** | `~/.iterion/projects/<key>/memory/` (gitignored) — see [memory-and-knowledge.md](../memory-and-knowledge.md) | per-operator scratch, "what did we learn this session" | local to one machine/operator |
| **Board issues** | native kanban (`.iterion/`, gitignored) | open tasks / findings to act on | until closed |
| **Bilans (this dir)** | `docs/bot-runs/<bot>.md` (committed) | durable lessons the next operator must read before launching the bot | forever, in git history |

Cross-bot lessons (Goodhart, façade patterns, asymptote rules) live in
[workflow_authoring_pitfalls.md](../workflow_authoring_pitfalls.md), not here —
this directory is **per-bot**.

## Bilan template

Append one dated section per run to `docs/bot-runs/<bot>.md` (newest first):

```markdown
## YYYY-MM-DD — <short label> (run <id-prefix>)
- Status: validated | partial | failed
- Versions: bot <manifest version> · iterion <git sha>
- Method: backend(s)/model(s), budget, key --vars, flags (--merge-into, post_to_board, sandbox image)
- Result: converged? iterations, cost $, duration, where commits landed (branch/sha)
- Value: the high-value thing it actually produced (or: low value + why)
- Findings / misses: what the bot caught or missed
- Engine hardening: iterion bugs found → commits/ADRs
- Lessons for next run: what to change (vars, prompt, scanner, skill)
```

The run artifacts (`.iterion/runs/<id>/`) are gitignored, so the bilan is the
only committed trace. Regenerate the full chronological run report any time with:

```sh
iterion report --run-id <id> --output /tmp/<bot>-<id>.md
```

and cite the run-id in the bilan so it can be reconstructed.

## Index

Persona → bot directory, with current validation status. Add the link when the
first bilan for a bot lands.

| Persona | Bot | Kind | Bilan |
|---|---|---|---|
| Nexie | `whats-next` | orchestrator / board triage | [whats-next.md](whats-next.md) |
| Willy | `whole-improve-loop` | whole-repo review-fix loop | _not yet_ |
| Billy | `branch-improve-loop` | branch-scoped review-fix loop (commits) | [branch-improve-loop.md](branch-improve-loop.md) |
| Featurly | `feature-dev` | one-shot feature dev + review loop | _not yet_ |
| Doki | `docs-refresh` | docs↔code convergence loop | _not yet_ |
| Revi | `review-pr` | read-only cross-family reviewer | _not yet_ |
| Revi (converse) | `revi-converse` | conversational PR follow-up | _not yet_ |
| Seki | `sec-audit-source` | source SAST audit | [sec-audit-source.md](sec-audit-source.md) |
| Depsy | `sec-audit-deps` | supply-chain SCA audit (scaffold) | _not yet_ |
| Renovacy | `secured-renovacy` | dependency upgrade pipeline | _not yet_ |
| Bmady | `bmady` | BMAD multi-persona human-gated delivery | _not yet_ |
| Devy | `devbox-setup` | devbox.json bootstrap | _not yet_ |
