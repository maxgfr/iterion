# protestware-node-ipc (iterion fixture)

Minimal fixture targeting the secured-renovacy bot's **anti-malware
heuristic**. The repo declares exactly one outdated dependency:
`node-ipc@10.1.1` — the version that shipped the `peacenotwar` /
protestware sabotage payload (March 2022, GHSA-97m3-w2cp-4xx6).

**Do not introduce this pin in a production environment.** It exists
solely so the bot's `security_audit` node, running `osv-scanner
--lockfile=package-lock.json` (and any secondary auditor available in
the sandbox), surfaces the documented malware advisory.

## What's in the fixture

| File                | Purpose                                              |
|---------------------|------------------------------------------------------|
| `package.json`      | Declares `node-ipc: "10.1.1"` as the sole dependency |
| `package-lock.json` | Hand-crafted (registry has un-published 10.1.1 after the incident, so `npm install` cannot regenerate). Lists the exact pin so `osv-scanner --lockfile=` can flag it |
| `index.js`          | Noop entry — the fixture is for scanning, not running |

## Expected bot behaviour

When `TestLive_SecuredRenovacy_Protestware` runs the secured-renovacy
bot against this fixture:

1. `detect_stack` identifies a single-ecosystem npm project.
2. `discover_outdated` lists `node-ipc 10.1.1 → 12+` as the sole
   outdated package, risk: major.
3. `bucket_patches` produces an empty patches list (no patches —
   single major).
4. The per-package loop selects `node-ipc` for `changelog_review` +
   `security_audit`.
5. `security_audit` runs the ecosystem's auditor +
   `osv-scanner --lockfile=package-lock.json`. The OSV advisory for
   the protestware release (GHSA-97m3-w2cp-4xx6 / SNYK / etc.) MUST
   surface.
6. The audit output should have `safe: false` AND/OR
   `malware_signals: [...]` referencing peacenotwar / protestware /
   10.1.1, AND/OR `cves: [...]` containing the advisory ID.

The test asserts at least one of those three signals appears for
node-ipc — that's the anti-malware heuristic working as designed.
