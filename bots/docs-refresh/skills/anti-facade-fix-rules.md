---
name: anti-facade-fix-rules
description: Discipline rules for docs-refresh fixers — verify code state, cite tool output, never rewrite code, escalate on ambiguity.
---

# Anti-façade fix rules

A "façade" fix is one that changes a doc to look like a fix without
actually verifying that the new doc text matches the code. Façades
are the dominant failure mode of LLM-driven docs work because the
metric ("the diff looks plausible, the next reviewer approves it")
can be satisfied without doing the underlying work. This skill
defines the discipline that prevents façades.

## The five rules

### Rule 1 — Verify state before writing

Before you write any character of replacement doc text, you must
have called `read_file` or `grep` to load the **actual current
code** at the `code_anchor` cited by the blocker. Your `summary`
field MUST contain the tool invocation (or a paraphrase tight
enough to reproduce) and a 3–5 line excerpt of the result.

A fix where the `summary` does not show evidence of having
consulted the code is treated as a façade by the next reviewer.

### Rule 2 — Reword from verified state, not from the old doc

Do not paraphrase the existing doc text and call it a fix. Read
the code, decide what the doc should say given the code, then
write that. The original doc text is evidence of past intent and
useful context, but it is not the ground truth — the code is.

### Rule 3 — Never edit code

You are forbidden from modifying any of:

- `.go`, `.ts`, `.tsx`, `.py`, `.rs`, `.js`, `.html`, `.css`,
  `.yaml` (except when a `.yaml` is in `doc_globs`)
- any file outside `scan_docs.doc_files[]`
- the **body** of a Go function (even when `go_comment_globs` is
  set, you only edit `//`-style or `/* */` comments — never
  statements or expressions)

If a fix appears to require a code change, you MUST instead set
`blocker.is_code_bug=true` on that finding and call `ask_user` to
escalate. The `fix_output.code_files_touched[]` field MUST be empty
on every run; non-empty means the bot's contract was broken.

### Rule 4 — Negative-space cleanup

When a fix deletes or rewrites a section because the underlying
feature/file/command is gone, you must also search for stale
cross-references to that section elsewhere in `doc_files[]`:

```bash
git grep -nF '<the removed identifier>' -- '*.md'
```

Fix or remove the cross-references in the same pass. A fix that
removes one mention of a dead concept but leaves five other
mentions intact is half a fix.

### Rule 5 — Escalate ambiguity, never guess intent

When the doc and code disagree and you cannot tell which is right
from the code alone (e.g. the doc says "by default we squash" but
the code has no default and asks at runtime — is "by default" the
common case or a misleading claim?), call `ask_user`. Do NOT pick
the interpretation that minimises your edit; that's the path-of-
least-resistance failure mode.

### Rule 6 — Pushback on inconsistent `(anchor_kind, code_anchor)`

The reviewer is supposed to emit blockers whose `anchor_kind` matches
the shape of `code_anchor` (see the consistency table in
`doc-mismatch-taxonomy.md`). If you receive a blocker where these
disagree — e.g. `anchor_kind: symbol` but `code_anchor: "<no longer
exists>"`, or `anchor_kind: external` but the anchor contains `:42` —
you cannot reliably verify it. Push it back with a justification
naming the inconsistency:

> Pushback: B3 has anchor_kind=symbol but code_anchor="<no longer
> exists>" — these are mutually exclusive. If the symbol is gone,
> the blocker's kind should be `removed` with an anchor like
> `"pkg/foo/old.go (removed in commit X)"`. Re-emit with the
> correct kind if the underlying mismatch is real.

Don't try to "fix it for the reviewer" by guessing what they meant.
The next reviewer will see the pushback and either correct the
classification or drop the blocker, both of which are healthier
outcomes than a fix you can't ground in real code state.

## Pushback is allowed; silent skip is not

If you receive a blocker you believe is not a real mismatch, you
may pushback by:

1. Leaving the doc unchanged.
2. Adding the exact blocker description to `fix_output.pushback[]`.
3. Writing a clear justification in
   `fix_output.pushback_justification`.

Pushback is the supported way to express "this isn't really a
mismatch" or "the reviewer misread the code." Silent skip (not
fixing, not pushing back) breaks the workflow's accounting.

## Sample summary (good)

> Fixed 2 blockers, pushed back 1.
>
> **B1 [stale_command, README.md:84-86]**: README claimed
> `iterion run --concurrency N`. Ran `grep -RIn '"concurrency"'
> cmd/iterion/ pkg/cli/run.go`; the flag is now `--max-parallel`
> (pkg/cli/run.go:142). Replaced the example in README.md with
> the current flag name and added a one-line note pointing to
> `--help` for the full list.
>
> **B2 [dead_link, docs/resume.md:12]**: link to
> `docs/recovery-modes.md` is dead — that file was renamed to
> `docs/recovery.md` (verified with `ls docs/recovery*`).
> Updated the link target and the anchor.
>
> **Pushback B3 [wrong_signature, docs/grammar/...]**: the
> reviewer flagged `func Parse(s string) (*AST, error)` as wrong;
> actually `pkg/dsl/parser/parser.go:Parse` matches that
> signature exactly (verified). Pushed back as a false positive.

## Sample summary (façade — would be rejected)

> Updated README.md and docs/resume.md to reflect current
> behaviour. Resolved all 3 blockers.

No `code_anchor`. No tool invocations. No evidence of having
consulted the code. The next reviewer will reject this and the
fixes will be re-checked.
