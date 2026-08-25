---
id: 001
title: Repository kickoff
started: 2026-08-24
---

## 2026-08-24 19:39 — Kickoff
Goal for the session: Begin work on incus-spire-attestor after creating the repository and enabling session journals.
Current state of the world: The public repository exists from meigma/template-go, is cloned locally, and the personal journal branch is initialized.
Plan: Wait for the first implementation request, then work incrementally from the template baseline.

## 2026-08-24 19:45 — SPIRE-side breakdown
Project goal: a SPIRE node attestor for Incus VMs (IncusOS clusters).
Researched SPIRE plugin architecture; grounded against spire-plugin-sdk (nodeattestor v1 protos) and SPIRE 1.15 docs.
What we build: a NodeAttestor plugin pair — agent plugin (in-VM, `AidAttestation` stream, emits opaque payload + answers challenges) and server plugin (`Attest` stream, verifies evidence, returns `AgentAttributes{spiffe_id, selectors, can_reattest}`). External go-plugin binaries wired via `plugin_cmd`/`plugin_checksum`; plugin name "incus" on both sides becomes selector type and agent ID namespace (`spiffe://<td>/spire/agent/incus/<suffix>`).
Key facts: payload is opaque to SPIRE — it is our versioned contract between the plugins; challenge/response optional and plugin-driven; Config service v1 delivers HCL plugin_data; AgentStore host service supports TOFU/anti-replay checks; SDK ships templates and a plugintest package.
Planned shape: one repo, two binaries (cmd/incus-attestor-agent, cmd/incus-attestor-server), shared payload schema + hexagonal verification core with the Incus/TPM authority behind a port.
Open decision: evidence archetype — platform-document (aws_iid-style, via /dev/incus guest API + server-side Incus API verify), hardware-rooted (vTPM quote), or hybrid. Leaning platform-document first for the prototype; payload seam allows swapping evidence later.
Next: break down the Incus side (what evidence a VM can actually produce).

## 2026-08-24 20:05 — VM-side evidence spike (sandbox01, Incus 7.3)
Launched `spike` VM (images:ubuntu/24.04 --vm) on sandbox01 and probed every guest-visible identity channel.
Guest API (`/dev/incus/sock`): GET /1.0 → state, instance_type, `location` (cluster member); /1.0/meta-data → cloud-init instance-id + local-hostname (instance name); /1.0/config → only `user.*`/cloud-init keys; /1.0/devices → NIC hwaddr. All read-only: guest PATCH of config is a silent no-op (only `state=Ready` PATCH works). No guest→host arbitrary-data channel.
SMBIOS `product_uuid` == `volatile.uuid` (host-assigned, guest-readable, verifiable via Incus API). meta-data instance-id == `volatile.cloud-init.instance-id`. Guest reads own vsock CID via ioctl 0x7b9 on /dev/vsock == `volatile.vsock_id` (verified equal: 2592790405).
Host→guest channel confirmed: `incus config set spike user.spire.test=hello` is immediately readable in-guest at /1.0/config/user.spire.test.
vTPM: `incus config device add spike vtpm tpm` (cold-add only) → TPM 2.0 at /dev/tpm0, backed by `swtpm socket --tpm2` with NO --create-ek-cert: EK exists but no EK certificate/CA chain → TPM binding needs per-instance EK enrollment. Defer.
Nothing guest-readable is signed — all claims. Leading design: agent payload carries claims (name, uuid, location, hwaddr, CID); server plugin cross-checks via Incus API, then proves residency with a config-nonce challenge: server sets `user.spire.nonce=<rand>` on the claimed instance via Incus API, agent echoes it from the guest API, server unsets it. Both directions of that channel verified in the spike.
Guest apt egress is blocked on sandbox01 (mirrors unreachable); tpm2-tools uninstallable in-guest — answered EK question from host-side swtpm invocation instead.
`spike` VM left running on sandbox01 for the next spike.
Next: validate the nonce round-trip end-to-end and the Incus API surface the server plugin needs (instance GET + config PATCH permissions, project scoping).

