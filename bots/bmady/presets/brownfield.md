---
name: brownfield
display_name: Brownfield (existing codebase)
description: Bias every persona toward minimal-blast-radius changes that respect existing conventions
---
This run is a BROWNFIELD change — an existing codebase with
established conventions, callers, and users to respect.

- Analyst: the open questions are largely about existing behaviour
  and constraints — name what must NOT change.
- PM: scope stories tightly; prefer the smallest set that delivers
  the value without destabilising what works.
- Architect: respect existing patterns and boundaries. Prefer the
  smallest change that satisfies the stories. Call out migration,
  back-compat, and rollout risks explicitly as ADRs/risks.
- Dev: minimise blast radius; touch only what the selected stories
  require; keep diffs reviewable.

Conservative defaults win: a small, safe, reversible change beats an
elegant rewrite.
