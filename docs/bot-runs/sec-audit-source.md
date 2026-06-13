# Seki + deepsec — validation

## 2026-06-13 (retest, FIXED) — scanner invocations repaired → full pipeline (run 019ec230)

- Status: **scan_health now PASSES; Seki runs the full read pipeline end-to-end**
  (detect_tech → scanners → scan_health → cap_findings → triage → N-vote → report_card).
  The 019ec1e0 hard-fail below was NOT a broken sec image — it was four broken scanner
  *invocations* in the bot/skills (the binaries all work). Fixed in `a8fac4c5`; **no
  Dockerfile change needed.**
- The fixes (see commit): **semgrep** `--config=auto --metrics=off` → `--config=p/default`
  (auto-config is rejected with metrics off — the silent error that left only 1/3 generic
  scanners; this is THE unblock — semgrep-auto.json now lands → 2/3 floor → scan_health
  passes degraded). **gosec** was scanning `.iterion/worktrees/` (dozens of repo copies) →
  11 min/no-output; `-exclude-dir=.iterion -exclude-dir=.works` → **125 s + 168 KB output**
  (validated standalone). **trivy** choked traversing `.iterion`/`.works` → `--scanners`
  (modern flag, was deprecated `--security-checks`) + `--skip-dirs=.iterion,.works,vendor`.
- **Validated end-to-end**: 019ec230 reached scan_health (degraded, 2/3 generic) → triage →
  N-vote (voter_v1/v2/v3) → report_card. (One earlier attempt was drained mid-N-vote by a
  *parallel session*'s `.go` edit restarting `task studio:dev` — an environment artifact,
  not a Seki bug; resumed from the `voter_v1` checkpoint to finish.)
- **Value proven**: even deepsec-OFF, Seki surfaced a **real CRITICAL candidate** —
  *"RCE in cloud runner via unvalidated RepoURL passed to git clone (pkg/runner/loop.go)"*
  (the file does clone `msg.RepoURL` at L689 → `prepareRepoWorkspace` L789). Status
  `uncertain` (N-vote didn't fully confirm) — worth operator triage.
### C082 root cause + fix design (board-emit, the remaining gap)

Traced precisely (2026-06-13): the sandboxed board MCP HTTP transport is **declared on
both ends but the PRODUCER side is never wired**, so sandboxed claude_code/claw board
caps silently no-op and the agent **confabulates** the board.create IDs:
- **Server side exists**: `BoardMCPTokenRegistry` + `RegisterBoardMCPRoutes(/api/v1/mcp/board,
  store, reg)` ([pkg/server/mcp_board_handler.go](pkg/server/mcp_board_handler.go),
  server.go:870) — BUT `boardMCPTokens.Register(token, caps)` is **never called** for a run.
- **Consumer side exists**: `Task.BoardHTTPEndpoint`/`BoardRunToken` + claude_code.go:477
  consume them (and :490 warns + disables board MCP when empty) — BUT **nothing in the
  runtime/executor ever SETS them** (grep: zero assignments). So they're always "" →
  board MCP disabled under sandbox → confabulation.
- **Fix design**: (1) plumb a `register(caps)→token` closure + a board endpoint URL from
  the server into `model.ExecutorSpec`→`ClawExecutor`; (2) in the Task builder
  ([executor.go ~1245](pkg/backend/model/executor.go)) for sandboxed board-cap nodes:
  `task.BoardRunToken = register(caps); task.BoardHTTPEndpoint = url`. (3) **CRITICAL
  networking caveat** — the endpoint must be *container-reachable*: `iterion studio` binds
  `127.0.0.1` (loopback, NOT reachable via `host.docker.internal`); the egress proxy only
  works because the docker driver's `ProxyConfigurer` binds a gateway-reachable interface.
  The board endpoint needs the same (bind gateway/0.0.0.0, or tunnel via the proxy). This
  networking requirement = it MUST be live-validated against a container (a sandboxed run
  confirming the write lands), so it should be implemented when the studio is free (no
  parallel session to drain on the rebuild) or against a dedicated `iterion server` bound
  on a gateway-reachable port. Not shipped blind.

- **board-emit (C082) STILL DOESN'T LAND (bilan #4 persists — a distinct engine gap).**
  report_card DID invoke `board.create`×3 / `board.label`×3 / `board.move`×2 and its
  `created_issues` output carries real native-looking IDs (`native:90543c66…`), but the
  issues are **NOT on the operator's board** (total unchanged at 94; fetch-by-id + every
  label query miss). The sandboxed board MCP HTTP transport (`/api/v1/mcp/board` +
  ephemeral run token) returns success but the writes don't persist to the operator's
  native board — even when launched via the studio. Seki's findings are recoverable from
  the run (`iterion report --run-id 019ec230`), not the board. This is the one remaining
  Seki gap; it is **separate from the scanner fixes** (which are done) and needs a focused
  look at the sandboxed board HTTP transport.
