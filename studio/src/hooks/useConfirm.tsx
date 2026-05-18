import { useCallback, useRef, useState, type ReactNode } from "react";
import ConfirmDialog from "@/components/shared/ConfirmDialog";

export interface ConfirmOptions {
  title: string;
  message: ReactNode;
  confirmLabel?: string;
  confirmVariant?: "default" | "danger";
}

interface UseConfirmResult {
  confirm: (options: ConfirmOptions) => Promise<boolean>;
  dialog: ReactNode;
}

// Promise-based wrapper around ConfirmDialog so call-sites keep an
// almost-synchronous shape:
//
//   const { confirm, dialog } = useConfirm();
//   const handleX = async () => {
//     if (isDirty() && !(await confirm({ title, message }))) return;
//     ...
//   };
//   return <>...{dialog}</>;
//
// One outstanding dialog per hook instance is enough — callers that
// need multiple concurrent confirms can mount the hook twice.
export function useConfirm(): UseConfirmResult {
  const [opts, setOpts] = useState<ConfirmOptions | null>(null);
  const resolverRef = useRef<((value: boolean) => void) | null>(null);

  const confirm = useCallback((options: ConfirmOptions) => {
    return new Promise<boolean>((resolve) => {
      resolverRef.current = resolve;
      setOpts(options);
    });
  }, []);

  const settle = useCallback((value: boolean) => {
    const resolve = resolverRef.current;
    resolverRef.current = null;
    setOpts(null);
    resolve?.(value);
  }, []);

  const dialog = opts ? (
    <ConfirmDialog
      open
      title={opts.title}
      message={opts.message}
      confirmLabel={opts.confirmLabel}
      confirmVariant={opts.confirmVariant}
      onConfirm={() => settle(true)}
      onCancel={() => settle(false)}
    />
  ) : null;

  return { confirm, dialog };
}
