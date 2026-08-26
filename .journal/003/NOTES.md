---
id: 003
title: New work session
started: 2026-08-25
---

## 2026-08-25 17:53 — Kickoff
Goal for the session: not yet stated; the user asked to open a new session and
will provide the task next.
Current state of the world: v1 architecture and plan approved (session 001,
`.journal/001/ARCHITECTURE.md` / `PLAN.md`). Session 002 is in-progress on the
first v1 implementation slice. Master worktree is 11 commits behind origin;
open branches include `fix/release-credential-names` (ahead, unmerged) and many
integrated phase branches under `.wt/`. Phases 1–8 branches exist; several show
integrated state. LICENSE resolution appears addressed by `docs/dual-license`
(integrated).
Plan: await the user's request, then scope and log work here.

## 2026-08-25 18:05 — Functional test plan proposed
Goal clarified: propose a two-environment functional test plan (sandbox01 +
glab IncusOS cluster) proving the plugin pair end-to-end on real hardware.
Grounded against lab2: cluster members nas01/lab01-03 (mgmt 10.10.10.11-14,
shared cluster cert :8443), instances on VLAN 40; sandbox01 standalone Incus
7.3 with no HTTPS listener yet. Key finding: gw01 drops VLAN40->mgmt and
tailnet ACLs give tag:sandbox no lab-range access, so the SPIRE Server on
sandbox01 cannot reach the cluster API without a narrow ACL/firewall change
(plan prerequisite P1, recommended: tailnet ACL tag:sandbox ->
10.10.10.11-14:8443). Plan written to .journal/003/FUNCTEST_PLAN.md: deep
matrix on sandbox01 (happy path incl. standalone location substitution,
re-attest/selector refresh, container + UUID-mismatch denials, no-hint
search, timeout/nonce-cleanup), cluster basics + cluster-only (real
location:<member> selector, cross-member API resolution, migration ->
location refresh, restricted-cert project scoping as can_edit evidence).
Awaiting user review.
