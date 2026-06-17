import { describe, expect, it } from "vitest";

// Regression guard for phantom / legacy Tailwind utility classes — names
// that have NO matching `--color-*` token in app.css, so Tailwind generates
// no rule and the element renders with no colour (an invisible button label,
// an unstyled primary button, a missing progress track…). Yesterday's UX
// round fixed 44 of these by hand; this gate keeps them — and the next batch
// (bg-fg-accent / text-on-accent / text-fg-warn|success / text-text-* /
// bg-bg-* / bare border-border) — from creeping back.
//
// This bans only names known to be undefined. Adding a *real* token to
// app.css never requires touching this list. The canonical replacements:
//   bg-fg-accent      -> bg-accent
//   text-on-accent    -> text-fg-onAccent
//   text-fg-warn      -> text-warning-fg
//   text-fg-success   -> text-success-fg
//   text-fg-error     -> text-danger-fg
//   text-text-1/2     -> text-fg-default / text-fg-muted
//   bg-bg-*           -> bg-surface-*
//   border-border     -> border-border-default

const BANNED: { label: string; re: RegExp }[] = [
  { label: "bg-fg-accent — use bg-accent (no --color-fg-accent token)", re: /\bbg-fg-accent\b/ },
  { label: "text-on-accent — use text-fg-onAccent", re: /\btext-on-accent\b/ },
  { label: "text-fg-warn — use text-warning-fg", re: /\btext-fg-warn\b/ },
  { label: "text-fg-success — use text-success-fg", re: /\btext-fg-success\b/ },
  { label: "text-fg-error — use text-danger-fg", re: /\btext-fg-error\b/ },
  { label: "text-text-* — legacy double-prefix; use text-fg-*", re: /\btext-text-/ },
  { label: "bg-bg-* — phantom double-prefix; use bg-surface-*", re: /\bbg-bg-/ },
  { label: "bare border-border — use border-border-default", re: /\bborder-border(?![-\w])/ },
];

// Load every source module as raw text via Vite's glob (works in vitest's
// node + jsdom envs, no node:fs). Test files are excluded — this file holds
// the banned names as regex data and must not match itself.
const RAW = import.meta.glob("/src/**/*.{ts,tsx}", {
  query: "?raw",
  import: "default",
  eager: true,
}) as Record<string, string>;

const files = Object.entries(RAW).filter(
  ([path]) => !path.includes("/__tests__/") && !/\.test\.tsx?$/.test(path),
);

describe("no phantom/legacy Tailwind classes", () => {
  it("scans a non-trivial number of source files", () => {
    expect(files.length).toBeGreaterThan(100);
  });

  for (const { label, re } of BANNED) {
    it(`bans ${label}`, () => {
      const hits: string[] = [];
      for (const [path, content] of files) {
        content.split("\n").forEach((line, i) => {
          if (re.test(line)) {
            hits.push(`${path}:${i + 1}  ${line.trim().slice(0, 100)}`);
          }
        });
      }
      expect(hits, `phantom class found — replace it:\n${hits.join("\n")}`).toEqual([]);
    });
  }
});
