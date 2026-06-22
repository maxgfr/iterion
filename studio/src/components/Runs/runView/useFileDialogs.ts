import { useCallback, useEffect, useState } from "react";

import { type RunFile, type RunFilesMode } from "@/api/runs";

// useFileDialogs owns the two modal-file-dialog handles RunView passes
// to its children:
//   - diffFile / diffMode: the read-only Monaco diff dialog the
//     FilesPanel opens on row-click. `diffMode` mirrors the mode the
//     FilesPanel was in so FileDiffDialog requests the same range from
//     the backend.
//   - editFile: the worktree path open in the editable Monaco tab
//     (FileEditDialog), or null. Driven by the FilesPanel "Edit
//     .gitignore" shortcut and the diff dialog's "Edit" affordance
//     (which closes the read-only diff first).
//
// Reset on runId change so navigating run A → run B doesn't drag a
// stale file selection into the new run.
export interface FileDialogsState {
  diffFile: RunFile | null;
  diffMode: RunFilesMode;
  editFile: string | null;
  handleSelectFile: (file: RunFile, mode: RunFilesMode) => void;
  handleEditFile: (path: string) => void;
  closeDiff: () => void;
  closeEdit: () => void;
}

export function useFileDialogs(runId: string | null): FileDialogsState {
  const [diffFile, setDiffFile] = useState<RunFile | null>(null);
  // Mode the FilesPanel was in when the user clicked the row; forwarded
  // to FileDiffDialog so it requests the same range from the backend.
  const [diffMode, setDiffMode] = useState<RunFilesMode>("");
  const [editFile, setEditFile] = useState<string | null>(null);

  const handleSelectFile = useCallback(
    (file: RunFile, mode: RunFilesMode) => {
      setDiffMode(mode);
      setDiffFile(file);
    },
    [],
  );
  const handleEditFile = useCallback((path: string) => {
    setDiffFile(null);
    setEditFile(path);
  }, []);
  const closeDiff = useCallback(() => setDiffFile(null), []);
  const closeEdit = useCallback(() => setEditFile(null), []);

  useEffect(() => {
    setDiffFile(null);
    setDiffMode("");
    setEditFile(null);
  }, [runId]);

  return {
    diffFile,
    diffMode,
    editFile,
    handleSelectFile,
    handleEditFile,
    closeDiff,
    closeEdit,
  };
}
