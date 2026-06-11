import { describe, expect, it } from "vitest";
import { isValidSecretName } from "@/api/secrets";

describe("isValidSecretName", () => {
  it.each([
    ["GITLAB_TOKEN", true],
    ["_internal", true],
    ["a", true],
    ["a1", true],
    ["FOO_BAR_BAZ", true],
  ])("accepts %s", (name, ok) => {
    expect(isValidSecretName(name).ok).toBe(ok);
  });

  it.each([
    ["", "required"],
    ["1foo", "digit"],
    ["foo bar", "underscore"],
    ["foo-bar", "underscore"],
    ["foo.bar", "underscore"],
    ["foo:bar", "underscore"],
  ])("rejects %s", (name, hint) => {
    const r = isValidSecretName(name);
    expect(r.ok).toBe(false);
    expect((r.error ?? "").toLowerCase()).toContain(hint);
  });

  it("rejects names exceeding 128 characters", () => {
    const r = isValidSecretName("A".repeat(129));
    expect(r.ok).toBe(false);
    expect(r.error).toMatch(/128/);
  });
});
