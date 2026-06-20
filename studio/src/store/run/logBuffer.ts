// MAX_LOG_BYTES caps the in-memory log tail so a verbose run doesn't
// bloat the React heap. Older bytes fall off the front; the start
// offset advances accordingly. Matches the backend ring of 1 MiB so
// the WS replay window stays consistent.
export const MAX_LOG_BYTES = 1 << 20;
// Truncate down to LOG_TRIM_TARGET (75% of cap) instead of the cap
// itself so we don't pay an O(N) slice on every appended chunk once
// the cap is reached — amortises the copy to one trim per ~256 KiB.
export const LOG_TRIM_TARGET = (MAX_LOG_BYTES * 3) >> 2;

// utf8Len returns the number of UTF-8 *bytes* a string encodes to, NOT
// its UTF-16 code-unit length (`String.prototype.length`). Used to keep
// the log byte cursor (RunLogState.nextByte) aligned with the backend,
// which tracks every log offset in bytes. Allocation-free so the
// one-shot ~1 MiB snapshot on tab open doesn't churn a Uint8Array.
export function utf8Len(s: string): number {
  let bytes = 0;
  for (let i = 0; i < s.length; i++) {
    const c = s.charCodeAt(i);
    if (c < 0x80) bytes += 1;
    else if (c < 0x800) bytes += 2;
    else if (c >= 0xd800 && c <= 0xdbff) {
      // High surrogate → a code point ≥ U+10000 (4 bytes); consume the
      // paired low surrogate so it isn't counted again.
      bytes += 4;
      i++;
    } else bytes += 3;
  }
  return bytes;
}
