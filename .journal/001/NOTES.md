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
