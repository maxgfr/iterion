# 2026-06-23 — GLM-5.2 dogfood campaign (Wave 1 + Wave 2 + integration)

Cross-bot campaign record. Per-bot detail lives in each `docs/bot-runs/<bot>.md`
(Wave-1 entries dated 2026-06-22) and in the commit messages on `main`.

## What it was

Battle-test the catalog bots on iterion itself to improve iterion / claw / the
bots, on a **z.ai/GLM-5.2 + Anthropic-forfait** stack (operator's request to use
z.ai to save Anthropic credits + compare). Stable studio on :4891 (no watchexec),
static binary, both providers exercised.

## Provider stack + the failover event (central finding)

- `.env` flipped to forfait + z.ai/glm-5.2 for all `claude_code` nodes; gpt-5.5
  stays OpenAI/ChatGPT forfait. glm-5.2 validated end-to-end (id confirmed newest,
  2026-06-13, **1M context** — not 200K).
- **The z.ai 5h rate-limit cap tripped mid-campaign** → manual failover to
  Anthropic/opus (resume from checkpoint, zero work lost). This **proved in
  production** the need for automatic per-provider+model failover.

## Bots run (10 runs)

Wave 1: **Nexie** ✅ (roadmap + 5 tickets, incl. the failover + registry tickets),
**Adry** ✅ (13 ADRs 029-041), **Evoly** ⏸ (parked: gpt-5.5 forfait context-overflow
in review fan-out), **Seki** ⏹ (deepsec 37 findings; triage stalled on gpt-5.5).

Wave 2 (both providers à fond): **Featurly** ×2 ✅ (provider+model failover feature
`9493f96a2`; dynamic model-spec registry `970687f58`), **Fini** ✅ (GLM caps
`9f46f4d7e`; `iterion models` CLI `4250d8c1c`), **Renovacy** ❌→fixed,
**Doki** (oscillated→fixed), **Seki** re-run (triage-on-glm validated the stall fix).

## iterion/claw improvements produced + INTEGRATED on main (31 ahead of origin, local)

Features (built BY the dogfood bots, from findings the dogfood surfaced):
- **Per-element provider+model failover chain** `provider: "zai:glm-5.2,anthropic:claude-opus-4-8"` (Featurly) — ADR-004 + docs.
- **Dynamic model-spec registry** (Featurly) — fetch models.dev + cache (~/.iterion, TTL) + curated fallback (glm-5.2=1M); ADR-042. `iterion models` CLI to inspect (Fini).

Engine / security / reliability fixes:
- **worktree:auto BY DEFAULT** (`6569460d5`) — compile default + git-repo guard; **root-cause fix** for non-worktree bots dirtying the live checkout. lint+test+e2e green.
- **#2 SSRF clone-host guard** (`2303f2644`) + **#3 shard-id collision** (`dacd3076f`) — from Seki/deepsec findings.
- **glm-5.2 context = 1M** (`a3c4acf2f`); **missing-field structured-output retry** in the engine (part of Seki fix `ab6c0ab7e`).

Bot fixes (all `iterion validate` + unit-validated by isolated subagents):
- **Seki** `ab6c0ab7e` — triage/voters use the failover chain (glm-5.2 1M → opus fallback) + tolerate transient missing-field. (Elegant loop: the failover feature the cap *proved necessary* now hardens Seki.)
- **Renovacy** `8a86e8bb3` — keep toolchain caches (GOPATH/GOMODCACHE/CARGO_HOME…) out of the worktree + crash-proof commit-prep (256MB buffers + cache-excluding pathspecs).
- **Doki** `3b6531038` — enforce asymptote: symmetric streak gate + `cumulative_dismissed_pairs`/`cumulative_pushback` (do-not-relitigate).

Plus: 13 ADRs (Adry `c9402ac12`), Wave-1 bilans (`1f0e4b2cd`), recovered prior docs-refresh link fixes (`847bc57c5`).

## Findings (the dogfood's diagnostic value)

1. **Auto provider+model failover needed** → BUILT (the campaign's centerpiece).
2. **glm-5.2 structured-output reliability < opus** on complex review schemas
   (Fini's reviewer hit `missing required field` repeatedly on glm-5.2, even with
   the new missing-field retry; completed on opus). → motivates generalizing the
   failover chain to the **vibe bots' reviewers** (feature-dev/feature-gap-fill/
   branch-improve-loop/whole-improve-loop) — **recommended follow-up, not yet done**.
3. **gpt-5.5 forfait context-overflow** in review fan-outs + sec triage (Evoly, Seki).
4. **Non-worktree bots dirtied the live checkout** (a prior docs-refresh left
   uncommitted edits; the "mystery main edits") → fixed by worktree:auto default.
5. **Inconsistent rate-limit handling** (graceful `acknowledge_recovery` pause vs
   hard `fail_resumable`) across node types.
6. glm-5.2 survey verbosity (Adry ~900 tool-calls), deepsec line-drift false-positives,
   gitleaks flags gitignored `.env*` backups, `ITERION_SEC_AUDIT_BACKEND=claude_code`
   leaks gpt-voters onto claude_code (needs MODEL+MODEL_CLAUDE both set).

## State

All work on local `main` (`4250d8c1c`, 31 ahead of origin, **unpushed**), built +
`go vet` + unit + e2e green. The operator's 14 unpushed bot-invocations commits are
preserved underneath. Nothing pushed; fully reversible.
