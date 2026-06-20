import { create } from "zustand";

import { readEnumFlag, writeStringFlag } from "@/lib/localStorageFlag";

export type ThemeMode = "system" | "light" | "dark";
export type ResolvedTheme = "light" | "dark";

const STORAGE_KEY = "iterion.theme";
const VALID_MODES: ThemeMode[] = ["system", "light", "dark"];

function readStoredMode(): ThemeMode {
  return readEnumFlag(STORAGE_KEY, VALID_MODES, "system");
}

function systemPrefersDark(): boolean {
  if (typeof window === "undefined") return true;
  try {
    return window.matchMedia?.("(prefers-color-scheme: dark)").matches ?? true;
  } catch {
    return true;
  }
}

function resolveMode(mode: ThemeMode): ResolvedTheme {
  if (mode === "system") return systemPrefersDark() ? "dark" : "light";
  return mode;
}

function applyTheme(resolved: ResolvedTheme) {
  if (typeof document === "undefined") return;
  document.documentElement.setAttribute("data-theme", resolved);
}

interface ThemeState {
  mode: ThemeMode;
  resolved: ResolvedTheme;
  setMode: (mode: ThemeMode) => void;
  cycleMode: () => void;
}

export const useThemeStore = create<ThemeState>((set, get) => ({
  mode: "system",
  resolved: "dark",
  setMode: (mode) => {
    writeStringFlag(STORAGE_KEY, mode);
    const resolved = resolveMode(mode);
    applyTheme(resolved);
    set({ mode, resolved });
  },
  cycleMode: () => {
    const order: ThemeMode[] = ["system", "light", "dark"];
    const idx = order.indexOf(get().mode);
    const next = order[(idx + 1) % order.length] ?? "system";
    get().setMode(next);
  },
}));

/**
 * Initialize theme synchronously, before React renders, to avoid a flash of
 * the wrong theme. Wires a media-query listener so "system" mode reacts to
 * OS-level theme changes while the app is open.
 */
export function initializeTheme() {
  const mode = readStoredMode();
  const resolved = resolveMode(mode);
  applyTheme(resolved);
  useThemeStore.setState({ mode, resolved });

  if (typeof window !== "undefined") {
    if (!window.matchMedia) return;
    const mq = window.matchMedia("(prefers-color-scheme: dark)");
    const handler = () => {
      const current = useThemeStore.getState().mode;
      if (current !== "system") return;
      const nextResolved: ResolvedTheme = mq.matches ? "dark" : "light";
      applyTheme(nextResolved);
      useThemeStore.setState({ resolved: nextResolved });
    };
    if (mq.addEventListener) mq.addEventListener("change", handler);
    else mq.addListener(handler);
  }
}
