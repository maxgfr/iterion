# renovacy-multi-stack (iterion fixture)

A polyglot fixture targeting `iterion`'s `secured-renovacy` bot live
test. Each subdirectory pins **deliberately vulnerable / outdated**
versions of real packages with documented CVEs so the bot's
`security_audit`, `changelog_review`, and `align_code` nodes have
something concrete to chew on.

**Do not introduce these into a production environment.** The pins
are chosen to surface known issues — upgrading them is the bot's
job, not something the maintainer should do here.

## Stacks present

| Dir | Manifest | Vulnerable pins (representative) |
|------|---------|-----------------------------------|
| `npm-app/` | `package.json`, `package-lock.json` | `axios@0.21.1` (CVE-2021-3749), `lodash@4.17.4` (CVE-2018-3721), `node-ipc@10.1.1` (protestware/sabotage) |
| `py-app/` | `requirements.txt` | `urllib3@1.26.4` (CVE-2021-33503), `pyyaml@5.1` (CVE-2020-1747), `requests@2.25.0` |
| `go-app/` | `go.mod` | `github.com/gin-gonic/gin@v1.6.0` (CVE-2023-26125) |
| `rust-app/` | `Cargo.toml` | `tokio@1.7.1` (RUSTSEC-2021-0072) |

## Anti-malware heuristic test

`node-ipc@10.1.1` is the version that shipped the `peacenotwar`
sabotage payload (March 2022). It is **documented in public
advisories** and should be flagged by any sensible security audit.
The bot's `security_audit` node is expected to surface it either via
`osv-scanner`, the GHSA advisory DB, or LLM judgement on the package
name + version.
