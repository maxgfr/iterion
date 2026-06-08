# devbox-setup (Devy) — status: SCAFFOLD (manifest + skill ready; main.bot to build)

Devy authors a pinned `devbox.json` for a repo so its build/test/e2e run in a
reproducible toolchain (ADR-017 Tier-3). This bundle ships the **manifest**
and the **playbook skill** (`skills/devbox-setup.md` — the substance: detect
→ map to Nix → shape → validate → scope). The **`main.bot` (executable DSL)
is the remaining focused build** — deliberately not rushed, because the
smallest existing bot is ~685 lines of careful DSL (schemas, edges, sandbox)
and a half-correct one that doesn't `iterion validate` is worse than none.

## Intended workflow (build spec)

```
detect_stack  (agent, claude_code, read-only: read_file/glob/grep/bash)
  → reads manifests (go.mod, package.json, pyproject, …) + build/test/e2e
    commands; emits {languages, runtimes, build_cmd, test_cmd, e2e, pins}
generate_devbox (agent, claude_code, tools incl. write_file)
  → writes /workspace/devbox.json per skills/devbox-setup.md (pinned, minimal)
verify_devbox  (tool, deterministic)
  → `cd /workspace && devbox install` (must exit 0 + produce devbox.lock),
    then `devbox run -- <build>` / `<test>` smoke; degrade-with-report on fail
approve_devbox (human, default mode=propose)
  → show the devbox.json diff + verify output; approve → keep, reject → drop
done
```

- **sandbox**: an image with `nix` + `devbox` (today `ghcr.io/socialgouv/
  iterion-sandbox-sec:edge` has them; a slim `base+devbox+node` image is the
  ADR-017 target). Mount `~/.claude` for the claude_code backend.
- **worktree: auto** (it writes a file → isolate + gate before it lands).
- **vars**: `workspace_dir`, `apply_mode` (propose|apply), `devbox_model`.
- **idempotent**: if `/workspace/devbox.json` exists, propose a diff (add
  missing tools), never clobber existing pins (see skill §6).

## Why this is its own build (not a tail-end add)
A correct iterion bot needs validated schemas, C012-exhaustive edges, prompt
contracts, and a sandbox spec, then a real run to confirm the generated
devbox.json installs. That is a focused effort with `iterion validate` +
dogfood iterations — tracked as the next step, not bolted on.