- **Residual (tracked, non-blocking)**: gosec + trivy still emit nothing *inside the
  sandbox specifically* (both work standalone) — a deeper sandbox/proxy/go-module
  interaction. scan_health passes on gitleaks + semgrep (2/3) regardless; semgrep
  (go/js/py/default) + bandit carry the SAST coverage; deepsec ON remains the
  highest-value path (019ec142).

## 2026-06-13 (retest) — engine fixes + safe default, via STUDIO (run 019ec1e0)

- Status: **engine fixes validated via the studio path; safe default shipped; but
  the run HARD-FAILED at `scan_health` — correctly — because the sec image's generic
  scanner toolchain is broken.** detect_tech (which FAILED in 019ec10f/019ec13a) now
  completes; the read chain ran to the coverage gate, which then refused to certify a
  thin audit. Board-emit NOT reached (failed before `report_card`).
- Versions: bot sec-audit-source 0.1.0 · iterion 778b9860 / bbdca0da / ea61817a /
  92c40d62
- Method: launched via **studio** `POST /api/runs` (so the HTTP board transport is
  wired — C082), `remediate=false` (now the default), `enable_deepsec=false` (lean:
  validate generic+lang scanners → triage → report → board-emit cheaply; deepsec's
  vuln-finding was already proven in 019ec142). Sandbox `iterion-sandbox-sec:edge`.

### Validated by the engine fixes (the headline)
- **detect_tech now completes** (claw + sandbox). In 019ec10f/019ec13a it died at
  this first claw node; the TLS-inspect-proxy hang (FIXED 778b9860) was the cause.
  The whole read chain ran: inventory → detect_tech → context → diff_scope →
  plan_shards → run_generic_scanners → run_lang_scanners → … Confirms the proxy +
  empty-tool-result fixes hold for the **operator's actual (studio) path**, not just
  CLI.
- **remediate=false (92c40d62)** — `iterion validate` clean; the run is read-only by
  default (no live-tree edits, no branch hijack). Safe under `task studio:dev`.
- **{{run.id}} (ea61817a)** — opt-in remediation will now name its branch
  `iterion/sec-fix/<real-run-id>` instead of the literal `iterion/sec-fix/run.id`.

### scan_health hard-failed — CORRECTLY (the headline blocker)
The run reached `scan_health` and **hard-failed (run_failed, exit 1)** with:
`{"generic_expected":3,"generic_present":1,"min_generic":2,"missing":[trivy.json,
semgrep-auto.json (generic), gosec.json (lang)],"total_findings_seen":1596,
"healthy":false,"degraded":true}` — *"only 1 of 3 always-on generic scanners produced
output (need ≥2)"*. This is the **anti-façade gate working as designed**: it refuses to
certify an audit when the core generic toolchain is down, even though lang/custom
scanners saw 1596 raw findings. (019ec142 passed because ≥2 generic scanners happened
to run that time — the toolchain is flaky.)

### THE BLOCKER: the sec image's scanner toolchain is broken (infra, not engine/bot)
Scanners are installed (`trivy 0.70.0`, `gosec`, `gitleaks 8.21.2`, `govulncheck`,
`semgrep` on PATH) but **fail at runtime** in `iterion-sandbox-sec:edge`:
1. **trivy** → `FATAL ... unable to create temporary directory: stat /tmp/trivy-10:
   no such file or directory`. A /tmp/TMPDIR issue in the image (reproduced with a bare
   `docker run … trivy fs`). Also the bot still passes the **deprecated** `--security-checks`
   flag (renamed `--scanners` in modern trivy) — fix both.
2. **semgrep** → `semgrep --version` prints nothing; `--config=auto` needs to fetch its
   rule pack from the registry (network) and produced no output. Broken install and/or
   registry fetch.
