# Functional test results — 2026-08-25/26

Product: incus-spire-attestor @ master `80dd5c3`.
SPIRE v1.15.3 (musl, linux-amd64). Binaries built via `mise exec -- moon run
root:build`; installed-file SHA-256 (== `plugin_checksum`):

- `incus-agent`  `3444a5fbb207da472dad582b3f6811c3d5fd55de32318fed79541f74f7a9633f`
- `incus-server` `8dc0b0bbf766a63bbc06178b5a6aae185e27a1d0aeb39beea206dbfe37002cb0`

Trust domain `ft.glab.lol`. All nonce assertions were made by key **name**
only; no nonce value was printed, logged, or retained.

## Leg S — sandbox01 (standalone Incus, VM `spike`, uuid `e90499b4-…8f93`)

| Test | Result | Evidence |
|---|---|---|
| S1 happy path | PASS | Agent ID `spiffe://ft.glab.lol/spire/agent/incus/e90499b4-4395-418f-8dfb-f39ab7108f93`; `can_reattest=true`; selectors exactly `location:sandbox01` (standalone `none`→`server_name` substitution), `name:spike`, `profile:default`, `project:default`, `user.environment:functional-test`, `uuid:…`; workload SVID `spiffe://ft.glab.lol/workload/ft` fetched in-guest (uid 0). |
| S2 re-attest + refresh | PASS | Same agent ID after restart; `user.environment` selector refreshed `functional-test`→`refreshed` from API truth. |
| S3 container denial | PASS | Two layers: unreadable host DMI in container → transient-retry failure with no send; readable fake UUID → permanent `attestation denied: guest instance type "container" is not "virtual-machine"` (agent exits, no retry). Zero nonce keys ever on the container. |
| S4 UUID mismatch denial | PASS | `BindReadOnlyPaths` wrong-UUID over `product_uuid` (unit-scoped; production path untouched) → server `attestation denied: instance uuid mismatch`; zero nonce keys → denial before mutation. |
| S5 no-hint search | PASS | Agent `plugin_data {}` with server `projects=["default","ft-proj2"]` → single match resolved, same agent ID, SVID issued. |
| S6 timeout + cleanup | PASS | `challenge_response_timeout="1ms"` → server `receive response: context deadline exceeded` (standard cause preserved); zero `user.spire.attestor.nonce.*` key names after `cleanup_timeout`; restored config re-attested successfully. |
| S7 restore | PASS | `ft-proj2` deleted, `user.environment` unset, listener/trust/`~/spire-ft`/guest dirs removed; final state == captured pre-state (only pre-existing `user.spire.test` remains on spike). |

## Leg C — glab IncusOS cluster (VM `spire-ft`, uuid `985e6b37-…922d`)

Server plugin on sandbox01 → `https://10.10.10.14:8443`, `tls_ca_path` =
committed `incus-cluster.crt`, **restricted** client cert scoped to project
`spire-test`.

| Test | Result | Evidence |
|---|---|---|
| C0 prep | PASS | Restricted cert: `spire-test` instances visible; `default` project list empty; direct cross-project instance GET → HTTP 403. VM launched on lab01 via macvlan on `fast40` (10.10.40.202). |
| C1 happy path | PASS | Agent ID `…/incus/985e6b37-3844-4001-b229-17d8658e922d`; selectors `location:lab01` (real member, not substitution), `project:spire-test`, `user.environment:functional-test`, profile/name/uuid; workload SVID `…/workload/ft-cluster` fetched in-guest. |
| C2 cross-member | PASS | Endpoint member nas01 while instance on lab01 — resolution is cluster-wide (same run as C1). |
| C3 migration refresh | PASS | Stop-move lab01→lab02, re-attest: **same agent ID**, selector `location:lab02`; SVID issued. |
| C4 cleanup + restore | PASS | Zero nonce-prefix keys on `spire-ft`; VM, project, restricted trust entry removed; cluster back to `default`-only. |

## can_edit blast-radius note (deployment review)

An Incus **restricted certificate** limited to a dedicated project executed
the full attestation flow (lookup + nonce set/unset) while the rest of the
cluster remained invisible (empty list) and inaccessible (403). Project-scoped
`can_edit` is a workable containment for the v1 credential; recommend this as
the documented deployment pattern.

## Deviations from plan

- S3's plan wording assumed the type check fires first; in a container the
  DMI read fails before it (permission denied). Both denial layers verified.
- S4/S5/S6 restarts used `systemd-run` transient units; a stopped unit
  disappears (re-run, not `systemctl restart`).
- Cluster VM NIC: `fast40` is unmanaged, so `nictype=macvlan,parent=fast40`
  with the profile NIC masked by a `none` device.
- sandbox01 still cannot resolve `glab.lol` (NXDOMAIN via 10.10.40.1);
  leg C ran against the IP endpoint per plan.

## Verdict

End-to-end proof complete on real hardware for both environments: attestation,
identity/selector correctness, workload SVID issuance, negative denials
(container, UUID mismatch), timeout/cleanup hygiene, migration refresh, and
scoped-credential containment. No product defects found.
