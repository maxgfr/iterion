# ADR-043 — Provider/model support tiers: Anthropic + OpenAI first-class, z.ai/GLM as an opt-in fallback lane

Status: accepted (2026-06-23)

## Context

The 2026-06-23 GLM-5.2 dogfood campaign ran the bot catalogue on a mixed stack —
`claude_code` nodes routed to z.ai/GLM-5.2 (1M context, billed on a separate z.ai
credit) with Anthropic/opus + OpenAI/gpt-5.5 forfait as the other lane — to save
Anthropic quota and compare. See
[2026-06-23-glm-dogfood-campaign.md](../bot-runs/2026-06-23-glm-dogfood-campaign.md)
and `/tmp/opus-vs-glm52.md` for the run-level detail.

Observed, live:

- GLM-5.2 strengths: 1M context (digested sec-audit's large triage input where
  gpt-5.5/forfait stalled ~18 min); separate credit pool (parallel-provider
  capacity, Anthropic savings); drop-in via the `claude_code` backend (`zai` hint).
- GLM-5.2 weaknesses: **structured-output reliability < opus on complex schemas**
  (feature-gap-fill's reviewer looped on `missing required field` on GLM-5.2 even
  with the engine missing-field retry, then completed on opus; sec-audit's voters
  needed retry + an explicit OUTPUT CONTRACT to tolerate GLM); survey verbosity /
  slow convergence (adr-cartograph ~900 tool-calls); occasional transient throttle
  + the z.ai 5h rate-limit cap tripped mid-campaign.
- Anthropic/opus: reliable structured output (completed what GLM missed), tighter
  loop convergence; smaller standard context than GLM; forfait quota to husband.

## Decision

**Anthropic and OpenAI are the first-class model/provider tiers.** Bots' default
models, schemas, and reliability guarantees target opus/gpt. **z.ai/GLM is a
supported but second-tier, opt-in lane** — best used as the *primary* of a
per-node provider+model failover chain with an Anthropic fallback
(`provider: "zai:glm-5.2,anthropic:claude-opus-4-8"`, ADR-004), where its 1M
context and cost help while opus catches its structured-output misses.

We are **not** investing now in making GLM first-class (the work below). It would
add real complexity for a second-tier provider; the failover chain already gives
GLM's upside with opus's reliability as the safety net.

## Follow-ups required IF we later promote GLM to first-class (NOT doing now)

1. Generalize the per-provider+model failover chain to every review-loop bot's
   `claude_code` reviewers (feature-dev, feature-gap-fill, branch-improve-loop,
   whole-improve-loop) — today only sec-audit-source uses it; the others fail hard
   on a GLM structured-output miss instead of falling to opus.
2. Schema-robustness pass: bots that consume strict structured output must tolerate
   a transient missing/short field (bounded retry + defensive coercion) rather than
   hard-fail — done for sec-audit-source's voters + the engine `validateAndRetry`
   missing-field retry; extend the pattern to the other strict-schema nodes.
3. z.ai rate-limit ergonomics: surface the 5h-cap window + make the failover
   automatic+seamless (the chain handles per-call fallback; a global "z.ai capped,
   prefer anthropic for N hours" hint would avoid per-call retries during a cap).
4. Model-spec registry: GLM coverage in the dynamic registry (ADR-042) is via the
   curated fallback until models.dev indexes new GLM releases; a z.ai-specific spec
   source would remove the lag.
5. GLM survey/convergence prompt tuning (verbosity) if GLM becomes a default.

## Consequences

- Bots stay reliable on the first-class lane by default; GLM is available for
  cost/context wins via explicit opt-in (env model overrides + the failover chain).
- The dogfood-built features (per-provider+model failover ADR-004, dynamic
  model-spec registry ADR-042) remain valuable regardless — they make the
  multi-provider story work without GLM being first-class.
