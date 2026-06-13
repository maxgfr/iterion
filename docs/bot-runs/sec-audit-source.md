# Seki + deepsec — validation

## 2026-06-13 — iterion self-audit dogfood (runs 019ec10f, 019ec13a)

- Status: **blocked → 2 engine fixes; SAST validation still pending a clean run.**
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
