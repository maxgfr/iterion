// Tiny wrappers around localStorage for persisted UI state (booleans,
// enums, strings, JSON). Centralized so every toggle/store handles the
// "storage is unavailable" edge case the same way (private mode, quota
// errors, SSR) instead of re-implementing try/catch at each call site.

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

// readJSONFlag parses a JSON-encoded value, returning `fallback` when the
// key is missing, unreadable (private mode / SSR), or holds malformed JSON
// (e.g. a value from an older build). Pairs with writeJSONFlag for the
// object/array UI state scattered across the stores (recents, downloads,
// library, watch lists, layout sizes, per-run filters).
export function readJSONFlag<T>(key: string, fallback: T): T {
  try {
    const raw = window.localStorage.getItem(key);
    if (raw === null) return fallback;
    return JSON.parse(raw) as T;
  } catch {
    return fallback;
  }
}

export function writeJSONFlag(key: string, value: unknown): void {
  try {
    window.localStorage.setItem(key, JSON.stringify(value));
  } catch {
    // storage may be unavailable or the value not serialisable
  }
}

// removeFlag deletes a key, swallowing the unavailable-storage case so
// callers don't each wrap removeItem in their own try/catch.
export function removeFlag(key: string): void {
  try {
    window.localStorage.removeItem(key);
  } catch {
    // storage may be unavailable
  }
}
