[← Documentation index](README.md) · [← Iterion](../README.md)

# CLI Reference

All commands support `--json` for machine-readable output and `--help` for usage details.

## `iterion init`

Scaffold a new project with an example workflow:

```bash
iterion init              # Current directory
iterion init my-project   # New directory
```

Creates `pr_refine_single_model_backend.bot`, `.env.example`, and `.gitignore`. Idempotent — won't overwrite existing files.

## `iterion validate`

Parse, compile, and validate a workflow without running it:

```bash
iterion validate workflow.iter
```

Reports errors and warnings with diagnostic codes (C001–C072, sparse — see [references/diagnostics.md](references/diagnostics.md) for the authoritative list), file positions, and descriptions.

## `iterion run`

Execute a workflow:

```bash
iterion run workflow.iter [flags]
```

| Flag | Description |
|------|-------------|
| `--var key=value` | Set workflow variable (repeatable) |
| `--recipe <file>` | Apply a recipe preset (JSON) |
| `--preset <name>` | Apply a named in-source preset from the workflow's `presets:` block before `--var` overrides |
| `--run-id <id>` | Use a specific run ID (default: auto-generated) |
| `--store-dir <dir>` | Run store directory (default: `.iterion`) |
| `--timeout <duration>` | Global timeout (e.g. `30m`, `1h`) |
| `--log-level <level>` | Log verbosity: `error`, `warn`, `info`, `debug`, `trace` |
| `--no-interactive` | Don't prompt on TTY; exit on human pause |
| `--sandbox <mode>` | Sandbox override: `auto` (read `.devcontainer/devcontainer.json`) or `none` (force off). Empty inherits `ITERION_SANDBOX_DEFAULT` then the workflow's `sandbox:` block |
| `--sandbox-default-image <image>` | Image ref used by `sandbox: auto` when no `.devcontainer/devcontainer.json` is found (overrides `ITERION_SANDBOX_DEFAULT_IMAGE`) |
| `--merge-into <target>` | For `worktree: auto` runs — `current` (default), `none` (skip merge, branch only), or a branch name |
| `--branch-name <name>` | For `worktree: auto` runs — override the storage branch name (default `iterion/run/<friendly-name>`) |
| `--merge-strategy <mode>` | For `worktree: auto` runs — `squash` (default, collapses run commits into one) or `merge` (fast-forward, preserves history) |
| `--auto-merge` | For `worktree: auto` runs — apply `--merge-strategy` at run end (default `true` on the CLI; the editor sets `false` and defers the merge to a UI action) |

## `iterion inspect`

View run state and history:

```bash
iterion inspect                          # List all runs
iterion inspect --run-id <id>            # View a specific run
iterion inspect --run-id <id> --events   # Include event log
iterion inspect --run-id <id> --full     # Show full artifact contents
```

## `iterion resume`

Resume a paused workflow run with human answers:

```bash
iterion resume --run-id <id> --file workflow.iter --answer key=value
iterion resume --run-id <id> --file workflow.iter --answers-file answers.json
```

See [resume.md](resume.md) for the full failure / resume matrix.

## `iterion diagram`

Generate a Mermaid diagram from a workflow:

```bash
iterion diagram workflow.iter              # Compact view (default)
iterion diagram workflow.iter --detailed   # Include node properties
iterion diagram workflow.iter --full       # Include templates and loop details
```

