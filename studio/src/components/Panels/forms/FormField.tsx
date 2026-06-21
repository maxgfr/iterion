import {
  useCallback,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent,
  type ReactNode,
} from "react";
import { Checkbox } from "@/components/ui/Checkbox";
import { FieldLabel } from "@/components/ui/FieldLabel";
import { HelpHint } from "@/components/ui/HelpHint";
import { RefAwareInput, RefAwareTextarea } from "@/components/ui/RefAwareInput";
import PromptOverlayHighlight from "@/components/ui/PromptOverlayHighlight";
import { Select } from "@/components/ui/Select";
import type { RefContext } from "@/lib/refCompletion";
import { Pencil1Icon } from "@radix-ui/react-icons";

const labelClass = "block text-xs text-fg-subtle mb-1";
const inputClass = "w-full bg-surface-1 border border-border-strong rounded px-2 py-1 text-sm text-fg-default focus:border-accent focus:outline-none disabled:opacity-60 disabled:cursor-not-allowed";

interface FieldRowChildArgs {
  /** id to apply to the primary control inside the row. */
  inputId: string;
  /** Space-separated ids to feed `aria-describedby` (help + error). */
  describedBy: string | undefined;
}

interface FieldRowProps {
  label: string;
  help?: string;
  error?: string;
  className?: string;
  children: ReactNode | ((args: FieldRowChildArgs) => ReactNode);
}

/**
 * Standard label + control + error layout used by every text-like
 * field. Owns the id generation + aria-describedby plumbing so screen
 * readers correctly announce the help icon and the error message
 * alongside the input. Fields that take an `error` prop only need to
 * pipe `inputId` and `describedBy` through; the FieldRow renders the
 * <p role="alert"> automatically.
 */
function FieldRow({
  label,
  help,
  error,
  className = "mb-2",
  children,
}: FieldRowProps) {
  const baseId = useId();
  const inputId = `${baseId}-input`;
  const helpId = help ? `${baseId}-help` : undefined;
  const errorId = error ? `${baseId}-err` : undefined;
  const describedBy = [helpId, errorId].filter(Boolean).join(" ") || undefined;
  return (
    <div className={className}>
      <FieldLabel htmlFor={inputId} help={help} helpId={helpId}>{label}</FieldLabel>
      {typeof children === "function"
        ? children({ inputId, describedBy })
        : children}
      {error && (
        <p
          id={errorId}
          role="alert"
          className="text-caption text-danger mt-0.5"
        >
          {error}
        </p>
      )}
    </div>
  );
}

interface TextFieldProps {
  label: string;
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
  multiline?: boolean;
  rows?: number;
  help?: string;
  error?: string;
  /** When provided, enables {{...}} reference autocomplete for this field. */
  refContext?: RefContext;
}

export function TextField({ label, value, onChange, placeholder, multiline, rows = 3, help, error, refContext }: TextFieldProps) {
  return (
    <FieldRow label={label} help={help} error={error}>
      {({ inputId, describedBy }) =>
        multiline ? (
          refContext ? (
            <RefAwareTextarea
              value={value}
              onChange={onChange}
              placeholder={placeholder}
              rows={rows}
              refContext={refContext}
            />
          ) : (
            <textarea
              id={inputId}
              aria-describedby={describedBy}
              className={inputClass + " resize-y"}
              value={value}
              onChange={(e) => onChange(e.target.value)}
              placeholder={placeholder}
              rows={rows}
            />
          )
        ) : refContext ? (
          <RefAwareInput
            value={value}
            onChange={onChange}
            placeholder={placeholder}
            refContext={refContext}
          />
        ) : (
          <input
            id={inputId}
            aria-describedby={describedBy}
            className={inputClass}
            type="text"
            value={value}
            onChange={(e) => onChange(e.target.value)}
            placeholder={placeholder}
          />
        )
      }
    </FieldRow>
  );
}

interface CommittedTextFieldProps {
  label: string;
  value: string;
  onChange: (v: string) => void;
  onCommit?: (newValue: string) => void;
  validate?: (v: string) => string | null;
  placeholder?: string;
  help?: string;
}

