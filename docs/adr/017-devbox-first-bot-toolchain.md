# ADR-017: devbox-first bot toolchain — bot tools vs project devbox, project-devbox execution, and a devbox-setup bot

- **Status**: Proposed (design + feasibility prototype; staged rollout below)
- **Date**: 2026-06-08
- **Authors**: devthejo
- **Code (today, partial)**: [pkg/backend/tool/claw_builtins.go](../../pkg/backend/tool/claw_builtins.go)
  (`BashExtraEnv`, the `devbox run --` project-managed-toolchain hook),
  [pkg/runtime/sandbox_mounts.go](../../pkg/runtime/sandbox_mounts.go)
  (devbox/npm state persistence via `host_state`),
  [pkg/sandbox/devcontainer/](../../pkg/sandbox/devcontainer/) (current
  image/devcontainer provisioning), `bots/*/main.bot` (inventory already
  lists `devbox.json` as a manifest)
- **Prototype**: [bots/sec-audit-source/devbox.json](../../bots/sec-audit-source/devbox.json)

## Context

Bots need tools: scanners (gosec, gitleaks, semgrep, trivy), language
runtimes (Go, Node, Python), and project-specific build/test stacks. Today
those tools are **baked into per-bot sandbox images** (e.g.
`iterion-sandbox-sec`). That model hurts:

- Adding one tool (deepsec) meant rebuilding + republishing the image; the
  published image lags the bot; a default-on feature (`enable_deepsec=true`,
  ADR-adjacent) **degrades on every stock-image run** until the image is
  rebuilt. Per-bot images are hand-maintained and don't scale across bots.
- iterion already ships devbox + nix in the sec image, has a `devbox run --`
  toolchain hook, persists devbox state, and inventories `devbox.json` — but
  a code comment is explicit: **"devbox isn't first-class in the sandbox"**
  (root-owned `$HOME` breaks devbox writes). So we are half-way and stuck.

Crucially there are **two distinct toolchain needs that must NOT be
conflated**:

1. **The bot's own toolchain** — the scanners/tools the bot runs. Bot-owned,
   project-agnostic, identical regardless of target repo.
2. **The target project's toolchain** — to run the project's own
   `build` / `test` / reproducers / `playwright` e2e in the environment the
   project declares. The remediation ladder
   (`build_rung` / `regress_rung` / `reproduce_rung`) currently shells out
   with whatever is on `PATH`, **not** the project's declared toolchain —
   fragile (wrong Go/Node version, missing project deps → false build/test
   failures that sink real patches).

## Decision

Adopt a **two-tier devbox model**, plus a bootstrap bot.

### Tier 1 — Bot toolchain devbox (the bot's own tools)
- Each bot bundle ships a `devbox.json` (e.g. `bots/sec-audit-source/
  devbox.json`) declaring its **Nix-packaged** tools (gosec, gitleaks,
  semgrep, trivy, ripgrep, …), pinned.
- The sandbox provisions it from a **bot-controlled directory OUTSIDE the
  project tree** (the bundle / a sandbox tool dir), with its own
  `DEVBOX_*` env. Tools resolve through the existing `devbox run --` hook
  (or a PATH activated from that env).
- This **replaces the hand-maintained per-bot image**: a slim base
  (`nix` + `devbox` + `node`) + the bot's `devbox.json`. Adding a tool =
  edit `devbox.json`, no image rebuild; reproducible (Nix-pinned).

### Tier 2 — Project devbox (the target repo's tools)
- On run start the bot **detects the target project's `devbox.json`**
  (already inventoried). If present → `devbox install` once (warm it).
- Project commands run **through the project's devbox**:
  `cd <project> && devbox run -- <cmd>` — so `build_rung` / `regress_rung`
  / `reproduce_rung` and any e2e (playwright) use the project's **declared**
  toolchain (correct Go/Node/Python, project deps). Far more reliable than
  bare `PATH`.
