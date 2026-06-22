// Extracted from LaunchView.tsx to keep that file focused.
// AttachmentsSection renders the file-upload row block when the
// workflow declares an `attachments:` field set. Presentational only:
// the actual upload orchestration (XHR + state mutation) stays in
// LaunchView so a single owner controls progress + error coalescing.

import type { AttachmentField, UploadLimits } from "@/api/types";
import { formatBytes, totalSize } from "@/lib/attachmentValidation";

import AttachmentFieldInput, {
  type AttachmentValue,
} from "../AttachmentFieldInput";

export interface AttachmentsSectionProps {
  fields: AttachmentField[];
  attachments: Record<string, AttachmentValue | null>;
  limits: UploadLimits | null;
  submitting: boolean;
  onChange: (field: AttachmentField, next: AttachmentValue | null) => void;
}

export default function AttachmentsSection({
  fields,
  attachments,
  limits,
  submitting,
  onChange,
}: AttachmentsSectionProps) {
  if (fields.length === 0) return null;
  return (
    <section className="mb-6">
      <h2 className="text-xs font-medium text-fg-muted mb-2">Attachments</h2>
      {limits && (
        <p className="mb-3 text-caption text-fg-subtle font-mono">
          Max {formatBytes(limits.max_file_size)} per file ·{" "}
          {formatBytes(limits.max_total_size)} total · up to{" "}
          {limits.max_files_per_run} files ·{" "}
          {limits.allowed_mime.slice(0, 4).join(" ")}
          {limits.allowed_mime.length > 4 ? " …" : ""}
        </p>
      )}
      <div className="space-y-4">
        {fields.map((f) => (
          <div
            key={f.name}
            id={`attach-${f.name}`}
            className="grid grid-cols-[160px_1fr] gap-3 items-start"
          >
            <label className="pt-1">
              <div className="text-xs font-medium font-mono">{f.name}</div>
              <div className="text-caption text-fg-subtle">
                {f.type}
                {f.required ? " · required" : ""}
              </div>
            </label>
            <AttachmentFieldInput
              field={f}
              value={attachments[f.name] ?? null}
              onChange={(next) => onChange(f, next)}
              serverLimits={limits}
              disabled={submitting}
            />
          </div>
        ))}
      </div>
      {Object.values(attachments).some((a) => a?.file) && (
        <p className="mt-2 text-caption text-fg-subtle">
          {Object.values(attachments).filter((a) => a?.file).length} file(s),{" "}
          {formatBytes(totalSize(attachments))} total
        </p>
      )}
    </section>
  );
}
