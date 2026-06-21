# rtk — command-output compression (token saver)

[rtk](https://github.com/rtk-ai/rtk) ("Rust Token Killer") is a single static
Rust binary that rewrites a dev command into a token-compressed equivalent —
e.g. `git status` → `rtk git status` — filtering, grouping, truncating and
deduplicating the output before it reaches the LLM. On noisy commands (git,
cargo, npm/pnpm, pytest/jest, `go test`, grep/find, docker/kubectl, …) it saves
**60–90 %** of the output tokens an agent would otherwise spend. iterion can use
it as an **opt-in** compressor across all three of its shell-execution surfaces.

It is **off by default**. Nothing changes until you enable it, and if the `rtk`
binary is not installed every code path falls back to the original command — so
enabling rtk on a host without it is a safe no-op.

## Install the binary

rtk is a normal CLI; install it once on the host that runs iterion:

```sh
# script (installs to ~/.local/bin/rtk)
curl -fsSL https://raw.githubusercontent.com/rtk-ai/rtk/master/install.sh | sh
# or: brew install rtk   |   cargo install --git https://github.com/rtk-ai/rtk
```

iterion locates the binary via `ITERION_RTK_BIN`, then `PATH`, then the
conventional install dirs (`~/.local/bin/rtk`, `/usr/local/bin/rtk`,
`/usr/bin/rtk`). Verify with `rtk --version` (must be ≥ 0.23 — that's when
`rtk rewrite`, the integration primitive, was added).

## Turning it on

Precedence (highest first): **run override → node `rtk:` → workflow `rtk:` →
`ITERION_RTK` env → off**.

| Surface | How |
|---|---|
| Whole process | `export ITERION_RTK=on` (or `ultra`, or `off`) |
| One run (CLI) | `iterion run bot.bot --rtk on` |
| One run (studio) | the **rtk** control in the Launch modal |
| A workflow | `rtk: on` in the `workflow <name>:` block |
| One node | `rtk: on` on an `agent` / `judge` / `tool` node |

Values: `on` (standard rewrite), `ultra` (rtk's densest output, `--ultra-compact`),
`off` (disabled). Anything else is a compile error (diagnostic **C102**).

```
workflow demo:
  rtk: on            # every claude_code / claw node in this run compresses

agent implement:
  backend: claude_code
  rtk: ultra         # this node overrides the workflow with ultra-compact
```

## The three surfaces

All three route through one primitive — rtk's own `rtk rewrite "<cmd>"`
subcommand — so iterion never second-guesses which commands rtk supports.

1. **claude_code** (implementers/fixers). iterion installs a `PreToolUse` hook
   on the Bash tool that rewrites the command to its `rtk <cmd>` form and
   auto-allows it. This is rtk's native integration, driven through iterion's
   own hook plumbing (no `~/.claude/settings.json` edit required).
2. **claw** (in-process LLM backend). The bash builtin rewrites its `command`
   before executing. This is especially valuable for claw, whose bash tool
   hard-truncates output at 10 000 bytes — compression keeps more signal under
   that cap.
3. **tool nodes** (deterministic shell). **Node-level opt-in only**: a tool
   node compresses *only* when its own `rtk:` field is `on`/`ultra`. A run
   override can force-*disable* (kill switch) but never force-*enable* a tool
   node. This protects deterministic tool output — e.g. a review loop's
   `git diff` feeding a reviewer — from being silently compressed by a global
   toggle, which would break review fidelity (see
   [workflow_authoring_pitfalls.md](workflow_authoring_pitfalls.md)).

### Compressor, never a gate

rtk's permission model (allow/ask/deny verdicts) is **not** used by iterion.
iterion runs delegated agents under `bypassPermissions` with its own
tool-policy + secret-guard, and treats rtk purely as an output compressor: it
applies the rewrite whenever `rtk rewrite` emits one (exit 0 *or* 3 — under
default rtk config a rewritable command yields exit 3/"Ask", not 0), and runs
the original command otherwise (exit 1/2 or any failure). A `deny`/`ask`
verdict never blocks an iterion command.

## Sandbox

When a run is sandboxed, the rewrite *decision* happens host-side (claude_code
hook / tool node) or in-container (sandboxed claw runner), but the rewritten
`rtk <cmd>` always *executes* inside the container. iterion therefore
bind-mounts a host `rtk` binary at `/usr/local/bin/rtk` whenever one is present
(`addRtkBinaryMount`), so the decision and the execution can never disagree.
The Linux release is a static musl binary, so the host binary runs as-is in the
`iterion-sandbox-slim`/`-full` images.

- **Different host/container arch** (e.g. macOS host, Linux container): the host
  binary won't run in the container — bake rtk into a custom sandbox image
  instead (same caveat as the bind-mounted `iterion` binary).
- **Cloud / kubernetes** (no host filesystem): there is nothing to bind-mount;
  bake rtk into the sandbox + runner images.

## Configuration & telemetry

rtk reads its own config from `~/.config/rtk/config.toml` (e.g.
`[hooks] exclude_commands = ["curl", "playwright"]`). iterion does not manage
that file; configure rtk there as you would for interactive use.

iterion sets `RTK_TELEMETRY_DISABLED=1` on every rtk invocation by default
(matching iterion's no-telemetry posture). os.Environ() is inherited first, so
an operator who deliberately re-enables telemetry in their own environment still
wins.

## Environment variables

| Var | Meaning |
|---|---|
| `ITERION_RTK` | process-wide default mode: `on` \| `ultra` \| `off` (default off) |
| `ITERION_RTK_BIN` | explicit path to the rtk binary (overrides PATH lookup) |
| `RTK_TELEMETRY_DISABLED` | rtk's own telemetry switch; iterion sets `=1` by default |

## Implementation pointers

- Primitive + mode/precedence: [pkg/backend/rtk/rtk.go](../pkg/backend/rtk/rtk.go)
- claude_code hook: [pkg/backend/delegate/claude_code.go](../pkg/backend/delegate/claude_code.go)
- claw bash builtin: [pkg/backend/tool/claw_builtins.go](../pkg/backend/tool/claw_builtins.go)
- tool nodes: [pkg/backend/model/executor_tool.go](../pkg/backend/model/executor_tool.go)
- per-node resolution: [pkg/backend/model/executor.go](../pkg/backend/model/executor.go) (`buildTask`)
- sandbox mount: [pkg/runtime/sandbox_mounts.go](../pkg/runtime/sandbox_mounts.go) (`addRtkBinaryMount`)
- DSL field: `rtk:` in `pkg/dsl/` (parser/ast/ir/compile/validate/unparse)
