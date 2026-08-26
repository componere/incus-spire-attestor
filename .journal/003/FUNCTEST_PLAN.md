# Functional test plan — incus-spire-attestor v1 on real hardware

Goal: prove the shipped product end-to-end — SPIRE Agent in a real Incus VM
attests through the `incus` plugin pair and a workload obtains an SVID — on
both lab environments. Deep matrix runs once on sandbox01; the IncusOS
cluster runs the basics plus cluster-only behavior. No test is repeated on
both unless it exercises environment-specific code.

## Environments

| | sandbox01 (leg S) | glab IncusOS cluster (leg C) |
|---|---|---|
| Incus | Zabbly stable 7.3, standalone | IncusOS members nas01/lab01–03 |
| API endpoint | `https://10.10.40.10:8443` (must be enabled) | `https://10.10.10.14:8443` (IP; glab.lol does not resolve from sandbox01) |
| Instance network | `incusbr0` (NAT) | VLAN 40 (`fast40`, 10.10.40.0/24 DHCP from gw01) |
| SPIRE Server host | sandbox01 itself | sandbox01 (see prerequisite P1) |
| SPIRE Agent host | test VM on sandbox01 | test VM on a cluster member |
| Mutability | expendable; mutate freely | production-shaped; project-scoped, restore everything |

Both environments are amd64. Binaries: `moon run root:build` snapshot from
the release candidate commit; record SHA-256 of every deployed executable
(they become `plugin_checksum`). Pin one SPIRE release (latest stable 1.x
compatible with spire-plugin-sdk v1.15.0) and use it for both legs.

## Prerequisites and open decisions

- **P1 (RESOLVED 2026-08-25, leg C): server→cluster-API path.** gw01 now
  permits VLAN 40 → mgmt, tailnet ACL grants `tag:sandbox` →
  `10.10.10.11-14:8443/tcp` only, and sandbox01 enrolls with
  `--accept-routes`. Verified from sandbox01: all four members answer
  `GET /1.0` over HTTPS; adjacent port/address probes (`.14:22`, `.15:8443`,
  `.14:8444`) are dropped.
- **P5 (blocking, leg C): cluster certificate convergence.** All members
  still present the bootstrap cert (`CN=root@nas01.glab.lol`, SAN only
  `nas01.glab.lol` + loopback, no CA:TRUE). The host adapter passes
  `tls_ca_path` as `ConnectionArgs.TLSCA` — standard CA-pool + SAN
  verification — and sandbox01 cannot resolve `glab.lol`, so the endpoint
  must be an IP with a matching IP SAN. Run
  `moon run fleet-cluster:certificate` to converge the committed
  `incus-cluster.crt` (CA:TRUE, DNS+IP SANs for all members) before C0;
  then `tls_ca_path` = that committed cert.
- **P2 (leg S): sandbox01 Incus API.** Enable `core.https_address :8443`,
  generate a test client cert, `incus config trust add`. sandbox01's deploy
  is the reset path; still record and revert these mutations.
- **P3 (leg C): scoped credential.** Create project `spire-test` on the
  cluster and trust a **restricted** client certificate limited to that
  project. This doubles as the deployment-review evidence for the `can_edit`
  blast radius (smallest practical scope).
- **P4: guest image.** Ubuntu cloud VM image (cloud-init present so
  `volatile.cloud-init.instance-id` is set and `local-hostname` equals the
  instance name). sandbox01 guest egress is restricted — push SPIRE + plugin
  binaries via `incus file push`, never guest apt.

## Hygiene rules (both legs)

- Never print, log, or retain nonce **values**; assert cleanup by key
  **names** only (`incus config show` filtered to `user.spire.attestor.nonce.`).
- Capture pre-state of every mutated surface before the first test; final
  step diffs against it.
- Evidence per test: command transcript, SPIRE server/agent versions, plugin
  SHA-256s, Incus versions, pass/fail against the stated criterion. Store in
  `.journal/<session>/` notes.

## Leg S — sandbox01, full matrix

