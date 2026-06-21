// @vitest-environment jsdom
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render } from "@testing-library/react";
import axe from "axe-core";

// Composed-surface a11y layer — extends the primitives axe smoke
// (primitives.test.tsx) up to whole panels/dialogs, where landmark
// conflicts, heading order and label associations actually surface. We
// only mount surfaces that render under jsdom with light setup (Zustand
// stores have defaults; no network). Canvas (xyflow) + WS-driven views
// stay manual / Playwright.

import AppearanceTab from "@/views/Settings/AppearanceTab";
import AboutTab from "@/views/Settings/AboutTab";
import { RunViewSkeleton, RunViewLoadError } from "@/components/Runs/runView/RunViewLoadStates";

// jsdom lacks matchMedia; the theme store + a few hooks read it on mount.
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

async function expectNoViolations(container: HTMLElement, label: string) {
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

function mount(node: ReactNode): HTMLElement {
  // Wrap in <main> so single-panel surfaces sit in a landmark (axe's
  // region rule otherwise flags top-level content as not contained).
  return render(<main>{node}</main>).container;
}

describe("a11y / composed surfaces", () => {
  afterEach(() => cleanup());

  it("Settings · AppearanceTab — theme + chat-input radio groups", async () => {
    await expectNoViolations(mount(<AppearanceTab />), "AppearanceTab");
  });

  it("Settings · AboutTab — version/info panel", async () => {
    await expectNoViolations(mount(<AboutTab desktopFeatures={false} />), "AboutTab");
  });

  it("RunView · skeleton load state", async () => {
    await expectNoViolations(mount(<RunViewSkeleton />), "RunViewSkeleton");
  });

  it("RunView · error load states (404 + generic)", async () => {
    await expectNoViolations(
      mount(
        <div>
          <RunViewLoadError runId="run-abc" status={404} message="not found" />
          <RunViewLoadError runId="run-def" status={500} message="boom" />
        </div>,
      ),
      "RunViewLoadError",
    );
  });
});
