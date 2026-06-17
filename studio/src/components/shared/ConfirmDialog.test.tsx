// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";

import ConfirmDialog from "./ConfirmDialog";

afterEach(cleanup);

function setup(extra?: Record<string, unknown>) {
  const onConfirm = vi.fn();
  const onCancel = vi.fn();
  render(
    <ConfirmDialog
      open
      title="Delete node?"
      message="This removes the node from the graph."
      confirmLabel="Delete"
      confirmVariant="danger"
      onConfirm={onConfirm}
      onCancel={onCancel}
      {...extra}
    />,
  );
  return {
    onConfirm,
    onCancel,
    cancel: screen.getByRole("button", { name: "Cancel" }),
    confirm: screen.getByRole("button", { name: "Delete" }),
  };
}

describe("ConfirmDialog", () => {
  it("moves focus to Cancel (least-destructive) on open", () => {
    const { cancel } = setup();
    expect(document.activeElement).toBe(cancel);
  });

  it("Escape calls onCancel", () => {
    const { onCancel } = setup();
    fireEvent.keyDown(window, { key: "Escape" });
    expect(onCancel).toHaveBeenCalledTimes(1);
  });

  it("exposes a labelled modal dialog", () => {
    setup();
    const dialog = screen.getByRole("dialog");
    expect(dialog.getAttribute("aria-modal")).toBe("true");
    expect(dialog.getAttribute("aria-label")).toBe("Delete node?");
  });

  it("traps Tab inside the dialog (last → first)", () => {
    const { cancel, confirm } = setup();
    confirm.focus();
    expect(document.activeElement).toBe(confirm);
    fireEvent.keyDown(window, { key: "Tab" });
    expect(document.activeElement).toBe(cancel);
  });

  it("traps Shift+Tab inside the dialog (first → last)", () => {
    const { cancel, confirm } = setup();
    cancel.focus();
    fireEvent.keyDown(window, { key: "Tab", shiftKey: true });
    expect(document.activeElement).toBe(confirm);
  });

  it("pulls stray focus back into the dialog on Tab", () => {
    const { cancel } = setup();
    // Simulate focus having escaped to the background (e.g. via a click).
    document.body.focus();
    fireEvent.keyDown(window, { key: "Tab" });
    expect(document.activeElement).toBe(cancel);
  });
});
