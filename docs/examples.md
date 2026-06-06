[← Documentation index](README.md) · [← Iterion](../README.md)

# Examples

The [`examples/`](../examples/) directory ships a curated set of
proven, productized bots and one actively-developed workflow.

> **Extensions.** Iterion accepts both `.iter` and `.bot`. We use
> `.iter` for raw DSL or work-in-progress workflows and `.bot` for
> productized operational bots — workflows meant to run unmodified
> against real systems with human gates, mitigation steps and reports
> (see [`.iter` vs `.bot`](../README.md#iter-vs-bot)). The parser,
> runtime, and editor treat them identically.

## 🤖 Productized bots (folder-per-bot under [`examples/`](../examples/))

All bots below are also wired as embedded dispatcher assignees by
[`pkg/cli/dispatch_defaults.go`](../pkg/cli/dispatch_defaults.go), so
`iterion dispatch` (with no config) recognises their names out of the box.
See the catalogue and decision tree in
[`bots/whats-next/skills/iterion-bot-catalog.md`](../bots/whats-next/skills/iterion-bot-catalog.md).

| Persona | Bot | Description |
|---|---|---|
| 🧭 Nexie | [`whats-next/`](../bots/whats-next/) | Operator-loop bot: explore → elicit → roadmap → materialise the chosen `next_action` as kanban issues; pairs with `iterion dispatch` |
| 🛠️ Featurly | [`feature_dev/`](../bots/feature_dev/) | Self-driven feature development bot: plan → implement → review → refine loop with judge gates |
| 🌿 Billy | [`branch_improve_loop/`](../bots/branch_improve_loop/) | Branch-scope variant of the alternating improve loop with auto-commit between iterations |
| 🌍 Willy | [`whole_improve_loop/`](../bots/whole_improve_loop/) | Alternating Claude/GPT review-and-fix pattern with cross-family streak detection (whole-repo scope) |
| 📚 Doki | [`docs-refresh/`](../bots/docs-refresh/) | Detect & fix doc/code drift across README, `CLAUDE.md`, and `docs/**/*.md` (alternating Claude/GPT review with a mechanical coverage gate) |
| 🔎 Revi | [`code_review/`](../bots/code_review/) | Read-only cross-family code review; publishes findings to the native board (no fix, no commit) |
| 🛡️ Seki | [`sec-audit-source/`](../bots/sec-audit-source/) | Source-code SAST audit (gitleaks/trivy/semgrep/gosec/bandit) with per-repo cross-run FP memory — see [docs/security-bots.md](security-bots.md) |
| 📦 Depsy | [`sec-audit-deps/`](../bots/sec-audit-deps/) | Supply-chain malware/CVE audit on installed deps (npm/pip/go/…) + host-wide package cache — see [docs/security-bots.md](security-bots.md) |

## 🚧 In active development

| Persona | Bot | Description |
|---|---|---|
| ⬆️ Renovacy | [`secured-renovacy/`](../bots/secured-renovacy/) (`.botz` bundle) | Autonomous, security-aware dependency upgrades for any stack (yarn/npm/pnpm/pip/poetry/uv/cargo/go/bundler/composer/maven). Run via `iterion run bots/secured-renovacy/` or against the packed archive `iterion bundle pack bots/secured-renovacy && iterion run bots/secured-renovacy.botz`. |

## 📦 Archived

Older `.iter` workflows that were useful at one time but are no longer
maintained have moved to [`.archive/examples/`](../.archive/examples/).
They are not embedded in the binary and are kept only for historical
reference and as fixtures for the test suite.