- No project `devbox.json` → fall back to bare commands (today's behaviour),
  or to the Tier-1 languages, or run the Tier-3 setup-bot first.

### No-conflict rule (the critical separation)
The two environments are **isolated**: distinct project dirs, distinct
`DEVBOX_*` / Nix profiles. The bot's devbox lives **outside the project
tree**, so it:
- never edits/creates the project's `devbox.json` or `devbox.lock`,
- never pollutes the project's lockfile or collides on versions,
- can pin a **different** version of a shared package (e.g. the bot's Go for
  `semgrep-go` vs the project's pinned Go) with zero interference.

The project's devbox is used **install + run only** (never mutated) — the
sole writer of a project `devbox.json` is the Tier-3 setup-bot, and only
that bot, deliberately.

### Tier 3 — A dedicated "devbox-setup" bot
A new bot whose **sole mission**: generate/populate a repo's `devbox.json`
with everything it needs (detect languages/runtimes/build+test tools +
e2e/playwright deps → synthesise a pinned `devbox.json` (+ `devbox.lock`) →
optionally open a PR). Analogous to `docs-refresh`, but for the dev
environment. This makes Tier 2 **universal**: even repos with no devbox get
one, so every downstream bot can run build/test/e2e in a reproducible env.

### Caching / network / cold-start (the make-or-break)
- **Persist the Nix store + devbox cache across runs** (mount `/nix` + the
  devbox cache; build on the existing `host_state` mounts) AND **fix the
  root-`$HOME` friction** so devbox is first-class. Warm cache → fast
  provisioning; this is the single biggest enabler.
- **Network**: the sandbox proxy allowlist must include the Nix substituter
  (`cache.nixos.org`) + the devbox cache. Cold provisioning needs egress;
  on a blocked/flaky network it degrades with a clear error (same failure
  class we hit installing deepsec).
- **Cold-start**: first run on a cache miss is slow (Nix download);
  acceptable with a persistent warm cache. For CI, pre-warm the store in the
  base image or a shared cache volume.

## Consequences

**Positive**
- Per-bot tooling is declarative, reproducible, and **lives with the bot** —
  no per-bot image churn; new tool = `devbox.json` edit.
- Solves the "tool not in the published image" class generally (deepsec was
  the trigger).
- Tier 2 makes the remediation ladder run the project's **real**
  build/test/e2e → far fewer false failures sinking valid patches.
- The setup-bot bootstraps reproducible envs for any repo.

**Negative / costs**
- More runtime moving parts (provision at run): cache miss / network blip =
  slow or failed provision — mitigated by persistent cache + allowlist +
  graceful degrade.
- Requires making devbox first-class in the sandbox (root-`$HOME` fix) —
  engine work.
- **Non-Nix tools** (npm-only, e.g. deepsec) still need their own installer:
  Tier-1 devbox supplies `node`, but the package install stays `npm i -g`
  (keep baking or a declared init-hook for those — devbox is not a fix for
  npm-package cost).

## Alternatives considered
1. **Status quo** (bake everything per-bot image) — reliable but
   high-maintenance, lags, doesn't scale across bots/tools.
2. **devbox as a runtime fallback only** (image-first, devbox if a tool is
   missing) — lighter, but keeps two mechanisms and yields neither the
   declarative/reproducible win nor Tier 2 (project execution).
3. **Pure Nix flakes** (no devbox) — more powerful, steeper ergonomics;
   devbox is the layer the team already uses and ships in the image.

## Rollout (staged, each independently shippable)

**Verified already in place (2026-06-08) — step 1 is mostly done:**
- First-class `$HOME`: `pkg/runtime/sandbox_mounts.go` already lays a
  uid-owned writable tmpfs HOME (+ nested `.cache/go-build`, `go/pkg/mod`
  binds). The prototype confirmed `$HOME` writable as uid 1000 and
  `devbox install` succeeds — the "not first-class" comment predates this;
  the residual gap is the store, below.
- Substituter allowlist: `cache.nixos.org` / `channels.nixos.org` /
  `releases.nixos.org` + `registry.npmjs.org` are already in
  `pkg/sandbox/netproxy/preset.go`.

**Remaining — persistent store (the only real gap; prototype: cold 16 min vs
warm 0 s).** A per-run container's `/nix/store` + devbox profile are
ephemeral, so each run re-provisions. Two paths:
- **(A) Pre-warm in the image** — `devbox install` the bot's `devbox.json`
  at image build so the closure is baked. Simple, deterministic; but a
  per-bot-image rebuild, and **redundant for today's sec image** (its
  scanners are already baked). This is the path for a *slim* base
  (`base + devbox + node`, image built FROM `devbox.json`).
- **(B) Named `/nix` docker volume** (seeded from the image on first mount)
  — warms Tier-1 *and* Tier-2 with no rebuild. Footguns to handle before
  shipping: ~596 MB first-seed copy; concurrent runs sharing the store (nix
  db lock); k8s can't `host_state` (gate docker-only); volume
  lifecycle/prune; and the big one — **a stale volume shadows a rebuilt
  image's `/nix`**, so the volume must be keyed/invalidated on the image
  digest. **IMPLEMENTED** (commit `806f53f6`, opt-in
  `ITERION_SANDBOX_PERSIST_NIX`, default OFF → merge-safe): the docker driver
  mounts a digest-keyed named volume at `/nix`. Disambiguation confirmed the
  volume is **viable** — `nix-store --verify` + `devbox` both succeed on a
  seeded volume; the earlier cold-install rc=1 was the flaky network, not the
  volume. Remaining for full prod: concurrency review (shared store nix-db
  lock), a volume-prune reaper for stale image digests, and an end-to-end
  warm-reuse run on a stable network.

**Then:**
1. **Tier 2**: detect the project `devbox.json` → `devbox install` on start
   → route `build_rung`/`regress_rung`/`reproduce_rung`/e2e through
   `devbox run --` when present.
2. **Tier 3**: build the `devbox-setup` bot.
3. **Slim**: once (A)/(B) is solid, drop the baked scanners and rely on the
   `devbox.json` closure.
4. **Non-Nix**: keep deepsec baked / init-hook (Tier-1 devbox gives node;
   deepsec install stays npm).

## Prototype results (2026-06-08, `iterion-sandbox-sec:edge`, uid 1000)

Provisioned `bots/sec-audit-source/devbox.json` (gosec, gitleaks, semgrep,
trivy, ripgrep) in the sec image (already has nix 2.34.6 + devbox 0.17.2 +
a 596 MB baked `/nix/store`), as the `devbox` user:

| Metric | Result |
|---|---|
| `$HOME` writable as uid 1000 | **yes** — `devbox install` works in a plain container; the "not first-class" friction is **sandbox-mount-specific** (root-owned `$HOME` via mounts), not inherent |
| Cold `devbox install` (store miss + network) | **~955 s (16 min)** — dominated by semgrep's Python closure (`python3.13-semgrep` + `mcp` + `pydantic` …) from `cache.nixos.org` |
| Warm `devbox install` (cached) | **0 s** |
| All 5 tools resolve via `devbox run -- <tool>` | **yes** (gosec, gitleaks, trivy, semgrep, rg) |

**Reading.** The model works end-to-end (declared → provisioned → resolved).
The decisive number is **cold 16 min vs warm 0 s**: a **persistent Nix
store is non-negotiable** — without it every run pays ~16 min. So the
rollout MUST pre-warm the store (bake the closure into the base image, or a
shared cache volume) so "cold" is effectively warm; then the
slim-base + per-bot `devbox.json` model gives reproducibility at ~0
provisioning cost. semgrep (Python) is the heavy item; the Go-binary
scanners are cheap. The `$HOME`-writable result also shows the "first-class
devbox" fix is a sandbox-mount detail, not a blocker.

## See also
- Prototype: [bots/sec-audit-source/devbox.json](../../bots/sec-audit-source/devbox.json).
- Existing devbox hooks: `claw_builtins.go`, `sandbox_mounts.go`.
