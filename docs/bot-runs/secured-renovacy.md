# secured-renovacy (Renovacy) — dogfood bilan

Index + template: [README.md](README.md). Newest first.

## 2026-06-14 — first full validated run (safe mode, run 019ec5c5)

- Status: **validated.** Full end-to-end run in safe mode on a clean iterion
  clone; real upgrades applied, vendored, validated, committed to a storage
  branch — major bumps correctly skipped.
- Versions: iterion branch `c082-board-emit` (C082 worktree binary) ·
  secured-renovacy current.
- Method: dedicated worktree studio :4899, `worktree: auto` on a clean iterion
  clone, `sandbox: iterion-sandbox-sec:edge`. Safe-mode vars:
  `major_policy=skip`, `scope=patch,minor`, `max_packages_per_run=3`,
  `merge_into=none`. (`major_policy=skip` honours the standing "ask before
  `major_policy: attempt`" rule.)
- Result: `Run finished`; `final_commit be365eab` on storage branch
  `iterion/run/comet-haze-arctickazoo-3b09` (not merged — `merge_into=none`).
  Four commits: a batch of **15 patch upgrades**, plus minors
  `aws-sdk-go-v2/config v1.32.25`, `aws-sdk-go-v2/credentials v1.19.24`, and
  **`golang-jwt/jwt/v5 → v5.3.1`** (a security-relevant JWT lib bump). `vendor/`
  regenerated (117 files, +11257/-2927). Pipeline ran end-to-end:
  detect_stack → discover_outdated → bucket_patches → batch_upgrade_patches →
  install/validate → … → emit_sbom → done. No human pause was needed in safe mode.
- Value: **high.** Real, correctly-tiered dependency hygiene (patch+minor only,
  major skipped) with vendoring + a per-upgrade commit trail, on a repo it had
  never seen. The golang-jwt bump is the kind of security-relevant update this
  bot exists to surface.
- Robustness finding (positive): **devbox silently fails in the sec sandbox** —
  `~/.cache` is root-owned, so `devbox run …` returns EMPTY output, which a naive
  bot would read as "all dependencies up to date" (a façade). Renovacy's
  `discover_outdated` agent **detected the silent failure**, fell back to the
  image's `/usr/bin/go` (go1.26.0, matching go.mod) with writable `/tmp` caches,
  and **warned the downstream upgrade/install agents** to do the same. That's
  exactly the anti-façade behaviour the workflow-authoring pitfalls doc calls for.
- Engine/sandbox finding (to fix): the devbox wall above affects ANY sandboxed
  devbox-based bot (e.g. Devy's `devbox install` verify would hit it too). Root
  cause is `~/.cache` ownership inside the sec sandbox under `user: 1000:1000`
  with host-state mounts. Worth fixing (ensure `~/.cache` is writable by the
  container UID, or point devbox/Nix caches at a writable dir) so devbox bots
  don't depend on a host-go fallback.
- Finding (minor, recurring): claude_code nodes emit a spurious
  `Tool error: StructuredOutput — No such tool available: StructuredOutput`
  before recovering via iterion's fmt-pass — same family seen in Billy/Devy.
  Non-fatal here.
- Lessons for next run: safe mode (`major_policy=skip`, `scope=patch,minor`,
  small `max_packages_per_run`) is a reliable, bounded, valuable config. Fix the
  sandbox devbox `~/.cache` wall so detection doesn't rely on an agent noticing
  the silent failure. For a major-bump run, get explicit operator sign-off first.
