import { useEffect, useRef } from "react";
import { useDocumentStore } from "@/store/document";
import * as api from "@/api/client";

export function useAutoValidation() {
  const document = useDocumentStore((s) => s.document);
  const setDiagnostics = useDocumentStore((s) => s.setDiagnostics);
  const timerRef = useRef<ReturnType<typeof setTimeout>>(undefined);
  const abortRef = useRef<AbortController>(undefined);

  useEffect(() => {
    if (!document) return;
    clearTimeout(timerRef.current);
    timerRef.current = setTimeout(async () => {
      // Abort any in-flight validation to prevent stale results
      abortRef.current?.abort();
      const controller = new AbortController();
      abortRef.current = controller;
      try {
        const result = await api.validate(document, controller.signal);
        if (!controller.signal.aborted) {
          setDiagnostics(result.diagnostics, result.warnings, result.issues);
        }
      } catch (err) {
        // AbortError is expected when a newer keystroke supersedes
        // this request; everything else (network failure, 5xx, parse
        // error) means stale diagnostics are now misleading users.
        // Surface it to the console so devs notice but keep the UI
        // quiet — a toast on every transient hiccup during typing
        // would be worse than no signal at all.
        if (controller.signal.aborted) return;
        const name = (err as { name?: string } | null)?.name;
        if (name === "AbortError") return;
        console.warn("[useAutoValidation] validate failed:", err);
      }
    }, 1500);
    return () => clearTimeout(timerRef.current);
  }, [document, setDiagnostics]);
}