Paste the output into any Mermaid-compatible renderer (GitHub Markdown, [Mermaid Live Editor](https://mermaid.live), etc.).

## `iterion bundle`

Create and inspect `.botz` workflow bundles (see [bundles.md](bundles.md)):

```bash
iterion bundle init my-bot              # Scaffold a bundle source directory
iterion bundle pack my-bot              # Build my-bot.botz next to the source
iterion bundle pack my-bot -o out.botz  # Choose the output archive
```

`iterion bundle pack` flags:

| Flag | Description |
|------|-------------|
| `-o, --output <file>` | Output `.botz` path (default: `<dir>.botz` next to the source) |
| `--force` | Overwrite the output file if it already exists |

## `iterion report`

Generate a chronological report for a completed run:

```bash
iterion report --run-id <id>
iterion report --run-id <id> --output report.md
```

The report includes:
- **Summary table** — workflow name, status, duration, tokens, cost, model calls
- **Artifacts table** — all published artifacts with versions
- **Timeline** — chronological reconstruction of every node execution, edge selection, verdict, branch lifecycle, and budget warning

## `iterion editor`

Launch the visual workflow editor:

```bash
iterion editor                     # Default port 4891
iterion editor --port 8080         # Custom port
iterion editor --dir ./workflows   # Custom directory
iterion editor --no-browser        # Don't auto-open browser
```

See [visual-editor.md](visual-editor.md) for features.

## `iterion conduct`

Run the conductor daemon: poll a tracker, dispatch eligible issues to a workflow, and expose the optional REST/WebSocket surface (see [conductor.md](conductor.md)):

```bash
iterion conduct iterion.conductor.yaml
iterion conduct iterion.conductor.yaml --port 4892
iterion conduct iterion.conductor.yaml --no-server
```

| Flag | Description |
|------|-------------|
| `--store-dir <dir>` | Override the iterion store directory |
| `--port <port>` | HTTP port for the conductor REST/WS surface (overrides `server.port` in config) |
| `--no-server` | Run headless — disable the HTTP surface even if `server.port` is set |

## `iterion issue`

Manage the native kanban tracker used by the conductor (see [native-tracker.md](native-tracker.md)):

```bash
iterion issue create --title "Fix auth" --label backend --priority 10
iterion issue list --state todo --unclaimed
iterion issue show <id-or-prefix>
iterion issue move <id-or-prefix> --to doing
iterion issue update <id-or-prefix> --title "New title" --field bot=review
iterion issue close <id-or-prefix>
iterion issue board show
iterion issue board init --force
```

All `iterion issue` subcommands accept the persistent `--store-dir <dir>` flag.

Common subcommands and flags:

| Command | Flags |
|---------|-------|
| `issue create` | `--title <text>` (required), `--body <text>`, `--state <state>`, `--label <label>` (repeatable), `--priority <n>`, `--assignee <name>`, `--blocker <id>` (repeatable), `--field key=value` (repeatable) |
| `issue list` | `--state <state>` (repeatable), `--label <label>` (repeatable), `--assignee <name>`, `--claimed`, `--unclaimed` |
| `issue move <id-or-prefix>` | `--to <state>` (required) |
| `issue update <id-or-prefix>` | `--title <text>`, `--body <text>`, `--labels <csv>`, `--priority <n>`, `--assignee <name>`, `--blockers <csv>`, `--field key=value` (repeatable), `--clear-field <key>` (repeatable) |
| `issue board init` | `--from <board.json>`, `--force` |

## `iterion bench asymptote`

Measure inter-session quality stabilisation curves from persisted runs (see [asymptote-bench.md](asymptote-bench.md)):

```bash
iterion bench asymptote --runs r1,r2,r3 --judge-node final_judge --output report.md
iterion bench asymptote --runs r1,r2 --variant-runs r3,r4 --judge-node final_judge
```

| Flag | Description |
|------|-------------|
| `--store-dir <dir>` | Store directory (default: `.iterion`) |
| `--runs <ids>` | Comma-separated run IDs of the same workflow |
| `--variant-runs <ids>` | Comma-separated run IDs of an alternative recipe variant |
| `--label <name>` | Primary group label (default: `asymptote`) |
| `--variant-label <name>` | Variant group label (default: `variant`) |
| `--judge-node <id>` | IR node ID of the judge whose verdicts will be scored (required) |
| `--judge-field <field>` | Output field on the judge node carrying the verdict (default: `approved`) |
| `--loop <name>` | Restrict scoring to one bounded loop name (default: first observed) |
| `--approval-threshold <n>` | Score threshold for the approved flag (default: `0.5`) |
| `--output <file>` | Markdown output file (`-` or empty for stdout) |
| `--title <text>` | Report title (default: `Asymptote Benchmark`) |
| `--include-per-run` | Append a per-run iteration list at the end |

## `iterion sandbox`

Inspect and configure the iterion sandbox subsystem (see [sandbox.md](sandbox.md)):

```bash
iterion sandbox doctor   # Report the active driver (Docker/Podman), image cache, and capabilities
```

## `iterion server`

Start the long-running HTTP server (editor SPA + run console + cloud API). Used both for the local web editor and for cloud mode deployments — install via [`oci://ghcr.io/socialgouv/charts/iterion`](https://github.com/SocialGouv/iterion/pkgs/container/charts%2Fiterion) (chart sources in [`charts/iterion/`](../charts/iterion/)).

## `iterion runner`

Run a cloud-mode runner pod that consumes workflows from the NATS queue. Configured via `pkg/config/` env vars; deployed by the Helm chart with KEDA-based autoscaling.

## `iterion version`

Print version and commit hash.
