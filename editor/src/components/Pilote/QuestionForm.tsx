import { useMemo, useState } from "react";

import { Button } from "@/components/ui";
import type { FormAnswer, FormSpec } from "@/lib/pilote/questionForm";
import { OTHER_SENTINEL } from "@/lib/pilote/questionForm";

import QuestionInput from "./QuestionInput";

interface Props {
  spec: FormSpec;
  // Called with the merged answers map when the user submits a
  // satisfying form. Question.id → value (string | string[]).
  onSubmit: (answers: FormAnswer) => void;
  busy?: boolean;
}

// QuestionForm renders the FormSpec and emits a FormAnswer on submit.
//
// Ergonomy (matches "Auto" choice in the design pass):
// - 1 question  → no extra heading; the question label sits directly
//   above the input, with an inline Send button.
// - 2+ questions → titled form-style layout; one Submit at the bottom.
//
// The form keeps internal state for each question. Submission is
// gated on validation: required free_text must be non-blank, required
// radio/select must have a chosen value, required checkbox must have
// at least one selection.

export default function QuestionForm({ spec, onSubmit, busy = false }: Props) {
  const initial: FormAnswer = useMemo(() => {
    const out: FormAnswer = {};
    for (const q of spec.questions) {
      out[q.id] = q.kind === "checkbox" ? [] : "";
    }
    return out;
  }, [spec]);

  const [answers, setAnswers] = useState<FormAnswer>(initial);

  const setOne = (id: string, value: string | string[]) => {
    setAnswers((prev) => ({ ...prev, [id]: value }));
  };

  const valid = isValid(spec, answers);
  const single = spec.questions.length === 1;
  const submitLabel = spec.submitLabel ?? "Send";

  const submit = () => {
    if (!valid || busy) return;
    onSubmit(answers);
  };

  return (
    <div
      className={
        single
          ? "flex items-stretch gap-2"
          : "rounded-lg border border-border-subtle bg-surface-1 p-3 space-y-3"
      }
    >
      <div className={single ? "flex-1" : "space-y-3"}>
        {spec.questions.map((q, i) => (
          <div key={q.id} className={single ? "" : "space-y-1"}>
            {!single && (
              <div className="space-y-0.5">
                <label className="text-[12px] font-medium text-fg-default">
                  {q.label}
                  {q.required !== false && (
                    <span className="ml-1 text-danger-fg" aria-hidden="true">
                      *
                    </span>
                  )}
                </label>
                {q.description && (
                  <p className="text-[11px] text-fg-muted">{q.description}</p>
                )}
              </div>
            )}
            <QuestionInput
              question={q}
              value={answers[q.id]}
              onChange={(v) => setOne(q.id, v)}
              disabled={busy}
            />
            {single && i === 0 && q.description && (
              <p className="mt-1 text-[11px] text-fg-muted">{q.description}</p>
            )}
          </div>
        ))}
      </div>

      <div className={single ? "self-end" : "flex justify-end pt-1"}>
        <Button
          variant="primary"
          size="sm"
          disabled={!valid || busy}
          onClick={submit}
        >
          {busy ? "…" : submitLabel}
        </Button>
      </div>
    </div>
  );
}

// isValid: every required question carries a non-empty, non-sentinel
// answer. OTHER_SENTINEL is what QuestionInput stores when the user
// has selected Other but not yet typed any text — submit must stay
// gated until they do.
function isValid(spec: FormSpec, answers: FormAnswer): boolean {
  for (const q of spec.questions) {
    const isRequired = isRequiredQuestion(q);
    if (!isRequired) continue;
    const v = answers[q.id];
    if (q.kind === "checkbox") {
      if (!Array.isArray(v) || v.length === 0) return false;
      // A checkbox with ONLY the Other sentinel and no other
      // selections is also incomplete.
      const meaningful = v.filter((x) => x !== OTHER_SENTINEL && x.trim() !== "");
      if (meaningful.length === 0) return false;
    } else if (typeof v !== "string" || v.trim() === "" || v === OTHER_SENTINEL) {
      return false;
    }
  }
  return true;
}

function isRequiredQuestion(q: FormSpec["questions"][number]): boolean {
  if (q.required !== undefined) return q.required;
  // Defaults: radio + free_text required; checkbox + select optional.
  return q.kind === "radio" || q.kind === "free_text";
}
