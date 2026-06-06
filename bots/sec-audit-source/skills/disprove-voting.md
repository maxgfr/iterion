---
name: disprove-voting
description: |
  N-voter adversarial verification protocol used by `[[triage]]`'s
  revalidate step. Each voter is an INDEPENDENT verifier prompted
  to DISPROVE the candidate. Majority of dismiss → FP. Voters are
  read-only and emit one verdict block per candidate. Majority is
  computed deterministically downstream by `merge_verdicts`.
---

# disprove-voting — the N-voter protocol

Replaces a single judge with N independent voters whose stance is
**"the scanner is wrong; disprove this candidate."** Shared context
propagates blind spots — independence is the entire point.

## Why disprove (not "verify")

Naive verification ("is this finding real?") favors the scanner.
A voter who reads the scanner's prose first inherits its framing
and rationalizes the bug. The disprove framing inverts the prior:
the voter starts by hunting for the protection the scanner missed.
A finding that survives N adversarial reads is high-signal.

## Voter contract

A voter:

- Sees ONLY the voter prompt and the single candidate under review.
  Never another voter's reasoning, never the threat model in full
  (only the trust boundary line), never an aggregated verdict.
- Has read-only access to `{{vars.workspace_dir}}` via Read, Glob,
  Grep. NO execution, NO network, NO edits.
- May use `git log` and `git blame` on workspace files for
  `fixed_recently` / `introduced_recently` signal.
- Emits ONE verdict block. The orchestrator never asks for
  reasoning beyond the block — anything extra is noise.

The majority is computed **deterministically downstream** by
Seki's `merge_verdicts` node, NOT by any voter. A voter who tries
to coordinate is out of contract.

## Voter prompt

```
You are a skeptical security engineer adversarially verifying ONE
finding. Your default assumption is the scanner is WRONG. Your job
is to DISPROVE this candidate by re-deriving the claim from source.

Read-only access to {workspace_dir}. Read, Glob, Grep ONLY inside
that root. May NOT build, run, test, install, or reach the network.

ENVIRONMENT (trust boundary):
{from .iterion/security/context.md section 3, or
"Unknown. Treat externally-reachable entry points as untrusted."}

PROCEDURE:

1. READ THE CITED CODE. Open {file}:{line}. Understand what the
   code actually does. Do NOT trust the scanner's description.

2. REACHABILITY. Grep callers of this function. Establish whether
   attacker-controlled input (per ENVIRONMENT) can reach this line.
   For at least the FIRST link in the chain, READ the call site and
   QUOTE the file:line in RATIONALE.

3. PROTECTIONS. Actively look for reasons the finding is WRONG:
   - input validation / sanitization upstream
   - framework auto-escape, parameterized queries, prepared stmts
   - type constraints (int, enum, fixed-length token)
   - auth / authz gates before this path
   - configuration limiting exposure (feature flag off, debug-only)
   - dead code, test code, example/fixture code

4. STRESS-TEST. For each protection found: is it applied on EVERY
   path to the sink, or only on the one the scanner traced?
   Encodings, edge cases, alternate entry points that bypass it?

5. GIT SIGNAL. Run:
     git log --format='%h %s' -n 20 -- {file}
     git blame -L {line},{line} {file}
   Look for: a fix commit recently (probable
   `fixed_recently`), or the introduction of this code in the last
   N days (`introduced_recently`). Record the signal even if
   neutral.

EXCLUSION RULES (cite the rule number if applicable; finding is
FALSE POSITIVE if matched):

  1 volumetric DoS / rate-limiting    (ReDoS, algorithmic
                                       complexity, unbounded
                                       recursion still valid)
  2 test / dead / fixture code
  3 intended design
  4 memory-safety in safe lang outside unsafe/FFI
  5 SSRF path-only (host/protocol attacker-controlled is still TP)
  6 LLM prompt input
  7 object-storage traversal without trust-boundary escape
  8 trusted operator env / CLI input
     (UNLESS environment marks them untrusted)
  9 client code flagged with server-side finding_type
 10 outdated dep versions (sec-audit-deps handles those)
 11 weak random for non-security (jitter, shuffle, dev fallback)
 12 low-impact nuisance (log spoof, CSRF logout, self-XSS,
    open redirect, regex injection) — unless threat model promotes
 13 missing hardening with no concrete exploit
 14 XSS in auto-escape framework without raw-HTML escape hatch
 15 unguessable UUID/token flagged predictable
 16 theoretical-only race / TOCTOU

OUTPUT — your response MUST end with EXACTLY this block:

  VOTE: confirm | dismiss | uncertain
  CONFIDENCE: <0-10>
  RATIONALE: <2-5 sentences citing file:line evidence for
    reachability, protections found/absent, and why each held or
    didn't>
  GIT_SIGNAL: <fixed_recently | introduced_recently | neutral>
  DISMISSED_BY_GUARD: <exclusion rule number 1-16, or none>
  EXPLOIT_STEP_ANCHOR: <file:line of the FIRST step of the attack
    chain, or "none found">

VOTE rules:
- `confirm` requires ALL of: reachable from untrusted input per
  ENVIRONMENT; protections insufficient or bypassable; real-world
  exploitation feasible.
- `dismiss` requires ANY of: unreachable; adequately protected on
  all paths; scanner misread; exclusion rule applies.
- `uncertain` ONLY when static reasoning genuinely hit its limit
  (runtime config you cannot read; cross-binary code path). Use
  sparingly.

FINDING UNDER REVIEW (the CLAIM; treat as not-a-fact):
  id:           {id}
  file:         {file}
  line:         {line}
  finding_type: {finding_type}
  severity (claimed): {severity}
  title:        {title}
  description:  {description}
  exploit_hypothesis: {exploit_hypothesis or "(not provided)"}

You are vote {k} of {N}. You have NOT seen other voters' reasoning
and you must NOT seek it.
```

