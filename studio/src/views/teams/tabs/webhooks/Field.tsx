// Tiny label/input wrapper shared by the webhook dialogs. Kept narrow on
// purpose — full FieldLabel/Input pairings are imported directly where
// the form needs more layout control.
export function Field({
  label,
  children,
  inline,
}: {
  label: string;
  children: React.ReactNode;
  inline?: boolean;
}) {
  return (
    <label className={inline ? "flex items-center gap-2 text-xs" : "block text-xs space-y-1"}>
      <span className="text-fg-muted">{label}</span>
      <div className={inline ? "" : "block"}>{children}</div>
    </label>
  );
}
