# ADR-013: `dispatch.attachments` is unsupported — fail fast instead of silently dropping it

- **Status**: Accepted
- **Date**: 2026-06-02
- **Authors**: devthejo
- **Code context**: [`pkg/dispatcher/config.go`](../../pkg/dispatcher/config.go)
  (`Config.Validate` attachments rejection, `unsupportedAttachmentsErr`,
  `DispatchConfig.Attachments` detection-only field),
  [`pkg/dispatcher/loop.go`](../../pkg/dispatcher/loop.go) (`buildSpec` — the
  removed render block), [`pkg/dispatcher/runner.go`](../../pkg/dispatcher/runner.go)
  (the removed `DispatchSpec.Attachments` field),
  [`docs/dispatcher.md`](../dispatcher.md) ("Dispatch templates").
  Related: [ADR-011 dispatcher retry attempt cap](015-dispatcher-retry-attempt-cap.md)
  (same theme — converting a silent dispatcher failure into an honest signal).

## Context

`dispatch.attachments` (and its per-assignee twin
`assignee_dispatch[].attachments`) was documented as a working input that
"maps workflow inputs to per-issue values", declared on `DispatchConfig`,
validated at config load (the template parsed), rendered per dispatch in
`buildSpec`, and stored on `DispatchSpec.Attachments`.

It then went **nowhere**. `EngineRunner.Dispatch` passes only `spec.Vars`
to the engine; no runner ever read `spec.Attachments`, and a repo-wide grep
confirmed the field had zero readers. An operator who followed the docs and
set `dispatch.attachments` got a config that validated cleanly, a value that
rendered, and a bot that ran **without** the attachment — no error, no
warning. That is exactly the silent context-loss this audit targets, made
worse by the asymmetry that the sibling `bot_args` path *does* warn when a
key won't reach the workflow, and the studio/server launch path *does* honor
attachments (via `WithAttachmentPromote`). The dispatcher was the only input
path that lost them.

The root cause is a semantic gap, not a missing wire. Workflow attachments
are **binary files** (`pkg/dsl/ir.Attachment`, kinds `file`/`image`)
referenced as `{{attachments.<name>.path}}` / `.url` / `.mime` / `.size` /
`.sha256`. Runtime promotion (`store.WriteAttachment` +
`runtime.WithAttachmentPromote`) materialises **bytes** — from an uploaded
file (launch path) or a bundle file (`promoteBundleAttachmentDefaults`).
`dispatch.attachments`, by contrast, renders a **template string**. There is
no defined mapping from a rendered string to an attachment's bytes:

- Is the string the attachment's **inline content** (write it as the file)?
- A **filesystem path** to open and copy?
- An **upload id** like the launch path?

Each interpretation produces different — and silently *wrong* — bytes if
guessed incorrectly, and an `image` attachment cannot be produced from a
string at all. There is no example, test, or doc that pins the intended
semantic; the feature was never functional, so there is no prior behaviour
to preserve.

## Decision

Treat `dispatch.attachments` / `assignee_dispatch[].attachments` as a
**load-time error**. `Config.Validate` now rejects any non-empty attachments
block with an actionable message that names the field, explains why it can't
be honored, and points the operator at `dispatch.vars` / a ticket's
`bot_args` for per-issue context. The dead plumbing is removed:

- `buildSpec` no longer renders attachments (a config that reaches it has
  none — Validate already refused otherwise).
- `DispatchSpec.Attachments` (the write-only field) is deleted.
- The `docs/dispatcher.md` "Dispatch templates" section drops the false
  claim and documents the unsupported status explicitly.

`DispatchConfig.Attachments` is **kept** purely so the rejection can fire:
`Load` uses non-strict `yaml.Unmarshal`, so deleting the struct field would
make a stray `attachments:` key parse-and-vanish, silently reintroducing the
bug. The field exists only to be detected and refused.

This is safe at both entry points: at startup `Load` returns the error and
`iterion dispatch` exits; on hot-reload `ConfigWatcher` logs "config reload
failed, keeping previous" and leaves the running config untouched — a
misconfigured edit can't crash a live daemon.

### Alternatives rejected

1. **Wire it (the reviewer's preferred option).** Materialise each rendered
   value as an attachment via `WriteAttachment` + `WithAttachmentPromote`,
   mirroring the launch path. Rejected **for now** because it requires first
   *pinning a semantic* (content vs path vs upload id) that nothing in the
   tree defines, handling the `image` type (impossible from a string),
   deriving MIME/filename, reconciling against compile-time-declared
   attachments, and validating end-to-end through the executor's
   `{{attachments.x.path}}` substitution (a live-LLM path). Guessing the
   semantic risks feeding the bot the wrong bytes — trading a *visible*
   silent-drop for an *invisible* silent-corruption, which is strictly worse
   on the reliability axis. Wiring is the right follow-up once the semantic
   is a deliberate product decision; it is a larger change than this audit's
   "small, safe fix" bar.
2. **Warn at dispatch and continue.** Rejected: a per-dispatch `WARN` lands
   in daemon logs the operator may never read, and the run still proceeds
   with the context missing. The reviewer explicitly called this out as
   insufficient. A load-time hard error is impossible to miss and stops the
   run before it produces misleading output.
3. **Delete the config field and the docs, keep no guard.** Rejected: with
   non-strict YAML the key would parse-and-vanish, so an operator who still
   had `attachments:` in their file would be back to a silent no-op — the
   exact failure mode being fixed.
4. **Leave it as-is and only document the limitation.** Rejected: the field
   validated and rendered, so every signal told the operator it worked. Docs
   alone don't stop the silent drop at runtime.

The non-obvious trade-off: we remove a documented capability rather than
implement it. We chose **honesty over feature coverage** — a clear "not
supported, use vars/bot_args" beats a feature that silently does nothing —
and deferred the real implementation to a future ADR that pins the semantic.

## Consequences

- A dispatcher config that declares `dispatch.attachments` (or
  `assignee_dispatch[].attachments`) now **fails to load** with a precise,
  actionable error instead of silently dropping the context. This is a
  behaviour change only for configs that were already broken (the block
  never reached a run).
- Per-issue context flows through `dispatch.vars` and a ticket's `bot_args`,
  both of which are wired into the run and (for `bot_args`) warn on
  undeclared keys.
- No frontend change: the studio Dispatcher dashboard is read-only and never
  exposed an attachments editor.
- Re-introducing attachment support is a clean follow-up: pin the semantic,
  build the promote func in `EngineRunner.Dispatch`, restore the
  `DispatchSpec` field + `buildSpec` render, and flip this ADR's status with
  a dated supersede entry.