## 2026-08-24 20:25 — IncusOS hardware-trust chain research
Question: can we chain proof from the host hardware TPM to VM identity?
IncusOS facts (docs + source): measured boot binds LUKS disk encryption to PCRs 4/7/11/15; all state (/var/lib) sits on that TPM-gated encrypted volume. incus-osd creates a TPM-resident ECC signing key (tpm2_createprimary -C e → tpm2_create → evictcontrol; incus-osd/internal/auth/token.go), public half at /var/lib/incus-os/tpm-auth.pem and exposed via Security API `tpm_public_key`; signs machine auth tokens in-TPM (tpm2_sign) for provider/Operations Center registration. Security API reports secure_boot_enabled, tpm_status (ok|pcr mismatch|swtpm — swtpm use permanently recorded), system_state_is_trusted.
Implicit chain exists today: cluster TLS/private keys only become available after PCR-verified boot unlocks the disk, so anything authenticated by cluster credentials transitively asserts trusted-booted IncusOS. Our Incus-API-verifying design inherits this.
Missing for explicit per-VM crypto chain: nothing signs per-instance identity docs with the TPM key; vTPM EKs uncertified (verified in spike); no remote quote endpoint; guest API read-only so guest can't request host signatures. IncusOS is immutable with a curated app list (app-build/applications.json) — no third-party host daemons deployable, so host-side signer requires upstream feature work.
Plan: v1 = claims + Incus API cross-check + user.* nonce + host posture gate (server plugin checks IncusOS Security API per member: tpm_status==ok, secure_boot_enabled, system_state_is_trusted; optionally pin tpm_public_key at enrollment; expose host trust as selectors). v2 (upstream proposal to lxc/incus + lxc/incus-os) = incus-osd signs per-instance identity documents with the TPM-resident key, delivered via /dev/incus; agent payload becomes the signed doc; server verifies against enrolled tpm_public_key. Alternative upstream: vTPM EK certs from a per-host TPM-rooted CA (swtpm supports EK cert provisioning).
Caveats recorded: tpm-auth key is not PCR-policy-bound at use time (gated by boot-time disk unlock, so runtime host compromise can still sign); it is a plain signing key, not an AK/quote — no boot-state freshness per message.
Keep evidence seam pluggable so v1 nonce evidence and v2 signed-doc evidence swap cleanly.

## 2026-08-24 20:40 — vTPM EK-cert flavor detailed
Grounded against swtpm_setup man page: `--create-ek-cert --vmid <id>` provisions EK + EK cert into vTPM NVRAM via pluggable create_certs_tool (default swtpm-localca); --vmid embeds instance UUID in cert; since v0.11 also IAK/IDevID keys+certs; swtpm-localca signingkey supports pkcs11 URIs → host CA key can live in hardware TPM.
Chain: HW TPM → per-host swtpm-localca CA (pkcs11 or on TPM-gated disk, certified by tpm-auth key) → vTPM EK cert (vmid=instance uuid) → AK via credential activation → guest PCR quotes. Server verifies offline against enrolled cluster-member CAs; Incus API off hot path.
Buys: standard TPM attestation flow (spire-tpm-plugin reuse possible), guest boot-integrity attestation (guest secure/measured boot → selectors), offline verifiability. Costs: mandatory upstream Incus change (invoke swtpm_setup at instance create), per-host CA lifecycle, migration = origin-host CA vs current host, most moving parts. Does NOT raise security ceiling — vTPM state is host-readable file; host remains trust root (ceiling only moves with SEV-SNP/TDX).
Ranking unchanged: v1 nonce → v2 TPM-signed instance docs (incus-osd) → v3 EK certs if guest measured boot becomes a requirement.

## 2026-08-24 21:05 — v1 architecture drafted, reviewed, simplified
Ran the requested pipeline: software-architect draft → architecture-reviewer (adversarial) → architect revision → complexity-reviewer → final architect pass. One bounded feedback round per reviewer.
Review round surfaced and fixed: no enforceable server-side deadline on the challenge-response phase (added challenge_response_timeout + adapter-local receive goroutine); non-atomic config mutation (ETag-protected copy-on-write one-key updates with verify-on-conflict retry); cleanup suppression on RPC cancellation (context.WithoutCancel + cleanup_timeout, armed before SetNonce); per-attempt random key suffix instead of fixed nonce key; honest TCB statement — any Incus principal with can_view over instance config can satisfy the check, and the client cert needs can_edit (blast radius called out as a deployment gate).
Complexity round cut: Configure generation/lease/retirement subsystem (→ atomic pointer snapshot), evidence handler registry (→ direct v1 type switch), global admission semaphore, nonce-residue scavenger (residue is inert; maintenance note only), release-checklist/test-inventory bloat, design-history appendix.
Final doc: .journal/001/ARCHITECTURE.md (~4 pages). Key decisions: JSON wire v1 with strict bounded codecs, evidence array seam for v2/v3, agent ID = lowercase volatile.uuid, can_reattest=true, no AgentStore/TOFU, selectors from API snapshot only (project/name/location/uuid/profile/user.*), fail-closed on Incus API unavailability.
Next: user review of ARCHITECTURE.md, then implementation.
