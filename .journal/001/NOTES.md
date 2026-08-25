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
