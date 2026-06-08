import { describe, expect, it } from "vitest";

import type { BotEntryWithSchema } from "@/api/bots";

import { botDisplayLabel } from "./botLabel";

const bots = [
  {
    name: "feature-dev",
    display_name: "Featurly",
    rel_path: "bots/feature-dev",
    is_bundle: true,
  },
] as unknown as BotEntryWithSchema[];

describe("botDisplayLabel", () => {
  it("maps a bundle's main.bot to its persona display_name", () => {
    expect(botDisplayLabel("bots/feature-dev/main.bot", bots)).toBe("Featurly");
  });

  it("falls back to the technical id when the catalog isn't loaded", () => {
    expect(botDisplayLabel("bots/feature-dev/main.bot", null)).toBe("feature-dev");
    // …and for the prefix-less form (label opened via newEditorTab).
    expect(botDisplayLabel("feature-dev/main.bot", null)).toBe("feature-dev");
  });

  it("falls back to the technical id when no persona is known", () => {
    expect(botDisplayLabel("bots/unknown-bot/main.bot", bots)).toBe("unknown-bot");
  });

  it("keeps the basename for loose files", () => {
    expect(botDisplayLabel("flows/my-flow.iter", bots)).toBe("my-flow.iter");
    expect(botDisplayLabel("scratch.bot", bots)).toBe("scratch.bot");
  });
});
