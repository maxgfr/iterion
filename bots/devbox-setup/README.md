# devbox-setup (Devy) — status: v1 (manifest + skill + main.bot validate; run-test pending)

Devy authors a pinned `devbox.json` for a repo so its build/test/e2e run in a
reproducible toolchain (ADR-017 Tier-3). This bundle ships the **manifest**,
the **playbook skill** (`skills/devbox-setup.md` — the substance: detect →
map to Nix → shape → validate → scope), and the **`main.bot`** (linear
detect → generate → verify → done; `iterion validate` OK, 5 nodes). It is
**not yet run-tested** — a real run needs a target repo and a stable network
for the cold `devbox install`; that dogfood is the remaining step.

## Intended workflow (build spec)

```
detect_stack  (agent, claude_code, read-only: read_file/glob/grep/bash)
  → reads manifests (go.mod, package.json, pyproject, …) + build/test/e2e
    commands; emits {languages, runtimes, build_cmd, test_cmd, e2e, pins}
generate_devbox (agent, claude_code, tools incl. write_file)
  → writes /workspace/devbox.json per skills/devbox-setup.md (pinned, minimal)
verify_devbox  (tool, deterministic)
  → `cd <workspace> && devbox install` (must exit 0 + produce devbox.lock);
    degrade-with-report on fail
done
```

- **sandbox**: an image with `nix` + `devbox` (today `ghcr.io/socialgouv/
  iterion-sandbox-sec:edge` has them; a slim `base+devbox+node` image is the
  ADR-017 target). Mount `~/.claude` for the claude_code backend.
- **worktree: auto** (it writes a file → isolate + gate before it lands).
- **vars**: `workspace_dir`, `apply_mode` (propose|apply), `devbox_model`.
- **idempotent**: if `/workspace/devbox.json` exists, propose a diff (add
  missing tools), never clobber existing pins (see skill §6).

## v1 scope + next
v1 is the linear flow above (detect → generate → verify → done); the
worktree + PR review is the gate. Next enhancements: an in-bot `human`
approve_devbox gate + an `apply_mode` (propose | apply), and a real dogfood
run (a target repo + a stable network for the cold `devbox install`) to
confirm the generated devbox.json installs.
