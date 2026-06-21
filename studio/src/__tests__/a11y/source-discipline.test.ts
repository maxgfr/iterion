import { describe, expect, it } from "vitest";

// Source-discipline regression traps. Each scans every source module as raw
// text (Vite glob, no node:fs — works in vitest's node + jsdom envs) and
// asserts a banned pattern stays at zero. Companion to broken-classes.test.ts
// (phantom tokens) and the design-system.md "Don'ts" table.
//
// These pin invariants the codebase already satisfies, so a regression shows
// up as a failing unit test instead of silent drift:
//   - no window.confirm/alert            -> use the useConfirm() hook
//   - no raw hex colour literals as JS/CSS-in-JS values
//                                        -> a token (lib/constants.ts) or a
//                                           Tailwind utility
//
// Adding a real token to app.css never requires touching this file.

const RAW = import.meta.glob("/src/**/*.{ts,tsx}", {
  query: "?raw",
  import: "default",
  eager: true,
}) as Record<string, string>;

const files = Object.entries(RAW).filter(
  ([path]) => !path.includes("/__tests__/") && !/\.test\.tsx?$/.test(path),
);

/** Collect `path:line  excerpt` hits for `re`, skipping allow-listed paths. */
function scan(re: RegExp, allow: (path: string) => boolean = () => false): string[] {
  const hits: string[] = [];
  for (const [path, src] of files) {
    if (allow(path)) continue;
    src.split("\n").forEach((line, i) => {
      if (re.test(line)) hits.push(`${path}:${i + 1}  ${line.trim().slice(0, 100)}`);
    });
  }
  return hits;
}

/** Whole-file variant for patterns that span lines (e.g. a multiline JSX tag). */
function scanWhole(re: RegExp, allow: (path: string) => boolean = () => false): string[] {
  const hits: string[] = [];
  for (const [path, src] of files) {
    if (allow(path)) continue;
    if (re.test(src)) hits.push(path);
  }
  return hits;
}

describe("source discipline", () => {
  it("scans a non-trivial number of source files", () => {
    // Guards against a glob-scope regression silently emptying the scan.
    expect(files.length).toBeGreaterThan(150);
  });

  it("never calls window.confirm() / window.alert() — use the useConfirm() hook", () => {
    const hits = scan(/\bwindow\.(confirm|alert)\s*\(/);
    if (hits.length) {
      throw new Error(
        `window.confirm/alert is banned (design-system.md § Don'ts) — use useConfirm():\n${hits.join("\n")}`,
      );
    }
    expect(hits).toHaveLength(0);
  });

  it("has no raw hex colour literals as values — use a token or Tailwind utility", () => {
    // A hex string used as an object / style value: `color: "#3b82f6"`,
    // `backgroundColor: '#fff'`. The `:` anchor excludes anchor hrefs
    // (`href="#main"`, which use `=`) and encoded data-URIs (`%23…`). The
    // canonical mirror lib/constants.ts is the one allowed home for raw
    // colour values (and it uses var() strings, not hex, today).
    const hits = scan(
      /:\s*["']#[0-9a-fA-F]{3,8}["']/,
      (path) => path.endsWith("/lib/constants.ts"),
    );
    if (hits.length) {
      throw new Error(
        `raw hex colour literal as a value is banned (design-system.md § Don'ts) — add a token to app.css and consume via lib/constants.ts or a Tailwind utility:\n${hits.join("\n")}`,
      );
    }
    expect(hits).toHaveLength(0);
  });

  it("has no legacy ${token}NN soft-bg interpolation — use softColor(token, pct)", () => {
    // `${color}22` only worked when `color` was an inline hex string; with the
    // var(--color-*) token strings it produces invalid CSS (var(--x)22) so the
    // background/border silently renders nothing. softColor() (color-mix) is
    // the correct replacement. The lookahead pins the 2 hex digits to a
    // closing quote/backtick (the template-literal-as-value shape) to avoid
    // matching incidental `${x}ed`-style text. lib/constants.ts documents the
    // legacy pattern in a comment, so it is allow-listed.
    const hits = scan(
      /\$\{[^}]+\}[0-9a-fA-F]{2}(?=["'\x60])/,
      (path) => path.endsWith("/lib/constants.ts"),
    );
    if (hits.length) {
      throw new Error(
        `legacy \${token}NN soft-bg is banned — use softColor(token, pct):\n${hits.join("\n")}`,
      );
    }
    expect(hits).toHaveLength(0);
  });

  it("uses the Checkbox / Radio primitives, not raw <input type=checkbox|radio>", () => {
    // ui/Checkbox + ui/Radio own the only native checkbox/radio inputs (token
    // border, brand-accent fill, free keyboard/SR semantics). `[^>]*` spans
    // newlines so a multiline `<input …>` tag is still caught; anchoring on
    // `<input` means a `type="radio"` *prop* on a component (e.g. OptionRow)
    // is correctly ignored.
    const RE = /<input\b[^>]*\btype\s*=\s*["'](checkbox|radio)["']/;
    const hits = scanWhole(RE, (path) => /\/ui\/(Checkbox|Radio)\.tsx$/.test(path));
    if (hits.length) {
      throw new Error(
        `raw <input type=checkbox|radio> is banned — use <Checkbox>/<Radio>/<RadioGroup>:\n${hits.join("\n")}`,
      );
    }
    expect(hits).toHaveLength(0);
  });

  it("uses the semantic type scale, not text-[Npx] sizes that have a token", () => {
    // 10/11/12/13/14/16px have tokens (text-caption/micro/body/label/title/
    // display). text-[8px]/[9px] are below the scale floor (no token, used in
    // pips/dense badges) and intentionally allowed.
    const hits = scan(/text-\[(10|11|12|13|14|16)px\]/);
    if (hits.length) {
      throw new Error(
        `text-[Npx] with a token equivalent is banned — use text-caption/micro/body/label/title/display:\n${hits.join("\n")}`,
      );
    }
    expect(hits).toHaveLength(0);
  });
});
