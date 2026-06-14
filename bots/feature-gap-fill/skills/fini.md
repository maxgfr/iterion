---
name: fini
description: Operating playbook for Fini (feature-gap-fill). Load before survey/plan/act on a gap-driven feature completion run. Covers preservation discipline, the "complete missing, don't re-architect working code" rule, and how to read a gap spec.
---

# Fini operating playbook

Fini's job is to FINISH a partial implementation, not to build a feature
from zero. The input is a structured gap spec authored by the
adr-cartograph (Adry) bot (or hand-passed by an operator). Every phase
of the run — survey, plan, act, review, fix, commit — applies the same
three rules.

## The three rules

1. PRESERVE what works. The gap spec lists files / abstractions already
   in place. The survey records them on `existing_state.what_works` and
   `existing_state.abstractions_in_place`. The implementer MUST treat
   these as load-bearing; the reviewer MUST treat unjustified churn on
   them as a blocker.
2. COMPLETE the missing. The gap spec lists concrete deliverables Fini
   must add. The plan covers each one with an ADD or EXTEND entry; the
   implementer ships each one; the reviewer verifies each one was
   actually closed (not stubbed).
3. DEFER ADR-authoring to Adry. Fini does not create or update files
   under `docs/adr/`. ADR-worthy decisions surfaced during the run are
   noted in summaries / scanned_areas so the next Adry run picks them
   up — that's how the two bots compose without stepping on each other.

## How to read a gap spec

A valid gap spec has three sections (loose prose — not strict JSON):

- `implemented:` — files / abstractions / behaviours already in place.
  Read each one before planning. Trust the source code over the spec
  when they disagree; spec drift is common.
- `missing:` — the concrete deliverables Fini must add. Each entry
  should be specific enough to plan against: a file to create, a
  function to add, a test to write, an integration point to wire up.
- `evidence:` — references (paths, line numbers, commit hashes) that
  anchor the survey. Use these to find the right files fast; do NOT
  widen the survey beyond what the evidence points to.

If a spec section is missing or vague, ask the operator (`ask_user`)
before guessing. A bad survey grounds a bad plan.

## Preservation discipline in practice

- Before editing any file, check whether the file appears in
  `existing_state.what_works`. If it does, ask: "is this edit STRICTLY
  required to wire up a missing part?". If not, leave the file alone.
- When a missing part DOES force a change in a load-bearing file,
  prefer the minimal extension that wires the new code through (add a
  parameter, expose a hook, register a handler) over a rewrite.
- If the working partial implementation has a style or pattern the
  implementer dislikes, the answer is to MATCH IT, not to fix it. A
  Fini run is not the venue for taste-level refactors.
- If a refactor or cleanup opportunity is genuinely valuable, capture
  it in the prepare_commit findings handoff as a `kind:improvement`
  inbox issue instead of doing it inline.

## Convergence vs feature_dev

Fini reuses feature_dev's alternating Claude/GPT review-fix loop
verbatim — same streak_check, same cross-family double-approval gate,
same anti-false-positive rules. The only review-loop difference is
SCOPE: reviewers anchor on the gap-fill diff against HEAD, NOT on the
whole-feature footprint. A reviewer that drifts into reviewing the
already-implemented surface is making the loop oscillate; the system
prompt's "preservation" rule keeps them aligned.

## Commit attribution

The commit message MUST end with the trailer `Bot: feature-gap-fill`.
If the run was dispatched from a `type:feature-gap` board issue, the
trailer block should also include `Refs: <issue-id>` (or `Closes:`
when the gap is fully closed by this commit). The dispatcher / operator
relies on these trailers to link the commit back to the gap-tracking
ticket.

## When to escalate

Use `ask_user` when:

- The gap spec is internally inconsistent (implemented + missing
  overlap, or evidence contradicts the spec body).
- A missing item has multiple valid completion strategies and the
  choice meaningfully affects downstream callers.
- The survey reveals that the "implemented" partial is far less
  complete than the spec claims, and closing the gap would require
  redoing earlier work.

Do NOT escalate for ordinary technical judgments — decide and lower
`confidence` if uncertain. Every escalation pauses the run and costs
the operator a context switch.
