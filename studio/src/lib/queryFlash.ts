// consumeQueryParams reads one-shot "flash" query params (e.g. an SSO callback's
// ?sso_error= / ?sso_linked=) and strips them from the URL so a refresh doesn't
// re-trigger the banner. Returns each requested key's value (or null), and does
// the history.replaceState cleanup once — centralising the easy-to-get-wrong
// pathname+search+hash dance that was otherwise copy-pasted across views.
export function consumeQueryParams(
  keys: string[],
): Record<string, string | null> {
  const u = new URL(window.location.href);
  const out: Record<string, string | null> = {};
  let changed = false;
  for (const k of keys) {
    out[k] = u.searchParams.get(k);
    if (u.searchParams.has(k)) {
      u.searchParams.delete(k);
      changed = true;
    }
  }
  if (changed) {
    window.history.replaceState({}, "", u.pathname + u.search + u.hash);
  }
  return out;
}
