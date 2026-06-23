# branch_improve_loop

Branch-scoped variant of `whole_improve_loop`. Runs the alternating
Claude/GPT review-fix loop against the diff between the current branch and
its base, auto-commits on convergence, and stops on cross-family
double-approval.

On large PRs the reviewer used to ingest the whole diff in one context,
overflow the window, and never converge. A deterministic `plan_chunks`
step now measures the diff and, above a threshold, splits it into small
diff chunks the reviewer reads one at a time before merging them into a
single whole-diff verdict (see [Large PRs](#large-prs) below).

## Inputs

All inputs are workflow `vars` (override with `--var name=value`):

| Var | Default | Description |
|---|---|---|
| `workspace_dir` | `${PROJECT_DIR}` | Repo to review (the run's workspace). |
| `base_ref` | `main` | Branch/ref to diff against; scope is `git diff {{base_ref}}...HEAD`. |
| `scope_notes` | `""` | Free-text guidance passed to the reviewers. |
| `chunk_threshold_loc` | `300` | Activate chunked review above this many changed LoC. |
| `chunk_max_loc` | `200` | Maximum changed LoC per chunk. |
| `chunk_dir` | `.branch-improve-chunks` | Gitignorable scratch dir for chunk diffs + manifest. |

## Run

```bash
iterion run bots/branch_improve_loop/main.bot \
  --var workspace_dir=/path/to/repo \
  --var base_ref=main
```

## Large PRs

When `git diff {{base_ref}}...HEAD` exceeds `chunk_threshold_loc` changed
lines, the `plan_chunks` tool splits the diff into `≤ chunk_max_loc` LoC
unified-diff files under `chunk_dir` (a single file larger than the cap is
hunk-split), plus a `manifest.json` mapping each chunk to its files. Then:

- **Review** — the reviewer reads the chunks one at a time, records one
  finding per chunk (`chunk_notes`), and **merges** them into a single
  whole-diff verdict.
- **Converge** — cross-family double-approval (`streak_check`) is computed
  on that merged WHOLE-diff verdict, never chunk-by-chunk, so the loop's
  stop condition is unchanged.
- **Fix** — the same-family fixer streams the per-chunk notes and opens
  only the relevant chunk diff files, never reloading the whole diff.

At or below the threshold the bot keeps its original single-pass
whole-diff behaviour. `chunk_dir` is scratch — gitignore it; the commit
step excludes it. Design rationale and the rejected alternatives parallel the
whole-repository chunking case in
[ADR-011](../../docs/adr/011-whole-improve-loop-context-chunking.md).

See [main.bot](main.bot) for the full DSL.
