import { useId, type ReactNode } from "react";
import { Radio } from "./Radio";

export interface RadioGroupOption {
  value: string;
  label: ReactNode;
  disabled?: boolean;
}

export interface RadioGroupProps {
  /** Shared `name` for the underlying radios (required for native grouping). */
  name: string;
  value: string;
  onChange: (value: string) => void;
  options: RadioGroupOption[];
  /** Optional group label, wired via `aria-labelledby` onto the radiogroup. */
  label?: string;
  orientation?: "horizontal" | "vertical";
  className?: string;
}

/**
 * Thin controlled wrapper around {@link Radio}: renders a labelled
 * `role="radiogroup"` set. Native radios provide arrow-key roving and the
 * single-tab-stop behaviour automatically; this only owns the value plumbing
 * and the group label association.
 */
export function RadioGroup({
  name,
  value,
  onChange,
  options,
  label,
  orientation = "vertical",
  className = "",
}: RadioGroupProps) {
  const labelId = useId();
  return (
    <div
      role="radiogroup"
      aria-labelledby={label ? labelId : undefined}
      className={className}
    >
      {label && (
        <span id={labelId} className="block text-xs text-fg-subtle mb-1">
          {label}
        </span>
      )}
      <div className={orientation === "horizontal" ? "flex flex-wrap gap-4" : "flex flex-col gap-1"}>
        {options.map((o) => (
          <Radio
            key={o.value}
            name={name}
            value={o.value}
            checked={value === o.value}
            disabled={o.disabled}
            onChange={() => onChange(o.value)}
            label={o.label}
          />
        ))}
      </div>
    </div>
  );
}
