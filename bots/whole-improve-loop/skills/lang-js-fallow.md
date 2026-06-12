---
name: lang-js-fallow
description: Static analysis for JavaScript/TypeScript repos using fallow (dead code, duplication, complexity). Use when the target repo is JS/TS and you need to find unused code, clones, or complexity hotspots — e.g. under the improve-quality or code-quality preset.
---

# Static analysis for JS/TS with fallow

Use this skill when the target repository is JavaScript or TypeScript and the
task calls for finding dead code, duplication, or complexity hotspots — for
example under the `improve-quality` (SRE) or `code-quality` preset.

[fallow](https://docs.fallow.tools/quickstart) is a zero-config static analyzer
(122 built-in plugins covering Next.js, Vite, Jest, Tailwind, and more). It runs
straight from npm with no install or config step.

## When to use it

Detect a JS/TS repo first — look for any of: `package.json`, `tsconfig.json`, or
`*.ts` / `*.tsx` / `*.js` / `*.jsx` source files. If none are present, skip this
skill: fallow only understands JavaScript/TypeScript.

## Commands

Run from the repository root:

- `npx fallow` — full pass: dead code + duplication + health in one run.
- `npx fallow dead-code` — unused files, exports, types, dependencies,
  circular dependencies, and module-boundary violations.
- `npx fallow dupes` — duplicated logic; add `--mode semantic` to catch
  variable-renamed clones.
- `npx fallow health` — complexity hotspots ranked as refactoring targets.
- `npx fallow fix --dry-run` — preview the automatic cleanup before applying.
- Append `--format json` to any command for machine-readable output you can
  parse and act on programmatically.

## How to apply the findings

1. Run `npx fallow dead-code --format json` and `npx fallow health --format
   json` from the repo root.
2. Treat confirmed dead exports / files and circular dependencies as removable —
   delete them, then re-run to confirm nothing else broke.
3. For complexity hotspots, prefer the smallest clear refactor (extract a
   function, collapse a branch) over a rewrite.
4. For duplication, factor out the common logic only when the clones are truly
   the same intent — do not over-abstract incidental similarity.
5. Re-run the relevant command after each batch of fixes; the finding count
   going down is your gate.

Never let fallow's autofix run unreviewed against real code — preview with
`fallow fix --dry-run` and apply changes deliberately.
