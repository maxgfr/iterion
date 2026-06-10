# ADR-018: Bot-secret binding scope-tightening — enforce AllowedHosts, drop the unenforceable scope fields

- **Status**: Accepted
- **Date**: 2026-06-10
- **Authors**: devthejo
- **Code**:
  [pkg/secrets/bindings.go](../../pkg/secrets/bindings.go) (`BotSecretBinding.AllowedHosts`),
  [pkg/secrets/generic.go](../../pkg/secrets/generic.go) (`GenericResolution.AllowedHosts`),
  [pkg/secrets/run_secrets.go](../../pkg/secrets/run_secrets.go) (`RunBundle.GenericSecretHosts`),
  [pkg/secrets/credentials.go](../../pkg/secrets/credentials.go) (`Credentials.GenericHosts`),
  [pkg/server/cloudpublisher/publisher.go](../../pkg/server/cloudpublisher/publisher.go) (resolve loop populates the host map),
  [pkg/runner/loop.go](../../pkg/runner/loop.go) (threads it onto `Credentials`),
  [pkg/backend/model/secretguard.go](../../pkg/backend/model/secretguard.go) (`effectiveSecretHosts`, the intersection)

## Context

A `BotSecretBinding` makes a stored org/user secret resolvable for a
specific bot under the name the bot's workflow declares. The binding type
shipped three "scope tightening" fields — `AllowedHosts`,
`AllowedWorkflowFiles`, `AllowedNodeIDs` — and the code comments + the
feature's intent advertised `AllowedHosts` as an egress control that
"intersects (never broadens) the workflow secret's declared egress hosts."

A production-readiness review found all three were **stored but never
enforced**: `IntersectHosts` had no non-test caller, the publisher set
`GenericResolution.AllowedHosts` but dropped it, `RunBundle` had no
per-secret host channel, and the runner's secret guard built its egress
allowlist solely from the workflow's own `secrets.<name>.hosts`. The core
risk is the binding feature's *raison d'être*: the unattended/webhook case
where an org admin binds an org credential to a bot whose workflow
declares the secret with **no** `hosts:`. The admin's `allowed_hosts`
restriction did nothing, so that credential was exfiltratable anywhere —
a documented security control that could not function.

The review's instruction was explicit: enforce it or remove it; leaving it
stored-but-unenforced is the unacceptable middle.

## Decision

Make `AllowedHosts` a real, end-to-end-enforced egress control, and
**remove** `AllowedWorkflowFiles` and `AllowedNodeIDs`.

- **Enforce `AllowedHosts`**: the resolver already carries it on
  `GenericResolution.AllowedHosts`; the publisher now threads it onto a new
  `RunBundle.GenericSecretHosts` (sealed per-run); the runner puts it on
  `Credentials.GenericHosts`; and `BuildSecretGuard` intersects it with the
  workflow's declared hosts per secret via the new `effectiveSecretHosts`.
- **Remove the other two fields** from the binding type and the CRUD route.

The intersection has one non-obvious rule. `secretguard` treats an **empty
`Hosts` list as "any host allowed."** A naive intersection of two
*disjoint* allowlists yields the empty list, which would therefore
**broaden** egress to *anywhere* — the exact opposite of containment.
`effectiveSecretHosts` represents a disjoint result as a deny-all sentinel
(`[""]`, an unmatchable host, since `hostMatch` returns false for the empty
pattern) so a binding can only ever narrow:

| workflow hosts | binding hosts | effective              |
|----------------|---------------|------------------------|
| empty          | empty         | empty (unrestricted)   |
| set            | empty         | workflow (unchanged)   |
| empty          | set           | binding (narrows)      |
| both set, overlap | —          | intersection           |
| both set, disjoint | —         | deny-all (`[""]`)      |

## Alternatives considered (and rejected)

1. **Pure removal of all three fields** (the review's option (b)). Honest
   and lowest-risk, but it deletes the org admin's *only* egress lever for
   a bound credential in the unattended/webhook path — precisely the case
   the binding feature exists for. Rejected because `AllowedHosts` has a
   real, tractable enforcement point; removing a working-once-wired
   security control weakens the product's posture.
2. **Keep all three, relabel them "reserved / not yet enforced."** This is
   the "unacceptable middle" — it still ships a credential-egress control
   that does nothing while telling operators it exists.
3. **Wire all three.** `AllowedNodeIDs` is architecturally unenforceable
   under the current model: secrets are resolved and **sealed once per run**
   and materialised for the whole run; there is no per-node secret gating
   at execution time. Enforcing it would require a separate run-time
   mechanism, out of scope here. `AllowedWorkflowFiles` *is* enforceable at
   the publisher (filter bindings by the launching workflow file), but it is
   a low-blast-radius over-grant *within an already-authorised bot*, not the
   credential-exfiltration risk; threading the workflow path through the
   resolver was not worth it for this fix. Both were removed rather than
   shipped as façades.

## Consequences

- `AllowedHosts` on a bot-secret binding is now enforced: it can only
  narrow (or leave unchanged) where a bound credential may egress, never
  broaden — including the disjoint-policy case, which denies all egress.
- The binding type and `POST/PATCH /api/teams/{id}/bots/{bot_id}/bindings`
  lose `allowed_workflows` / `allowed_nodes`. This is an API change, but
  those fields never had any effect, so no real behaviour is lost.
- `RunBundle` gains an additive `generic_secret_hosts` field
  (`omitempty`), so previously sealed bundles remain decodable.
- Per-node secret scoping, if ever wanted, needs a distinct execution-time
  mechanism (the secret guard runs per-run, not per-node) — noted here so a
  future contributor doesn't re-add an `AllowedNodeIDs` field expecting the
  resolution/sealing layer to honour it.