- **S0 Prep.** Enable HTTPS API (P2); build/hash binaries; install SPIRE
  Server on host, SPIRE Agent + `incus-agent` in VM `spike` (reuse if still
  present); configure both NodeAttestors per docs (`projects=["default"]`,
  one `user_selectors` key, e.g. `user.environment`, set on the VM).
- **S1 Happy path (basic).** Attest. Pass: agent ID is
  `spiffe://<td>/spire/agent/incus/<lowercase volatile.uuid>`;
  `can_reattest=true`; selectors exactly match the live API snapshot
  (project, name, uuid, profiles, `user.environment`); **`location:` equals
  sandbox01's `server_name`** (standalone substitution path). Then create a
  registration entry parented on the agent ID and fetch a workload X.509 SVID
  from inside the VM — the product's actual value delivered.
- **S2 Re-attest + selector refresh (basic).** Change the VM's
  `user.environment` value, restart SPIRE Agent. Pass: same agent ID;
  selector reflects the new API value.
- **S3 Container denial.** Same agent config in a disposable container in an
  allowed project. Pass: agent plugin rejects (`instance_type=container`)
  before any payload; no nonce key ever appears on the container.
- **S4 Mismatched-UUID denial.** Disposable agent service in the VM with a
  read-only bind mount of a wrong-UUID file over
  `/sys/class/dmi/id/product_uuid`. Pass: server denies; no instance config
  mutation observed; override removed and normal attestation restored.
- **S5 No-hint search.** Omit agent `project`; server `projects` lists
  `default` plus a second (empty) project. Pass: attestation succeeds via
  the full search; single match resolved.
- **S6 Timeout + cleanup.** Disposable server config with tiny
  `challenge_response_timeout` (below agent `poll_timeout`). Pass:
  attestation fails with a deadline; **no `user.spire.attestor.nonce.*` key
  name remains** after `cleanup_timeout`. Restore documented values and prove
  one final successful attestation.
- **S7 Restore.** Delete disposable container/agent overrides/test projects,
  assert zero nonce-prefix keys on every instance, diff against S0 pre-state.

Dropped as not functionally reachable/valuable: forced two-instance
ambiguity (UUID uniqueness makes it synthetic; unit-covered), concurrent
same-instance attests (unit-covered with race tests).

## Leg C — glab cluster, basics + cluster-only

- **C0 Prep.** Verify P1 probe; create `spire-test` project + restricted
  cert (P3); `tls_ca_path` is the committed cluster cert
  (`fleet/cluster/tls/incus-cluster.crt`); launch VM `spire-ft` in
  `spire-test` on lab01 (VLAN 40); point a second SPIRE Server instance on
  sandbox01 at `https://10.10.10.14:8443` with
  `projects=["spire-test"]`; agent `project = "spire-test"`.
- **C1 Happy path (basic).** Attest + workload SVID fetch, as S1. Pass
  additionally requires **`location:lab01`** — the real cluster-member
  selector, not the substitution path.
- **C2 Cross-member resolution.** Endpoint stays nas01 while the instance
  lives on lab01 (already true in C1 — assert it explicitly): proves the
  server resolves instances cluster-wide regardless of which member serves
  the API. No extra run needed; evidence is C1's endpoint vs location.
- **C3 Migration refresh (cluster-only).** `incus move spire-ft --target
  lab02` (stop-move acceptable), restart SPIRE Agent. Pass: same agent ID;
  `location:lab02` selector; everything else unchanged.
- **C4 Cleanup + restore.** Assert no nonce-prefix key names on `spire-ft`;
  delete the VM, `spire-test` project, restricted cert trust entry; revert
  the P1 network allowance; confirm cluster state matches C0 capture.

## Exit criteria

All S and C tests pass with recorded evidence; both environments restored;
the `can_edit` scoping note (P3 outcome) written up for the deployment
review thread. Failures found in the product stop the run, get fixed on a
branch, and re-run only the affected tests plus S1/C1.

## Out of scope

TPM/IncusOS-posture evidence (v2+), performance/load, arm64 hardware (none
in lab), HA SPIRE topologies, production OCI images.
