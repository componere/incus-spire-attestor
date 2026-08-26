---
id: 002
title: Begin v1 implementation
date: 2026-08-25
status: complete
repos_touched: [incus-spire-attestor]
related_sessions: [001]
---

## Goal
Review session 001's approved architecture and implementation plan, implement the first bounded slice with parallel Programmer agents and bounded reviews, verify it, and open a pull request. The user subsequently expanded the session to complete the full v1 plan, repository licensing and release setup, and outstanding dependency updates.

## Outcome
The expanded goal was met. The repository moved from its template baseline to a complete v1 SPIRE NodeAttestor plugin pair for Incus, with pure domain/application layers, real guest and host adapters, SPIRE SDK services, reproducible packaging, user documentation, live sandbox verification, dual licensing, repository configuration, and current dependencies. PRs #6 through #17 and Dependabot PRs #1 through #5 were reviewed, verified, and squash-merged. Local `master` was fast-forwarded and all session implementation worktrees were removed.

A real v1.0.0 release was intentionally not published. At the user's direction, generated Release Please PR #18 was closed and its branch deleted. Release Please can generate a new release PR after future changes.

## Key Decisions
- Preserve the session 001 v1 architecture: guest claims plus server-side Incus API authority, with a single-use config nonce challenge. This provides agent-to-instance correlation without making guest claims authoritative.
- Keep identity and selectors API-only. Guest-provided name, UUID, location, hardware address, and cloud-init ID are cross-check inputs, never selector or identity sources.
- Use consumer-owned ports and immutable runtime snapshots. This keeps domain/application behavior testable without I/O and makes SPIRE reconfiguration fail closed without disrupting in-flight attestations.
- Clone the Incus client with `UseProject` before applying `WithContext`. Incus v7.3 mutates the receiver in `WithContext`; applying it to the shared client would race concurrent attestations.
- Normalize standalone Incus location records from cached `environment.server_name`. Live Phase 8 testing exposed that guest `/1.0` reports the server name while standalone host records use the sentinel `none`.
- Publish two static plugin binaries and native APK, DEB, and RPM packages; remove the inherited OCI image path. This matches the SPIRE plugin deployment surface and avoids an unused container artifact.
- Dual-license the project under Apache-2.0 or MIT. This resolved the release-blocking mismatch between package metadata and the previously absent project license.
- Use the established `COMPONERE_RELEASE_APP_CLIENT_ID` and `COMPONERE_RELEASE_APP_PRIVATE_KEY` organization settings. No compatibility aliases or template-prefixed names remain.
- Accept repository settings except the protected-tag ruleset. GitHub continued to reject the release-app bypass actor with HTTP 422; the user explicitly chose to ignore that configuration drift.
- Close Release Please PR #18 without publishing v1.0.0. Production release side effects remain a separate explicit decision.

## Changes
- `internal/attest/` — canonical claims, identity/selector rules, nonce construction and verification, and domain errors.
- `internal/wire/` — strict bounded v1 JSON payload, challenge, and response codecs.
- `internal/config/` — strict HCL decoding and pure validation for agent and server configuration.
- `internal/agent/` and `internal/server/` — attestation application services, consumer-owned ports, deadlines, retry classification, and exactly-once cleanup.
- `internal/incus/guest/` and `internal/incus/host/` — Unix-socket guest API and project-scoped Incus v7.3 host adapters, including standalone location normalization.
- `internal/spire/` — SPIRE Plugin SDK agent/server NodeAttestor and Config services with immutable runtime publication.
- `cmd/incus-agent/` and `cmd/incus-server/` — production plugin entrypoints replacing the template CLI.
- `moon.yml`, `.goreleaser.yaml`, `mise.toml`, and `mise.lock` — reproducible dual-architecture builds, native packaging, SBOMs, checksums, signing integration, and locked tooling.
- `.github/workflows/` — release, CI, documentation, and binary security-scan workflows using pinned actions and Componere release credentials.
- `README.md`, `CONTRIBUTING.md`, `SECURITY.md`, and `docs/` — deployment, configuration, security-model, contribution, and support documentation.
- `LICENSE-APACHE` and `LICENSE-MIT` — canonical dual-license terms and contribution policy.
- `.github/repository-settings.toml` and `.github/scripts/configure_github_repo.py` — repository settings and ruleset automation; general settings and the default-branch ruleset were applied.
- Dependabot PRs #1–#5 — updated checkout, mise, MkDocs Material, cache, and CodeQL upload dependencies.

## Open Threads
- Production v1.0.0 publication is deferred. Release Please PR #18 was closed without merging; no tag or GitHub release was created.
- The `Default tags` ruleset remains unapplied because GitHub rejects the configured `meigma-release-please` bypass actor as outside the ruleset source or owner organization. The user accepted this drift for now.
- Production deployment must isolate or explicitly accept the Incus 7.3 project-wide mutation authority granted by `can_edit`; Incus cannot restrict it to the nonce key prefix.
- Future sandbox rehearsals should retain the complete projected baseline JSON, including devices, rather than only its hash.
- Material for MkDocs 9.x is approaching end of life on 2026-11-05; the documentation toolchain will need a deliberate successor or migration.
- TPM-backed evidence remains deferred to a future protocol version and requires enrollment infrastructure.

## Lessons
- Live Incus/SPIRE execution found a standalone-location mismatch that unit and integration tests could not expose. Functional verification remains required for trust-path changes.
- A mutation rehearsal needs a retained, structured baseline. A hash proves divergence but cannot identify or remediate the changed field.
- Snapshot release mode is not automatically side-effect free: signing can still create a public transparency-log entry. Use `--skip=sign` for local artifact rehearsals.

## References
- `.journal/001/ARCHITECTURE.md` — approved v1 architecture.
- `.journal/001/PLAN.md` — implementation phases completed in this session.
- PRs [#6](https://github.com/componere/incus-spire-attestor/pull/6), [#7](https://github.com/componere/incus-spire-attestor/pull/7), [#8](https://github.com/componere/incus-spire-attestor/pull/8), [#9](https://github.com/componere/incus-spire-attestor/pull/9), [#10](https://github.com/componere/incus-spire-attestor/pull/10), [#11](https://github.com/componere/incus-spire-attestor/pull/11), and [#12](https://github.com/componere/incus-spire-attestor/pull/12) — implementation and release construction.
- PRs [#13](https://github.com/componere/incus-spire-attestor/pull/13), [#14](https://github.com/componere/incus-spire-attestor/pull/14), and [#15](https://github.com/componere/incus-spire-attestor/pull/15) — documentation, verification, and live-sandbox fix.
- PRs [#16](https://github.com/componere/incus-spire-attestor/pull/16) and [#17](https://github.com/componere/incus-spire-attestor/pull/17) — dual licensing and release credential names.
- Dependabot PRs [#1](https://github.com/componere/incus-spire-attestor/pull/1), [#2](https://github.com/componere/incus-spire-attestor/pull/2), [#3](https://github.com/componere/incus-spire-attestor/pull/3), [#4](https://github.com/componere/incus-spire-attestor/pull/4), and [#5](https://github.com/componere/incus-spire-attestor/pull/5) — dependency updates.
- [PR #18](https://github.com/componere/incus-spire-attestor/pull/18) — generated v1.0.0 release PR, closed without publication.
