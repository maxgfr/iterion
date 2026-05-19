# sec-audit-deps

Universal supply-chain malware auditor — a bundled iterion workflow
that enumerates installed dependencies, runs static heuristics
adapted to each ecosystem, hands the structured signals to an LLM
reviewer with a strict JSON output contract, and emits one kanban
issue per dependency flagged MEDIUM or HIGH.

Inspired by
[SocialGouv/no-package-malware](https://github.com/SocialGouv/no-package-malware) —
the static-signals → LLM-with-schema pattern, generalised from npm to
multiple ecosystems and lifted out of the Verdaccio gateway it
originally shipped with.

## What it inspects

| Ecosystem | Manifest | Lifecycle hooks looked for | Vuln DB |
|---|---|---|---|
| npm / yarn / pnpm | `package.json`, `node_modules/**/package.json` | `preinstall`, `install`, `postinstall`, `prepare` | `npm audit --json` |
| pip / poetry / uv | `setup.py`, `pyproject.toml`, installed dist-info | `setup()` calls, custom commands | `pip-audit --format=json` |
| Go modules | `go.mod`, `go.sum`, `vendor/` | suspicious `replace` directives, `init()` side-effects | `govulncheck -json ./...` |
| Generic | extracted tarballs / wheels | embedded binaries, base64 blobs, locale anomalies, fetch+exec patterns | — |

Per-ecosystem coverage is documented in `skills/lang-*.md`. Add an
ecosystem by adding one skill + one router branch — see *Adding an
ecosystem* at the bottom.

## Quick start

```bash
devbox run -- iterion run examples/sec-audit-deps/main.bot \
  --var workspace_dir=$(pwd) \
  --var severity_threshold=medium

# Outputs:
#  - kanban issues: ready / labels = severity:* + type:supply-chain-* + ecosystem:* + source:sec-audit-deps
#  - host cache appended at ~/.iterion/security-cache/packages.jsonl
#  - markdown export at <store-dir>/runs/<run_id>/artifacts/export_report/findings.md
```

## Cross-run memory — host-wide package cache

A package version is a universal artifact: `left-pad@1.3.0` is the
same tarball whether you `npm install` it in repo A or repo B. The
cache lives at `~/.iterion/security-cache/packages.jsonl` so every
repo on the host benefits from past analysis.

Schema (one JSON object per line, append-only):

```json
{"ecosystem":"npm","name":"left-pad","version":"1.3.0","checksum":"sha256:...","scanned_at":"2026-05-19T10:00:00Z","risk_score":3,"risk_level":"LOW","flags":[],"scanner_version":"sec-audit-deps@0.1.0"}
```

The cache key is `ecosystem:name:version:checksum`. The
`load_package_cache` compute node loads the file in O(n), builds an
index, and the `filter_cached` compute node splits pending deps into
*already analysed at acceptable scanner_version* (skip) and *new or
stale* (scan).

The cache is **auto-mounted into the sandbox** when
`host_state: auto` is in effect (the default), so sandboxed runs
share the cache transparently. Pass `--sandbox-host-state=none` to
opt out — useful in multi-tenant cloud runners that must not share
operator state.

## Pipeline

```
enumerate_deps (claw, readonly)
  └─→ load_package_cache (compute: read ~/.iterion/security-cache/packages.jsonl)
  └─→ filter_cached (compute: split into already_scanned[] vs pending[])
  └─→ heuristic_scan (router fan_out_all on ecosystems present)
        ├─→ run_js_heuristics      (tool: parse package.json scripts, decode `*-install.js` payloads)
        ├─→ run_py_heuristics      (tool: parse setup.py / pyproject.toml)
        ├─→ run_go_heuristics      (tool: scan go.mod / go.sum / vendor/)
        └─→ run_generic_heuristics (tool: binary entropy, base64 blobs, locale anomalies)
  └─→ llm_review (claude_code, readonly, capabilities: board.create + board.label)
  └─→ score_merge (compute: max(heuristic, llm) → bucket LOW/MEDIUM/HIGH)
  └─→ update_package_cache (tool: append fresh lines to packages.jsonl)
  └─→ export_report (compute → markdown)
  └─→ done
```

## Adding an ecosystem

1. **Skill**: `skills/lang-<ecoid>.md` — manifest path, lockfile
   path, lifecycle-hook patterns, vuln DB command. Mirror `lang-js.md`.

2. **Heuristic tool node**: `tool run_<ecoid>_heuristics:` —
   emits JSON `{packages: [{name, version, checksum, signals: [...]}]}`.

3. **Router branch**: add a `... when tech.has_<ecoid>` branch
   under `heuristic_scan`.

No DSL primitive changes, no Go runtime changes. Pure composition.

## See also

- [sec-audit-source](../sec-audit-source/) — sibling bundle for in-repo source code.
- [docs/security-bots.md](../../docs/security-bots.md) — shared threat model + ops guide.
