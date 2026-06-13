# Evoly (`evolve`) — bot run bilan

Strategic / architectural evolution partner. Surveys a mature repo,
accumulates a long-horizon vision in **per-bot** memory across sessions,
elicits operator context, and proposes evolutions as dispatch-ready
backlog tickets + findings for Nexie. See
[bots/evolve/README.md](../../bots/evolve/README.md). Append newest-first.

---

## 2026-06-13 — first dogfood: per-bot memory + full pipeline (runs 019ec1d5, 019ec1dc)

- **Status:** validated (core pipeline survey→elicit→investigate→synthesize→
  cross-family review→human-review proven live; propose_evolutions +
  emit_backlog not yet exercised live — they run after vision approval, the
  well-trodden Nexie board path).
- **Versions:** bot 0.1.0 · iterion worktree `worktree-evolve-bot` (base
  `d1fe421c`).
- **Method:** all memory-bearing nodes `claw` + `openai/gpt-5.5` (ChatGPT
  forfait); the one cross-family "claude" reviewer `claude_code` +
  `claude-opus-4-8` (Claude Code OAuth forfait). Run from the
  `.claude/worktrees/evolve-bot` worktree against the iterion repo itself.
  `--var scope_notes=...`. No `--store-dir` (workspace default). No
  worktree:/sandbox: (read-only bot).
- **Result:** converged to the `human_review_vision` pause across two
  sessions. Run 2 (019ec1d5): survey → ask_brief (paused, answered via
  resume) → investigate (wrote CONTEXT_BRIEF.md) → synthesize (wrote
  VISION.md) → **failed at review_fanout** (workspace-safety: reviewers
  missing `readonly`). Run 3/4 (019ec1dc, after the readonly fix): full
  pipeline through both reviewers → aggregate (wait_all) → carry →
  human_review pause. review_claude on claude_code: 48s, 7979 tok, $0.34.
  Per-node gpt-5.5 nodes ~$0.05–0.09 each.
- **Value:** genuinely useful strategic output — a 10-axis vision *"Iterion
  as a Trustworthy Local Team Engine"* (backend-pipeline decomposition,
  one RunLifecycle contract, an auditable CapabilityContext, dispatcher
  invariants, observability taxonomy, .bot/persisted-format v1 subsets…),
  each axis with direction / key_moves / target_state, plus 11 guardrails
  and a rationale. Grounded in real files (ClawExecutor overload,
  Engine option sprawl, checkpoint-vs-event authority). This is exactly the
  "architect a vision" deliverable.

### Headline feature PROVEN live: per-bot cross-session memory

- `visibility: bot` writes landed at
  `~/.iterion/projects/-home-jo-lab-ai-iterion/bots/evolve/memory/vision/`
  (CONTEXT_BRIEF.md + VISION.md), and the **legacy project path
  `…/memory/vision/` was ABSENT** — proving the bot-visibility axis, not the
  legacy project-shared path.
- **Stable across worktrees:** the run launched from the *worktree*, but the
  memory keyed off the **main repo root** (`-home-jo-lab-ai-iterion`), not
  the ephemeral worktree path — i.e. `memBase = task.RepoRoot` via
  `findGitRoot`. So a future run from the main checkout sees the same
  accumulated vision. (This is the G2 worktree-stability concern from the
  plan, validated live.)
- **Cross-session continuity proven:** run 3's survey autoloaded run 2's
  VISION.md/CONTEXT_BRIEF.md and produced *materially deeper* axes — the
  accumulated context made the second pass smarter, exactly the intent.

### Findings / engine hardening

1. **(ENGINE BUG, HIGH) mid-turn `ask_user` is broken on claw +
   openai/gpt-5.5 (forfait).** The bot's first design used
   `interaction: human`/`llm_or_human` on the investigate agent so it could
   ask the operator mid-turn via the `ask_user` MCP tool. On claw+openai
   this fails: the `ask_user` call is **not** converted to a clean pause —
   the tool loop continues to a second LLM call, which openai rejects with
   `400: No tool output found for function call`. Reproduced with BOTH
   `interaction: human` and `llm_or_human`, so it is not mode-specific.
   - Static trace: the whole iterion chain preserves `*delegate.ErrAskUser`
     (handler → `ExecuteAskUser` → `RegisterClawTool` wrapper →
     `RegisterBuiltin` → `toolDefsToGeneration` →
     `executeToolsDirect`'s `errors.As` at
     [generation.go:576](../../pkg/backend/model/generation.go)). Yet the
     runtime decisively continued to step 2, so `errors.As` returned false —
     i.e. the claw-code-go **openai/forfait provider intercepts `ask_user`
     internally during streaming**, before iterion's `executeToolsDirect`
     sees the `ErrAskUser`. claw+anthropic ask_user was validated previously;
     claw+openai-forfait was not. Needs a focused fix in the openai-provider
     tool-result lifecycle (likely a claw-code-go revendor) + a live test.
   - **Workaround shipped:** elicitation now uses a graph-level `human` node
     (`ask_brief`) — the proven, backend-agnostic interaction path (Nexie's
     shape). The user goal (interrogate the operator during investigation,
     persist cross-session) is fully met; only the *mechanism* changed.
2. **(BOT BUG, fixed) reviewers needed `readonly: true`.** The two parallel
   judge reviewers had mutation-capable tools (`bash`) without `readonly`,
   so the workspace-safety guard rejected 2 mutating parallel branches.
   Added `readonly: true` to both (they only inspect). Lesson for any
   fan-out of tool-equipped judges: mark them `readonly`.
3. **(ENGINE NIT, MEDIUM) structured `{{outputs.X}}` substitution renders as
   a Go `map[...]` string, not JSON.** `review_claude` (claude_code)
   reported the vision arrived as an unrendered/odd blob and recovered via
   fallback; the human_review form likewise shows `map[_backend:claw …]`.
   The aggregate + pipeline still worked, but passing large structured
   outputs into prompts should render JSON, not Go's map stringification.
   Worth a templating fix.
4. **(NIT) `iterion validate` is more lenient than `iterion run`.** A literal
   `{{issue.title}}` mention in a prompt body (documentation text) passed
   `validate` but failed `run` with `C004 unknown reference namespace`.
   Validate should catch C004 too. (Fixed in the bot by de-bracing.)

### Lessons for next run

- The mid-turn ask_user limitation is real for forfait users — until the
  claw+openai fix lands, **author operator interaction as graph-level
  `human` nodes**, not mid-turn `ask_user`, on any claw+openai bot.
- Drive one full cycle THROUGH approval next time to exercise
  `propose_evolutions` (findings/ writes) + `emit_backlog` (backlog tickets
  with `set_bot` + self-contained body). Verify on `/board` that tickets are
  `backlog`, labelled `source:evolve`, and that a drag-to-`ready` dispatches
  the named bot.
- Consider rendering reviewer/human inputs as JSON (finding #3) so the
  reviewer sees clean structured data.
- The vision quality on gpt-5.5 was high; opus (via `claude_code`, or claw
  if an API key is ever acceptable) would only be worth it for the synthesis
  node if the operator wants more depth.