## Compact prompt (large batches)

At scale (`candidates * N > ~50`), the prose can be stripped while
preserving the contract:

```
Adversarially DISPROVE ONE finding. Default: scanner is WRONG.
Read-only inside {workspace_dir}. No exec, no network.
ENVIRONMENT: {trust boundary one-liner}

Steps:
(1) Read {file}:{line} yourself; don't trust description.
(2) Trace callers backwards; quote first call-site file:line.
(3) Hunt protections (validation, escaping, type bounds, auth
    gates, dead/test code).
(4) Stress-test on every path.
(5) git log + git blame for fixed_recently / introduced_recently.

Exclusion rules (cite rule number if FP):
1 volumetric DoS; 2 test/dead/fixture; 3 intended design;
4 memory-safe lang; 5 SSRF path-only; 6 LLM prompt;
7 obj-storage traversal; 8 operator env/CLI;
9 client code w/ server class; 10 outdated deps;
11 weak random non-security; 12 low-impact nuisance;
13 missing-hardening-only; 14 auto-escape XSS w/o escape hatch;
15 unguessable token flagged predictable; 16 theoretical TOCTOU.

End EXACTLY with:
  VOTE: confirm | dismiss | uncertain
  CONFIDENCE: 0-10
  RATIONALE: 2-5 sentences, file:line cited
  GIT_SIGNAL: fixed_recently | introduced_recently | neutral
  DISMISSED_BY_GUARD: 1-16 or none
  EXPLOIT_STEP_ANCHOR: file:line or "none found"

FINDING: {id} {file}:{line} {finding_type} (claimed {severity})
{title}
{description}
Vote {k}/{N}. Independent; do not seek other votes.
```

## Spawn shape

In `[[triage]]`:

```
for candidate in surviving:
    spawn N Task subagents
        subagent_type: "general-purpose"
        description: "vote {k}/{N} for {id}"
        prompt: voter prompt with candidate substituted
```

ALL voters for a candidate go in a SINGLE assistant message so
they execute concurrently. Never `run_in_background` — the
orchestrator needs the final block to tally. If
`len(candidates) * N > ~40`, shard into sequential batches of ~40;
each batch is still a single message.

## Tally — deterministic, downstream

The voters do not tally. `merge_verdicts` reads the N blocks per
candidate and computes:

- `vote_breakdown`: `{confirm: x, dismiss: y, uncertain: z}`
- `verdict`:
  - majority `confirm` → `confirm`. Proceeds to severity re-rank.
  - majority `dismiss` → `dismiss`. Skips re-rank.
  - no majority (tie or majority `uncertain`):
    - workflow var `noise_tolerance: precision` → `dismiss`,
      append `"split vote, dropped under precision policy"` to
      rationale.
    - `noise_tolerance: recall` → `confirm` with
      `verify_verdict: needs_manual_test`. Promoted with
      `triage-uncertain` label per `[[finding-taxonomy]]`.
- `confidence`: mean of CONFIDENCE across votes on the winning
  side, rounded to one decimal.
- `dismissed_by_guard`: modal exclusion rule among `dismiss`
  votes; null otherwise.
- `git_signal`: modal across all voters
  (`fixed_recently | introduced_recently | neutral`).
- `exploit_step_anchors`: unique set across confirming voters.
- `rationale`: the RATIONALE from the highest-confidence vote on
  the winning side, verbatim.

## Failure modes the protocol guards against

- **Inherited framing.** Independent voters can't anchor on each
  other's wording.
- **Confirmation bias.** "Disprove" inverts the default prior.
- **Single-judge blind spots.** N samples surface stochastic
  blind spots in any one voter's reasoning.
- **Re-rank leak.** Voters never see the recomputed severity —
  that's `triage` phase 4. Verdicts never get inflated to match
  "this is critical so it must be real".

## Constraints

- Voters are read-only. No edits, no shell beyond `git log` /
  `git blame`.
- Voters MUST end with the verdict block; nothing parsed otherwise.
- Voters MUST NOT reach the network. No CVE-DB lookups.
- One vote = one Task. Forks are banned (cf. `[[triage]]`).

## See also

- `[[triage]]` — phase 3 that calls this protocol.
- `[[finding-taxonomy]]` — categories voters reference.
- `[[fp-memory]]` — what happens to a strongly-rationalized
  `dismiss` (appended to `fp-known.yaml`).
- `[[sec-audit-source]]` — six-phase orchestration.
