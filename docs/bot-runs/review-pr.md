# Revi — `review-pr` run bilans

Read-only cross-family code reviewer. Two independent reviewers (Claude + GPT)
review a branch/PR diff, findings are merged/de-duplicated, and one issue per
finding is published to the native board (label `source:revi`); with `--var
pr_url` it also posts an inline forge review. Never edits or commits. See
[bots/review-pr/](../../bots/review-pr/).

## 2026-06-13 — review the campaign diff (run 019ec0e8)

- Status: **validated — high value.**
- Versions: bot review-pr 0.2.0 · iterion 7fea84cd (binary refreshed mid-campaign)
- Method: `POST /api/runs`, `base_ref=9197bcfd` (review the campaign's own fresh
  commits `9197bcfd..HEAD` — the `scan_shards`/`botregistry` fixes + the bilans),
  `severity_threshold=low`, `post_to_board=true`. Read-only, no sandbox. Backends:
  `claude_code` (reviewer_claude, emit) + `claw` gpt-5.5 (reviewer_gpt). ~37k tokens,
  ~$1.18, 151 steps, status `finished`.
- Result: `diff_precheck` (found changes) → fan-out **reviewer_claude ‖ reviewer_gpt**
  (parallel, confirmed) → `emit` → **1 deduped board issue** (`source:revi`,
  `severity:medium`, `type:correctness`). No commits (read-only, as designed).

### Value (genuinely high — caught a real second-order bug)
- The single finding is excellent: **"Cloud request-construction failures block until
  shard timeout" at `cmd/iterion/scan_shards.go:458`** — i.e. Willy's fix `4c525a6e`
  (handle the dropped `http.NewRequestWithContext` error) is *masked* by `awaitTerminal`,
  which polls a run document that never exists for a never-launched shard, hanging until
  `--timeout` (default 2h) instead of failing fast. Precise anchor, correct mechanism,
  actionable fix sketch. **Verified against the code and fixed** (`59cfedcc`, with a
  regression test). The pre-existing `ITERION_SERVER_URL`-unset / read-workflow paths
  had the same latent hang.
- **No noise:** the diff was mostly docs (≈280 of 387 lines) + two small code changes;
  Revi flagged 0 in the clean botregistry dedup, 0 in docs, and 1 real issue in the
  changed Go. Cross-family dedup worked; severity/type/confidence labels are clean.
- **Dogfood dynamic worth keeping:** a *breadth* bot (Revi) caught an incompleteness in
  a *depth* bot's (Willy) committed fix. Running review-pr over each loop bot's output is
  a cheap, high-signal second line of defence.

### Findings / misses
- The finding came from the **gpt** reviewer only (confidence `medium`) — Claude's
  reviewer didn't independently raise it. Single-family findings are real but lower-
  confidence; the cross-family agreement signal didn't fire here (still correctly
  published at the `low` threshold). No false positives.
- Minor: the `emit`/`reviewer_*` node outputs aren't surfaced in `run.json.checkpoint`
  in a easily-parsed shape (had to read the board to see findings) — cosmetic.

### Engine hardening
- `awaitTerminal` pre-dispatch-failure hang — **fixed `59cfedcc`** (+ regression test
  `TestAwaitTerminal_PreDispatchFailureDoesNotHang`). Directly attributable to this run.

### Lessons for next run
- Revi is a strong, low-noise read-only reviewer; point `base_ref` at the commit before
  the work to review a clean range (`base..HEAD`). Default `post_to_board=true` lands one
  issue per finding under `source:revi` — fine for real triage, set `false` for a pure
  dry-run.
- Use Revi as a routine second pass over Willy/Featurly/Billy output — it catches
  second-order issues the implementer's own review loop can miss.
