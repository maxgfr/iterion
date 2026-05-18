// Tiny wrappers around localStorage for boolean UI flags. Centralized so
// every collapse/expand toggle in the app handles the "storage is
// unavailable" edge case the same way (private mode, quota errors, SSR).

export function readBooleanFlag(key: string, fallback = false): boolean {
  try {
    const raw = window.localStorage.getItem(key);
    if (raw === null) return fallback;
    return raw === "1";
  } catch {
    return fallback;
  }
}

export function writeBooleanFlag(key: string, value: boolean): void {
  try {
    window.localStorage.setItem(key, value ? "1" : "0");
  } catch {
    // storage may be unavailable
  }
}

// readEnumFlag reads a string flag and validates it against a set of
// allowed values. Returns `fallback` when the stored value is missing,
// unreadable, or not in the allowlist. Cheap defence so the run-console
// can't get stuck on a tab id from an old build.
export function readEnumFlag<T extends string>(
  key: string,
  allowed: readonly T[],
  fallback: T,
): T {
  try {
    const raw = window.localStorage.getItem(key);
    if (!raw) return fallback;
    return (allowed as readonly string[]).includes(raw) ? (raw as T) : fallback;
  } catch {
    return fallback;
  }
}

export function writeStringFlag(key: string, value: string): void {
  try {
    window.localStorage.setItem(key, value);
  } catch {
    // storage may be unavailable
  }
}
