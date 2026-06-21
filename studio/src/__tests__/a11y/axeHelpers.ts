// Shared a11y test helpers for the jsdom + axe-core suites (primitives.test +
// views.test). NOT a test file — imported by them.
import axe from "axe-core";
import { expect } from "vitest";

/**
 * jsdom does not implement matchMedia; components that read the viewport
 * (DesktopOnlyNotice) or the theme store need a stub so they don't crash on
 * mount. Call once at module top in a jsdom test file.
 */
export function setupMatchMedia(): void {
  if (typeof window !== "undefined" && !window.matchMedia) {
    Object.defineProperty(window, "matchMedia", {
      writable: true,
      value: (query: string) => ({
        matches: false,
        media: query,
        onchange: null,
        addEventListener: () => {},
        removeEventListener: () => {},
        addListener: () => {},
        removeListener: () => {},
        dispatchEvent: () => false,
      }),
    });
  }
}

/**
 * Run axe-core over `container` against the WCAG 2.1 A + AA rule sets and
 * throw a readable per-violation summary on failure (`label` names the
 * surface under test).
 */
export async function expectNoViolations(
  container: HTMLElement,
  label: string,
): Promise<void> {
  const results = await axe.run(container, {
    runOnly: {
      type: "tag",
      values: ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"],
    },
  });
  if (results.violations.length > 0) {
    const summary = results.violations
      .map(
        (v) =>
          `  - [${v.id}] ${v.help} (${v.nodes.length} node${v.nodes.length > 1 ? "s" : ""})`,
      )
      .join("\n");
    throw new Error(`${label} has axe violations:\n${summary}`);
  }
  expect(results.violations).toHaveLength(0);
}
