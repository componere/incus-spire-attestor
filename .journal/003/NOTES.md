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

## 2026-08-25 18:30 — P1 verified; new blocker P5 found
User landed the network changes (gw01 VLAN40->mgmt allow, tailnet ACL
tag:sandbox -> 10.10.10.11-14:8443/tcp, sandbox01 accept-routes). Verified
live from sandbox01: BackendState=Running, tag:sandbox, accept-routes=true;
GET /1.0 returns 200 on all four members; adjacent probes .14:22, .15:8443,
.14:8444 all dropped. New finding: members present the bootstrap cluster
cert (CN=root@nas01.glab.lol, SAN nas01.glab.lol+loopback only, no CA:TRUE)
— the committed incus-cluster.crt (fingerprint 27:86:A3...) is NOT deployed
(live EA:49:30...). Host adapter uses ConnectionArgs.TLSCA (CA-pool + SAN
verification) and sandbox01 cannot resolve glab.lol, so the endpoint must be
IP with an IP SAN. Added P5 to FUNCTEST_PLAN.md: run
`moon run fleet-cluster:certificate` before leg C; endpoint changed to
https://10.10.10.14:8443 with tls_ca_path = committed incus-cluster.crt.

## 2026-08-25 19:55 — Cluster cert converged (P5 resolved)
Escrowed key found on unmerged branches: secrets feat/cluster-tls-key adds
fleet/shared/cluster-tls.sops.yaml; fleet fix/tls-key-secret-path repoints
TLS_KEY_SOPS_PATH there (master still points at the old fleet/cluster/
path). Verified escrowed key pubkey sha256 == committed incus-cluster.crt
pubkey before swapping. All members Online precheck passed. Ran
`CI= moon run fleet-cluster:certificate` from the fix worktree with
GLAB_SECRETS_DIR at the secrets feat worktree (AWS_PROFILE=lab-admin; user
completed SSO login; PGP decrypt path stalls ~60s on pinentry before KMS
succeeds). Result: all four members present 27:86:A3... (was bootstrap
EA:49:30...). From sandbox01: curl --cacert incus-cluster.crt against
10.10.10.11/.14:8443/1.0 -> 200, ssl_verify_result=0 (plugin TLSCA
semantics proven). Operator nas01 remote re-pinned automatically.
Open threads: both supporting branches unmerged (user's flow to PR/merge);
sandbox01 DNS for glab.lol still NXDOMAIN via 10.10.40.1 despite reported
fix — leg C uses IP endpoint regardless.

## 2026-08-25 20:05 — Supporting branches merged
Squash-merged via gh after green CI: fleet PR #8
"fix(cluster): escrow the TLS key under fleet/shared" and secrets PR #36
"feat(fleet): escrow the Incus cluster TLS key". Both local masters
fast-forwarded and verified: fleet master TLS_KEY_SOPS_PATH now
fleet/shared/cluster-tls.sops.yaml; secrets master contains the escrow.
Live cluster state and repo state are now consistent. Merged-branch
worktrees (.wt/fix-tls-key-secret-path in fleet, .wt/feat-cluster-tls-key
in secrets) left in place for their owners to prune.

## 2026-08-25 21:20 — Functional test run complete: all pass
Executed the full plan; results in .journal/003/RESULTS.md. Highlights:
SPIRE v1.15.3; plugins from master 80dd5c3 (agent 3444a5fb…, server
8dc0b0bb…). Leg S (sandbox01/spike): S1-S7 PASS incl. standalone
location substitution, selector refresh, container denial (both DMI-unreadable
and explicit type-denial layers), UUID-mismatch denial before mutation,
no-hint search, 1ms challenge timeout with clean nonce removal. Leg C
(glab cluster): C0-C4 PASS incl. real location:lab01 selector via nas01
endpoint (cross-member), stop-move migration lab01→lab02 with same agent ID
and location:lab02 refresh, restricted cert scoped to spire-test project
(default project invisible/403) proving project-scoped can_edit containment.
Both environments restored to captured pre-state (spike keeps pre-existing
user.spire.test; cluster back to default-only; sandbox listener/trust
removed). No product defects found. Ops notes: transient systemd-run units
vanish when stopped (re-run, don't restart); unmanaged fast40 needs
macvlan+profile-NIC mask; incus move needs explicit dest syntax for
--target; sandbox01 glab.lol DNS still NXDOMAIN.
