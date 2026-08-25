---
id: 001
title: Repository kickoff
date: 2026-08-24
status: complete
repos_touched: [incus-spire-attestor]
related_sessions: []
---

## Goal
Bootstrap the incus-spire-attestor project: create the repository, research the
SPIRE and Incus sides of a node attestor for Incus VMs (target: IncusOS
clusters), and produce a reviewed architecture plus an implementation plan for
v1.

## Outcome
Goal met. Repository created from meigma/template-go (still template state —
no implementation code yet). Research complete across SPIRE plugin contracts,
a live VM-side evidence spike, and IncusOS hardware-trust chaining. v1
architecture drafted, adversarially reviewed, complexity-reviewed, and approved
by the user. Implementation plan produced under a no-design-change constraint.
Implementation begins next session at PLAN.md Phase 0.

## Key Decisions
- v1 evidence design: guest claims + server-side Incus API cross-check +
  `user.*` config-nonce challenge -> only channel validated by the spike that
  proves agent-in-instance with zero new host infrastructure. Payload is a
  versioned evidence array so v2 (TPM-signed instance docs via incus-osd) and
  v3 (vTPM EK certs) slot in without replumbing.
- TPM evidence deferred -> Incus runs swtpm without EK certificates (verified
  on host), so vTPM binding needs enrollment infra; IncusOS posture gate
  deferred to v1.1 as API reachability from SPIRE server is not guaranteed.
- Selectors and SPIFFE ID derive exclusively from the Incus API snapshot;
  payload claims only locate the instance. Agent ID suffix = lowercase
  `volatile.uuid`; `can_reattest=true`; no AgentStore/TOFU (first observation
  adds no evidence).
- Per-attempt random nonce key `user.spire.attestor.nonce.<128-bit hex>` ->
  same-instance concurrency safety; ETag-protected one-key mutations; cleanup
  armed before SetNonce and run under `context.WithoutCancel`.
- Honest TCB: any Incus principal with `can_view` on instance config can pass
  the check; server credential needs `can_edit` (blast radius is an explicit
  deployment gate, documented, not hidden).

## Changes
- `.journal/001/ARCHITECTURE.md` — approved v1 architecture (this session's
  primary artifact).
- `.journal/001/PLAN.md` — 9-phase implementation plan (Phase 0 baseline
  through Phase 8 sandbox e2e).
- No repository source changes; tree is untouched meigma/template-go state.

## Open Threads
- License: template ships no LICENSE file but release metadata claims
  `Apache-2.0 OR MIT`. Must resolve before publishing any release.
- Phase 8 prerequisite: sandbox01 has no SPIRE deployment; decide whether to
  provision throwaway SPIRE server/agent for e2e.
- `can_edit` credential scoping for the server plugin must pass deployment
  review (Incus cannot scope writes to a key prefix).
- Upstream opportunities (post-v1): incus-osd signing per-instance identity
  docs with its TPM-resident key (v2); Incus invoking swtpm_setup for EK
  certs (v3).
- `spike` VM left running on sandbox01 (Ubuntu 24.04 VM with vtpm device,
  user.spire.test config key set) — reusable for Phase 8 or delete freely.

## Lessons
- Incus guest API (`/dev/incus/sock`) is read-only for config; host-set
  `user.*` keys are instantly guest-visible — this asymmetry is the residency
  proof. SMBIOS `product_uuid` == `volatile.uuid`; guest vsock CID ==
  `volatile.vsock_id`.
- IncusOS already has a hardware-TPM-resident signing key
  (incus-osd/internal/auth/token.go, pub via Security API `tpm_public_key`) —
  the v2 building block exists upstream; the feature gap is per-instance
  signing only.
- Guest apt egress is blocked on sandbox01; install guest packages via host
  or answer questions host-side.

## References
- `.journal/001/ARCHITECTURE.md` — approved v1 architecture
- `.journal/001/PLAN.md` — v1 implementation plan
- `.journal/001/NOTES.md` — full research log (spike transcripts, review
  findings)
- SPIRE plugin SDK: https://github.com/spiffe/spire-plugin-sdk
- SPIRE extending docs: https://spiffe.io/docs/latest/planning/extending/
- IncusOS security model: https://linuxcontainers.org/incus-os/docs/main/reference/security/
- IncusOS TPM auth key: https://github.com/lxc/incus-os/blob/main/incus-osd/internal/auth/token.go
- swtpm_setup EK certs: https://github.com/stefanberger/swtpm/blob/master/man/man8/swtpm_setup.pod
- Community TPM plugin (prior art): https://github.com/spiffe/spire-tpm-plugin