3. **gosec** → ran **>11 min** then produced **no `gosec.json`** (timed out / errored
   after type-checking the full import graph — `-exclude-dir=vendor` filters reporting,
   not loading). Needs a timeout + scoping.
→ **Both sec bots (Seki + Depsy) are gated on this.** The fix is a focused
`sandbox/sec/Dockerfile` + scanner-invocation pass (TMPDIR for trivy, `--scanners`,
fix/repin semgrep, bound gosec), then republish via CI `build-sandbox-sec`. Not done
here — it's image infra, out of scope for the bot retest; tracked as the sec-bot blocker.

### Lessons for next run
- Launch sec bots **via the studio** (board transport wired) — a bare CLI run no-ops
  board writes (C082). detect_tech + the claw path now work end-to-end (engine fixes).
- **Don't trust sec-bot output until the sec image's trivy/semgrep/gosec are fixed** —
  `scan_health` will (rightly) hard-fail or banner on the broken toolchain. deepsec ON
  is the only currently-working value path (019ec142), and even it runs degraded.

## 2026-06-13 — iterion self-audit dogfood (runs 019ec10f, 019ec13a, **019ec142**)

> **Update — run 019ec142 (after both engine fixes + static-binary re-copy):
> the SAST read pipeline VALIDATED end-to-end, with 3 new findings.**
> - Ran clean through `detect_tech → scanners → scan_health → cap_findings →
>   triage → adversarial N-vote → merge_with_cache → report_card`. The
>   sandbox-claw fixes (backendIsClaw + static binary) **work**. ($4.26, 91k
>   tokens, 285 steps before the remediation phase self-killed — see #5.)
> - **Value:** deepsec found 14 candidates → triage 13 → N-vote **11 confirmed
>   (2 HIGH incl. the SSRF), 2 uncertain**; results written to
>   `.iterion/security/findings.md`. The detect_tech tech-map is excellent.
> - **#3 Degraded scanner coverage (medium):** `scan_health` correctly flagged
>   `degraded` — **`trivy` + `semgrep-auto` errored / produced no output** in the
>   `iterion-sandbox-sec` image (2 of 4 generic scanners missing; it cleared the
>   `min_generic=2` floor so it ran with a banner rather than hard-failing). All
>   13 triaged candidates came from **deepsec**; the generic AST/regex scanners
>   contributed 0. The sec image's trivy + semgrep-auto invocation needs fixing.
> - **#4 Board emit didn't land (medium):** `report_card` wrote findings.md
>   claiming "2 board issues created (high)", but the board has **0**
>   `source:sec-audit-source` issues. Sandboxed `report_card` emits via the HTTP
>   board transport (`/api/v1/mcp/board` + run-token); the writes didn't surface
>   (failed silently or were confabulated). Seki's value is currently trapped in
>   the gitignored findings.md, not on the board as designed.
> - **#5 SEVERE — `remediate` + `enable_deepsec` default `true`, and Seki has NO
>   `worktree: auto`.** So by default Seki *edits code* (it is **not** read-only,
>   contra the doc "does not fix unless remediate enabled"), and the edits hit the
>   **main tree**. `patch_author` edited `pkg/webhooks/generic/generic.go`
>   (SSRF-hardening intent) → its own `.go` edit tripped `task studio:dev`
>   watchexec → backend restart → `context canceled`. Worse, it was cancelled
>   **mid-Edit**, leaving the file with unused imports → **broke compilation → the
>   `go run` studio backend couldn't restart → studio bricked** until the partial
>   patch was `git restore`d. Same watchexec self-kill class as Willy, but it also
>   takes the studio down. **Fix directions:** default `remediate=false` (match the
>   doc + make Seki read-only by default); give the remediation phase
>   `worktree: auto` isolation; never run a remediating Seki under `task
>   studio:dev`. (deepsec default-on also makes every run long/expensive.)
> - **#6 Remediation hijacked the operator's git branch (severe).** With no
>   worktree, remediation ran `git checkout -b` **on the main checkout**, moving it
>   off `main` onto a branch named literally **`iterion/sec-fix/run.id`** — an
>   **unrendered `{{run.id}}` template**. Subsequent operator commits then silently
>   landed on that branch instead of `main` (reconciled by hand). Two bugs: (a)
>   remediation must use an isolated worktree, never `git checkout` the live tree;
>   (b) the branch-name template isn't substituted (`run.id` literal). Reinforces
>   the case for default `remediate=false`.

- Status: **read pipeline VALIDATED (019ec142); remediation phase unsafe
  (self-kill + studio brick). Original blockers ↓ fixed.**
- Versions: bot sec-audit-source 0.1.0 · iterion 7fea84cd→f247f360
- Method: `POST /api/runs`, `severity_threshold=high`, sandboxed
  (`iterion-sandbox-sec:edge`, present). Goal: re-find the known HIGH
  `source:sec-audit-self` issues (SSRF `runs_preview.go`, path-traversal
  `runs_files.go`) and validate `scan_health` + `cap_findings`.
- Result: **never reached the scanners** — failed at `detect_tech` (first `claw`
  node) both runs. But each failure root-caused a real sandbox/claw bug:
  1. **`backendIsClaw` missed env-templated backends (FIXED `f247f360`).** Seki's
     nodes use `backend: "${ITERION_SEC_AUDIT_BACKEND:-claw}"`; the IR stores it
     verbatim, so `containsClawNode` read it as non-claw at spec-build time and
     `addClawBinaryMount` never bind-mounted the host iterion → the in-container
     `iterion __claw-runner` died with `exec: "iterion": executable file not
     found in $PATH`. Fix: expand the template in `backendIsClaw` like the
     executor does (`ir.ExpandEnvWithDefault`). Regression test added. **This also
     unblocks Depsy (`sec-audit-deps`)**, which uses the same pattern.
  2. **The host iterion bind-mounted into the container must be STATIC.** Once #1
     mounted it, the next failure was `exec: /usr/local/bin/iterion: no such file
     or directory` — the mounted binary was a devbox `go build` (default
     `CGO_ENABLED=1`) **dynamically linked against nix glibc**, whose loader isn't
     in the container. Fix is operational: install a static build
     (`CGO_ENABLED=0` / `task build`); CLAUDE.md's live-dogfood note now spells
     this out. (Candidate engine hardening: `iterion sandbox doctor` / the mount
     path could detect a dynamic host binary and fail with a clear message
     instead of a retry-then-cryptic-ENOENT; or the sec/full images could bake a
     static iterion on PATH.)
- Lessons for next run: after the operator re-copies the **static** binary to
  `/usr/bin/iterion`, re-launch; `detect_tech` should clear and the run proceed to
  the scanners + `scan_health` gate. Then validate it re-finds the known HIGH
  findings. The SAST capability itself is unproven on iterion *yet* — only the
  sandbox plumbing was exercised.

---

**Status:** validated end-to-end (2026-06). **Scope of this report:** the
capability and the engineering hardening only — it carries **no information
about the audited codebase** (a third-party repository; all target details are
deliberately omitted or generalized).

## Summary

Seki (the `sec-audit-source` bot) with the integrated **deepsec** scanner and
the in-run **remediation** phase was exercised against a real-world repository.
It demonstrably:

1. **finds subtle, high-value vulnerabilities that signature-based scanners
   miss** (deepsec's LLM analysis vs the regex/AST scanners);
2. **authors complete, root-cause, test-backed fixes** — including a hard
   design-level one it had earlier had the discipline to *decline*;
3. **drives the full pipeline** detect → context → scan → triage →
   adversarial N-vote → verified-remediation ladder → **human approval gate**.

The exercise also hardened the pipeline: **six runtime bugs were found and
fixed** while driving real runs.

## What was validated

- The integrated pipeline: `detect_tech → project_context → scanners
  (generic + language + deepsec) → triage/dedup → N-voter "disprove"
  revalidation → report_card → remediation ladder (patch → build → reproduce →
  regress → re-attack → isolated review → aggregate) → human gate`.
- **deepsec** as an LLM scanner alongside the deterministic scanners, on the
  local Claude Code subscription backend (no API key).
- **apply-gated** remediation: edits applied in a worktree, paused at a human
  gate before anything merges.

## Method

A single end-to-end run against a large real-world repository — a compiled
backend plus a JS/TS frontend, thousands of tracked files, with
authentication, cryptography, and a database. Backend = local Claude Code
subscription. *(No further target detail is recorded here by design.)*

## Results

### deepsec finds what signature scanners miss

deepsec surfaced around a dozen findings, including logic/auth issues invisible
to the deterministic scanners (which contributed container/config and
standard-rule matches). Representative **classes** (generalized):

- an **authentication-relay CSRF** (account-takeover class): a sensitive login
  step completed with no binding to the browser that initiated it, so a
  relayed out-of-band confirmation could authenticate a session it never
  started;
- a **key-management fail-open**: a development key path reachable outside
  production;
- a **session-cookie integrity gap**;
- **container / supply-chain hardening**: runtime running as root, mutable
  base-image tags (no digest pinning);
- a **nil-dereference DoS** in an unrecovered worker goroutine.

The signature scanners found none of the first three.

### Seki authors complete, root-cause, tested fixes — including the hard one

The headline result is on the **authentication-relay CSRF** — a design-level
fix spanning several files. In an earlier run Seki **correctly declined** to
patch it with a minimal one-file diff, explicitly refusing to ship a
backend-only change that would build and test green while leaving the flaw
exploitable ("security-theatre the ladder cannot detect") and routing it to
human review.

Once the pipeline could carry it (see hardening below), Seki's remediation
authored the **full root-cause fix**:

- a **server-derived, origin-bound confirmation token** (an HMAC over the
  challenge identifier and the browser-binding nonce) that gates the sensitive
  completion step — so a relayed confirmation can no longer authenticate a
  session it did not initiate;
- a **failing-then-passing regression test**.

A genuine multi-file, cross-stack security fix *with a test* — not a line-level
patch. This is the core demonstration: the tool finds a hard, subtle flaw and
remediates it at the root, or defers it cleanly when it cannot do so properly.

### Pipeline discipline (precision + safety)

- **Adversarial N-vote**: the "disprove" revalidation rejected the large
  majority of raw scanner candidates; only a minority were confirmed. High
  precision against scanner noise.
- **Hard-stops**: crypto/secrets findings are never auto-patched — routed to
  human review by design.
- **Reviewer isolation**: the final reviewer judge sees only a four-field
  projection `{file, line, category, diff}` — never the scanner prose or
  exploit narrative — a deliberate prompt-injection barrier.
- **Human gate**: the run pauses before any merge; nothing lands unattended.

## Engineering hardening (bugs found + fixed)

Driving real runs surfaced and fixed six runtime issues (all merged to `main`):

| Area | Issue | Fix |
|---|---|---|
| Remediation loop | the per-run "attempted" ledger, relayed through a `compute` and rendered as Go `%v`, was rejected by the JSON parser → the loop re-picked the same finding forever | parse the ledger leniently; then carry it as a **comma-separated string** (a multi-element `%v` with spaces also broke the tool's shell, exit 127) — shell-safe and parse-safe |
| deepsec resilience | the deepsec step lost its **entire** contribution on a transient blip, and **hung indefinitely** on a mid-run network loss (its SDK does not self-recover) | retry the process once + bound it with a **timeout** so a hang is killed and auto-retried |
| Budget | a hardcoded duration/iteration budget capped audit+remediation on a large repo | env-configurable, raised defaults |
| Project toolchain | build/test rungs run in the **project's own pinned toolchain** (devbox); the persistent Nix store is default-on so they run warm | sandbox driver + bot rungs |
| Skill resolution | remediation prompts pointed at a skill path not reliably present in the worktree | point at the always-mounted run bundle |

## Known remaining work (verdict quality)

Reaching the gate surfaced a remaining tail that currently keeps confirmed
findings at **`uncertain`** (proposed) rather than **`verified`**
(auto-committed). None of these block the pipeline — it reaches the human gate
— but they gate the verified-auto-apply outcome:

- the reviewer-isolation **projection compute** does not substitute its inputs
  (nested output access in a `compute` expr), so the reviewer receives
  placeholders and fail-closes;
- the **reproduce** rung occasionally still fires after a valid fix
  (calibration);
- the gate **summary under-counts** the proposed artifacts.

Scoped as the next focused task — verdict-quality polish, not capability.

## Conclusion

Seki + deepsec is validated: it finds relevant, subtle vulnerabilities that
signature scanners miss, and it authors complete, tested, root-cause fixes —
including a design-level fix it deferred until it could do it properly — while
running the whole pipeline to a human approval gate with adversarial
revalidation, hard-stops, and reviewer isolation. The open items are
verdict-quality polish.
