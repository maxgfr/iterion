# bmady (Bmady) — dogfood bilan

Index + template: [README.md](README.md). Newest first.

## 2026-06-14 — full BMAD flow validated end-to-end via API-driven gates (run 019ec66b)

- Status: **validated.** All five BMAD personas + all five human gates ran to a
  shipped, tested, committed feature. I drove the gates programmatically (resume
  API), not Playwright.
- Method: dedicated worktree studio :4899, `worktree: auto` (no sandbox), all
  nodes `claude_code`/opus (no claw/gpt → no forfait shape flakiness).
  `--var brief="Add a --short flag to iterion version that prints only the
  semantic version, for shell scripts"`, `merge_into=none`.
- Flow: analyst (Mary) → **elicit_brief** → pm (John) → **review_prd** →
  architect (Winston) → **approve_arch** → **select_stories** → dev (James) →
  qa → **final_review** → commit → done. Each gate paused as
  `paused_waiting_human`; answered via `POST /api/runs/{id}/resume` with the
  node's output-schema fields (`clarifications` / `action` / `approved` /
  `selected_story_ids`+`priority`+`wip_limit` / `action:ship`).
- Quality of each persona (high):
  - **Analyst** surfaced 5 genuine open questions (build-date misconception,
    `dev` un-stamped handling, leading-`v`, `--json` composition, flag spelling)
    — real ambiguities, no solutioning.
  - **PM** produced precise acceptance criteria incl. "no regression to
    RawVersion / desktop auto-updater" and "`--json` wins deterministically".
  - **Architect** correctly judged this "exposure work, not new machinery" and
    reused the existing `FullVersion`/`Version` seam (3-layer minimal design).
  - **Dev** implemented exactly to spec AND wrote tests unprompted
    (`cmd/iterion/version_test.go` + `pkg/cli/version_test.go`).
  - **qa** judge: blockers=[], confidence high.
- Result: `final_commit f452cccf` on storage branch
  `iterion/run/dawn-thrash-midnightkazoo-9da0` (not merged). The feature is
  correct: `ShortVersion()` reads only `appinfo.Version` (commit suffix
  structurally impossible), strips one leading `v`; `--short` prints it;
  `--json` takes precedence; `version_test.go` covers both; docs in
  `docs/cli-reference.md` + `CLAUDE.md`.
- Finding (confirms Doki): the commit also swept in **`.claude/skills/bmady-*.md`**
  (6 files) — the runtime skill mirror. The clone carries main's `.gitignore`
  (which lacks the `.claude/skills/` entry added on the `c082-board-emit` branch,
  commit `a9b6d671`), so Bmady's `commit_changes` included the mirror as if it
  were source. This is exactly the data-loss/noise Doki flagged and the gitignore
  fix prevents — a code bot DID sweep the mirror into its commit. Re-running with
  the gitignore fix in place would produce a clean feature-only commit.
- Lessons for next run: Bmady is reliable + high-value for a scoped feature when
  the gates are driven attentively; the all-claude_code/opus pipeline avoided the
  forfait shape flakiness that bit Seki. Land the `.claude/skills/` gitignore fix
  (already on this branch) before using Bmady on a repo where the commit matters.
