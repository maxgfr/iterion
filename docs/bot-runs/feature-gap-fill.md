[← Bot runs](README.md)

# feature-gap-fill (Fini) — bilans

Gap-driven feature completer. A specialisation of feature_dev: the input is a
structured `gap_spec` ("here is what's implemented, here is what's missing"),
not a greenfield prompt. Fini reads the partial implementation, completes the
missing parts, runs the alternating Claude/GPT review-fix loop to convergence,
then commits. Inputs are typically the `type:feature-gap` issues filed by the
adr-cartograph (Adry) bot.

## 2026-06-14 — first dogfood, file-diff size-cap gap (run 019ec599)

- **Status: validated** — Fini produced real, correct, tested code from a gap
  spec and converged cleanly. NOT a façade (verified independently).
- **Versions:** bot 0.1.0 · iterion @ `03f398e2` (main)
- **Method:** `iterion run bots/feature-gap-fill/main.bot --var gap_spec='<…>'
  --sandbox none --merge-into none`, gap_spec = the **file-diff-payload**
  issue Adry filed (`Cap file contents loaded for Monaco diff payloads`).
  Launched under `devbox run` with `~/.local/bin` on PATH so the host run had
  both `go` (devbox) and `claude` (host) — sandbox forced off because this
  worktree's engine predates the concurrent sandboxed-claw fixes and `~/.codex`
  (forfait) is not mounted into the container. `worktree: auto` still isolated
  the Go edits; forfait forced (`ITERION_OPENAI_USE_OAUTH=1`).
- **Result: converged + committed.** Flow: survey_existing → plan → act →
  simplify → reviewer_claude + reviewer_gpt **both approved cross-family on the
  first pass (0 fixer iterations)** → prepare_commit → commit_changes → done
  (~18 min, 12:07→12:25). Committed `88943d4b` ("feat(git): cap diff payload
  reads to avoid OOM on oversized files", **5 files, +267/-30**) on the
  storage branch `iterion/run/magneto-whomp-etherspark-4f0a` — **NOT on main**
  (`--merge-into none`), preserved for human review. The engine recorded
  `final_commit` + `final_branch` and removed the worktree cleanly.
- **Value: high — a genuine OOM fix.** The implementation is substantive and
  correct: an `errOversized` sentinel + `diffPayloadCap` (reuses
  `untrackedReadCap`'s 5 MiB), the reading primitives (`readWorktreeFile`,
  `showAt`) return oversized **before** loading the blob into memory, both
  sides blanked + `Oversized=true`, oversize-wins-over-binary — meeting all the
  issue's acceptance criteria. It also ADDED `pkg/git/diff_test.go` (160 lines:
  `TestDiffOversizedWorktree`, `TestDiffOversizedHead`), exactly as the
  acceptance criteria required.
- **Anti-façade verification:** I checked out `88943d4b` into a throwaway
  worktree and ran `go build ./pkg/git/` + `go test ./pkg/git/` independently
  → **builds clean, tests pass** (`ok 1.162s`). Real work, not a reported
  parity façade.
- **Pipeline validated:** the full A→C handoff works end-to-end — **Adry found
  the gap → filed a `type:feature-gap` issue → Fini completed it** with tested
  code. This is the architecture-evolution loop the suite was designed for.

### Lessons for next run
- Host run (`--sandbox none`) needs BOTH `go` and `claude` on PATH: launch
  under `devbox run` (go) with `~/.local/bin` appended (claude). The sandbox
  path is cleaner *if* `~/.codex` (forfait) is mounted into the container —
  it currently is not, so host-mode is the reliable forfait path until that's
  wired (or an API key is used).
- The implementation lands on a storage branch (`--merge-into none`) for human
  review — the operator merges it (or routes it through adr-rechallenge) rather
  than Fini pushing to main. Correct default for an autonomous code-mutating bot.
- Convergence was clean (0 fixers) because the gap_spec was precise (Adry's
  issue carried evidence + acceptance criteria). A vague gap_spec would lean
  harder on the review-fix loop.