/** TextField that only commits on blur or Enter, not on every keystroke. Used for name/rename fields. */
export function CommittedTextField({ label, value, onChange, onCommit, validate, placeholder, help }: CommittedTextFieldProps) {
  const [draft, setDraft] = useState(value);
  const [error, setError] = useState<string | null>(null);
  const focusedRef = useRef(false);

  // Sync draft from prop when not focused
  useEffect(() => {
    if (!focusedRef.current) {
      setDraft(value);
      setError(null);
    }
  }, [value]);

  const commit = useCallback(() => {
    const trimmed = draft.trim();
    if (trimmed === value) {
      setError(null);
      return;
    }
    if (validate) {
      const err = validate(trimmed);
      if (err) {
        setError(err);
        setDraft(value);
        return;
      }
    }
    setError(null);
    onChange(trimmed);
    onCommit?.(trimmed);
  }, [draft, value, validate, onChange, onCommit]);

  const handleBlur = useCallback(() => {
    focusedRef.current = false;
    commit();
  }, [commit]);

  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => {
      if (e.key === "Enter") {
        e.preventDefault();
        (e.target as HTMLInputElement).blur();
      } else if (e.key === "Escape") {
        setDraft(value);
        setError(null);
        (e.target as HTMLInputElement).blur();
      }
    },
    [value],
  );

  const isDirty = draft.trim() !== value;

  return (
    <FieldRow label={label} help={help} error={error ?? undefined}>
      {({ inputId, describedBy }) => (
        <div className="flex gap-1">
          <input
            id={inputId}
            aria-describedby={describedBy}
            aria-invalid={error ? true : undefined}
            className={`${inputClass} flex-1${error ? " ring-1 ring-danger border-danger" : ""}`}
            type="text"
            value={draft}
            onChange={(e) => { setDraft(e.target.value); setError(null); }}
            onFocus={() => { focusedRef.current = true; }}
            onBlur={handleBlur}
            onKeyDown={handleKeyDown}
            placeholder={placeholder}
          />
          {isDirty && (
            <button
              type="button"
              className="bg-accent hover:bg-accent-hover text-fg-onAccent text-xs px-1.5 rounded shrink-0"
              onMouseDown={(e) => {
                e.preventDefault(); // prevent blur before commit
                commit();
                (document.activeElement as HTMLInputElement)?.blur();
              }}
              title="Confirm"
              aria-label="Confirm edit"
            >
              &#x2713;
            </button>
          )}
        </div>
      )}
    </FieldRow>
  );
}

interface NumberFieldProps {
  label: string;
  value: number | undefined;
  onChange: (v: number | undefined) => void;
  placeholder?: string;
  min?: number;
  help?: string;
  error?: string;
}

export function NumberField({ label, value, onChange, placeholder, min, help, error }: NumberFieldProps) {
  return (
    <FieldRow label={label} help={help} error={error}>
      {({ inputId, describedBy }) => (
        <input
          id={inputId}
          aria-describedby={describedBy}
          aria-invalid={error ? true : undefined}
          className={inputClass}
          type="number"
          value={value ?? ""}
          onChange={(e) => onChange(e.target.value === "" ? undefined : Number(e.target.value))}
          placeholder={placeholder}
          min={min}
        />
      )}
    </FieldRow>
  );
}

interface SelectFieldProps {
  label: string;
  value: string;
  onChange: (v: string) => void;
  options: { value: string; label: string }[];
  allowEmpty?: boolean;
  emptyLabel?: string;
  help?: string;
  error?: string;
}

