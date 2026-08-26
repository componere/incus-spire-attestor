---
id: 003
title: Functional test of v1 on real hardware
date: 2026-08-25
status: complete
repos_touched: [incus-spire-attestor, GilmanLab/fleet, GilmanLab/secrets]
related_sessions: [001]
---

## Goal
Propose and execute a functional test plan proving the incus-spire-attestor
v1 plugin pair end-to-end on real hardware: the full matrix on sandbox01
(standalone Incus) and the basics plus cluster-only behavior on the glab
IncusOS cluster.

## Outcome
Goal met. All 11 tests passed on both environments with zero product defects
(`.journal/003/RESULTS.md`). Plugins built from master `80dd5c3`, SPIRE
v1.15.3. Both environments were restored to captured pre-state. Two lab
prerequisites were resolved along the way: network path from sandbox01 to the
cluster API (user landed gw01/tailnet changes; verified with positive and
deny-boundary probes) and cluster TLS convergence (executed here).

## Key Decisions
- Split matrix by environment capability -> standalone `location:none` ->
  `server_name` substitution is only testable on sandbox01; real
  `location:<member>` selectors, cross-member API resolution, and migration
  refresh are only testable on the cluster. Basics ran on both.
- Dropped forced two-instance ambiguity and same-instance concurrency from
  the functional plan -> `volatile.uuid` uniqueness makes them synthetic;
  both are unit/race covered.
- Used an Incus **restricted certificate** scoped to a throwaway
  `spire-test` project for the cluster leg -> doubles as deployment-review
  evidence: the full flow (lookup + nonce set/unset) works while the rest of
  the cluster is invisible (empty list) and inaccessible (403). This answers
  session 001's `can_edit` blast-radius open thread.
- Converged the committed cluster cert before leg C -> the host adapter uses
  `ConnectionArgs.TLSCA` (CA-pool + SAN verification), sandbox01 cannot
  resolve `glab.lol`, and only the committed cert has IP SANs and CA:TRUE.
  Verified the escrowed key's pubkey against the cert before the one-way swap.
- Ran SPIRE server/agents as transient `systemd-run` units and asserted nonce
  hygiene by key **name** only -> no nonce value was ever printed or retained.

## Changes
- `.journal/003/FUNCTEST_PLAN.md` — the reviewed two-leg functional test plan
  (P1–P5 prerequisites, S1–S7, C0–C4).
- `.journal/003/RESULTS.md` — full evidence: versions, checksums, per-test
  outcomes, deviations, blast-radius note.
- `GilmanLab/fleet` PR #8 (merged) — `fix(cluster): escrow the TLS key under
  fleet/shared`; `GilmanLab/secrets` PR #36 (merged) — `feat(fleet): escrow
  the Incus cluster TLS key`. Then converged: all four members now present
  the committed `incus-cluster.crt`.
- No incus-spire-attestor source changes; every test credential, listener,
  VM, project, and trust entry created for the run was removed.

## Open Threads
- sandbox01 cannot resolve `glab.lol` names (NXDOMAIN via 10.10.40.1) despite
  a reported DNS fix; leg C used the IP endpoint by design, but the fix is
  not effective from that host.
- Recommend documenting the restricted-certificate deployment pattern
  (project-scoped `can_edit`) in `docs/docs/explanation/security-model.md` or
  the deploy how-to — it is currently proven only in journal evidence.
- `spike` VM left running on sandbox01 with pre-existing `user.spire.test`
  key, as found; reusable or deletable freely.

## Lessons
- Restricted Incus certs make unauthorized projects *invisible* (empty list,
  403 on direct GET) rather than erroring — good containment, and the plugin
  flow needs nothing beyond its allowed project.
- In containers the guest DMI read fails with permission denied before the
  instance-type check fires; both denial layers are real and were exercised.
- Transient `systemd-run` units vanish once stopped — re-run `systemd-run`,
  don't `systemctl restart`. Unmanaged VLAN networks (`fast40`) need
  `nictype=macvlan,parent=...` with the profile NIC masked by a `none`
  device. Cluster-member moves need `incus move <r>:x <r>:x --target <m>`.

## References
- `.journal/003/FUNCTEST_PLAN.md`, `.journal/003/RESULTS.md`,
  `.journal/003/NOTES.md`
- `.journal/001/SUMMARY.md` — architecture and plan this run validated
- https://github.com/GilmanLab/fleet/pull/8,
  https://github.com/GilmanLab/secrets/pull/36
- SPIRE v1.15.3: https://github.com/spiffe/spire/releases/tag/v1.15.3
