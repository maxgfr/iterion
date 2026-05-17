import { describe, it, expect } from "vitest";

import type { FormQuestion } from "./questionForm";
import { OTHER_SENTINEL } from "./questionForm";

// The QuestionInput component owns the radio/checkbox/Other state
// machine but pure-function tests can still cover the shape contracts
// here. Component-render tests are out of scope per the editor's
// testing convention.

describe("questionForm types", () => {
  it("OTHER_SENTINEL is a non-empty distinctive string", () => {
    expect(OTHER_SENTINEL).toBe("__other__");
    // Just a regression: someone renaming the sentinel must update
    // any saved form-answer migrations downstream too.
    expect(typeof OTHER_SENTINEL).toBe("string");
    expect(OTHER_SENTINEL.length).toBeGreaterThan(2);
  });

  it("FormQuestion discriminates on kind", () => {
    const radio: FormQuestion = {
      kind: "radio",
      id: "x",
      label: "?",
      options: [{ value: "a", label: "A" }],
      allow_other: true,
    };
    const checkbox: FormQuestion = {
      kind: "checkbox",
      id: "x",
      label: "?",
      options: [],
    };
    const text: FormQuestion = {
      kind: "free_text",
      id: "x",
      label: "?",
      rows: 5,
    };
    const select: FormQuestion = {
      kind: "select",
      id: "x",
      label: "?",
      options: [],
      placeholder: "pick",
    };
    expect(radio.kind).toBe("radio");
    expect(checkbox.kind).toBe("checkbox");
    expect(text.kind).toBe("free_text");
    expect(select.kind).toBe("select");
  });
});