export function SelectField({ label, value, onChange, options, allowEmpty, emptyLabel = "-- none --", help, error }: SelectFieldProps) {
  return (
    <FieldRow label={label} help={help} error={error}>
      {({ inputId, describedBy }) => (
        <Select
          id={inputId}
          aria-describedby={describedBy}
          aria-invalid={error ? true : undefined}
          size="sm"
          error={!!error}
          value={value}
          onChange={(e) => onChange(e.target.value)}
        >
          {allowEmpty && <option value="">{emptyLabel}</option>}
          {options.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </Select>
      )}
    </FieldRow>
  );
}

interface SelectFieldWithCreateProps extends SelectFieldProps {
  onCreate: () => string; // returns the new name
}

export function SelectFieldWithCreate({ label, value, onChange, options, allowEmpty, emptyLabel, onCreate, help, error }: SelectFieldWithCreateProps) {
  return (
    <FieldRow label={label} help={help} error={error}>
      {({ inputId, describedBy }) => (
        <div className="flex gap-1">
          <Select
            id={inputId}
            aria-describedby={describedBy}
            aria-invalid={error ? true : undefined}
            size="sm"
            error={!!error}
            className="flex-1"
            value={value}
            onChange={(e) => onChange(e.target.value)}
          >
            {allowEmpty && <option value="">{emptyLabel ?? "-- none --"}</option>}
            {options.map((o) => (
              <option key={o.value} value={o.value}>
                {o.label}
              </option>
            ))}
          </Select>
          <button
            type="button"
            className="bg-success hover:bg-success/90 text-xs px-1.5 rounded shrink-0"
            onClick={() => {
              const newName = onCreate();
              onChange(newName);
            }}
            title={`Create new ${label.toLowerCase()}`}
            aria-label={`Create new ${label.toLowerCase()}`}
          >
            +
          </button>
        </div>
      )}
    </FieldRow>
  );
}

interface CheckboxFieldProps {
  label: string;
  checked: boolean;
  onChange: (v: boolean) => void;
  help?: string;
}

export function CheckboxField({ label, checked, onChange, help }: CheckboxFieldProps) {
  const id = useId();
  return (
    <div className="mb-2 flex items-center gap-2">
      <Checkbox
        id={id}
        checked={checked}
        onChange={(e) => onChange(e.target.checked)}
      />
      <label htmlFor={id} className="text-xs text-fg-subtle">
        {label}
        {help && <HelpHint text={help} />}
      </label>
    </div>
  );
}

interface TagListFieldProps {
  label: string;
  values: string[];
  onChange: (v: string[]) => void;
  placeholder?: string;
}

export function TagListField({ label, values, onChange, placeholder = "Add..." }: TagListFieldProps) {
  const [input, setInput] = useState("");

  const addTag = useCallback(() => {
    const v = input.trim();
    if (v && !values.includes(v)) {
      onChange([...values, v]);
    }
    setInput("");
  }, [input, values, onChange]);

  const onKeyDown = useCallback(
    (e: KeyboardEvent) => {
      if (e.key === "Enter") {
        e.preventDefault();
        addTag();
      }
    },
    [addTag],
  );

  return (
    <div className="mb-2">
      <label className={labelClass}>{label}</label>
      <div className="flex flex-wrap gap-1 mb-1">
        {values.map((v) => (
          <span key={v} className="bg-surface-2 text-xs px-2 py-0.5 rounded flex items-center gap-1">
            {v}
            <button
              type="button"
              aria-label={`Remove ${v}`}
              className="text-fg-subtle hover:text-fg-default"
              onClick={() => onChange(values.filter((x) => x !== v))}
            >
              x
            </button>
          </span>
        ))}
      </div>
      <div className="flex gap-1">
        <input
          className={inputClass}
          type="text"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={onKeyDown}
          placeholder={placeholder}
        />
        <button
          type="button"
          aria-label="Add"
          className="bg-surface-2 hover:bg-surface-3 text-xs px-2 rounded"
          onClick={addTag}
        >
          +
        </button>
      </div>
    </div>
  );
}

interface PromptPickerFieldProps {
  label: string;
  /** Current selected prompt name (or "" for none). */
  value: string;
  onChange: (v: string) => void;
  options: { value: string; label: string }[];
  /** Returns the new prompt name when the user clicks the create button. */
  onCreate: () => string;
  /** Invoked when the user clicks the pencil to edit the selected prompt. */
  onEdit: (promptName: string) => void;
  /** Body of the currently-selected prompt — used for the inline preview. */
  body: string;
  allowEmpty?: boolean;
  emptyLabel?: string;
  help?: string;
  error?: string;
}

/**
 * Prompt-first picker: like `SelectFieldWithCreate` but adds a pencil
 * button that opens the selected prompt in the prompt editor modal,
 * plus a collapsed monospace preview of the body so authors can scan
 * the prompt without leaving the node form. Used by the agent /
 * judge / human / router forms for any prompt slot.
 */
export function PromptPickerField({
  label,
  value,
  onChange,
  options,
  onCreate,
  onEdit,
  body,
  allowEmpty,
  emptyLabel = "-- select prompt --",
  help,
  error,
}: PromptPickerFieldProps) {
  const previewLines = useMemo(() => {
    if (!body) return "";
    const lines = body.split("\n").slice(0, 3);
    const truncated = lines.join("\n");
    return body.split("\n").length > 3 ? truncated + "\n…" : truncated;
  }, [body]);

  return (
    <FieldRow label={label} help={help} error={error}>
      {({ inputId, describedBy }) => (
        <>
          <div className="flex gap-1">
            <Select
              id={inputId}
              aria-describedby={describedBy}
              aria-invalid={error ? true : undefined}
              size="sm"
              error={!!error}
              className="flex-1"
              value={value}
              onChange={(e) => onChange(e.target.value)}
            >
              {allowEmpty && <option value="">{emptyLabel}</option>}
              {options.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </Select>
            <button
              type="button"
              className="bg-surface-2 hover:bg-surface-3 text-xs px-1.5 rounded shrink-0 inline-flex items-center justify-center disabled:opacity-40 disabled:cursor-not-allowed"
              onClick={() => value && onEdit(value)}
              disabled={!value}
              title={value ? `Edit prompt "${value}"` : "Select a prompt to edit"}
              aria-label={value ? `Edit prompt ${value}` : "Edit prompt"}
            >
              <Pencil1Icon />
            </button>
            <button
              type="button"
              className="bg-success hover:bg-success/90 text-xs px-1.5 rounded shrink-0"
              onClick={() => {
                const newName = onCreate();
                onChange(newName);
                onEdit(newName);
              }}
              title={`Create new ${label.toLowerCase()}`}
              aria-label={`Create new ${label.toLowerCase()}`}
            >
              +
            </button>
          </div>
          {value && body && (
            <button
              type="button"
              className="mt-1 w-full text-left rounded border border-border-default bg-surface-0 hover:border-accent transition-colors"
              onClick={() => onEdit(value)}
              title="Click to edit in large editor"
            >
              <PromptOverlayHighlight
                value={previewLines}
                inline
                className="px-2 py-1 text-micro font-mono text-fg-muted leading-snug"
                maxHeight="4.5em"
              />
            </button>
          )}
          {value && !body && (
            <p className="mt-1 text-caption text-fg-subtle italic">Empty prompt body — click the pencil to write it.</p>
          )}
        </>
      )}
    </FieldRow>
  );
}

interface MultiSelectFieldProps {
  label: string;
  values: string[];
  onChange: (v: string[]) => void;
  options: string[];
}

export function MultiSelectField({ label, values, onChange, options }: MultiSelectFieldProps) {
  return (
    <div className="mb-2">
      <label className={labelClass}>{label}</label>
      <div className="flex flex-col gap-1 max-h-32 overflow-y-auto bg-surface-1 border border-border-strong rounded p-1">
        {options.map((opt) => (
          <label key={opt} className="flex items-center gap-2 text-xs text-fg-muted px-1 hover:bg-surface-2 rounded cursor-pointer">
            <Checkbox
              checked={values.includes(opt)}
              onChange={(e) => {
                if (e.target.checked) {
                  onChange([...values, opt]);
                } else {
                  onChange(values.filter((v) => v !== opt));
                }
              }}
            />
            {opt}
          </label>
        ))}
        {options.length === 0 && <span className="text-xs text-fg-subtle px-1">No options available</span>}
      </div>
    </div>
  );
}
