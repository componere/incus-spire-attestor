---
id: 002
title: Begin v1 implementation
started: 2026-08-25
---

## 2026-08-25 09:15 — Kickoff
Goal for the session: Review session 001's approved architecture and implementation plan, complete the first implementation slice with bounded programmer and reviewer subagents, verify it, and open a pull request.
Current state of the world: Session 001 completed the v1 design and plan; the repository remains in its template state with implementation scheduled to begin at PLAN.md Phase 0.
Plan: Review the approved artifacts, identify the first bounded slice, create an isolated implementation worktree, implement and review the slice, verify behavior, then commit, push, and open a PR.

## 2026-08-25 09:20 — First slice selected
Reviewed session 001's approved architecture and implementation plan. The architecture and dependency ordering are internally consistent for implementation; a bounded reviewer is checking for blockers.
Decision: Treat all of Phase 0 as the first baseline slice. Sub-slice 0.1 establishes the final module namespace while retaining the temporary executable; sub-slice 0.2 adds pinned, reproducible mock generation before application interfaces arrive in Phase 2.
Worktrees: `feat/phase-0-baseline` is the integration branch; isolated programmer branches `agent/phase-0-module` and `agent/phase-0-mockery` own disjoint sub-slices.

## 2026-08-25 09:27 — Phase 0 baseline ready
Correction to the 09:20 entry: the first self-verifying slice is Phase 0.1 plus the mockery tool pin and four-platform lock from Phase 0.2. `.mockery.yaml` is deferred until the Phase 2 ports exist; committing it now would leave mock generation broken through Phases 0 and 1.
Architecture review: no blocker for this first slice. Before later phases, define the cross-package `GuestEvidence` absence/transience result, correct Incus client context propagation so concurrent calls do not mutate a shared client, reconcile the guest instance-name source/parsing rule, and make the Phase 2-to-3 dependency edges explicit.
Implementation: module and internal imports now use `github.com/componere/incus-spire-attestor`; goimports uses the same local prefix; Go remains 1.26.6; mockery 3.7.4 is pinned through the aqua backend with checksummed locks for Linux/macOS on amd64/arm64. Two Programmer agents implemented isolated sub-slices; bounded design and code reviewers assessed them.
Verification: `go test ./...` passed, `golangci-lint fmt --config .golangci.yml --diff` was empty, `go mod tidy` produced no diff, `mise lock --platform linux-x64,linux-arm64,macos-x64,macos-arm64` reproduced the lock with no diff, and `mockery version` returned v3.7.4. The implementation branch is clean and tracks no `.journal` files.

## 2026-08-25 09:28 — Pull request opened
Pushed `feat/phase-0-baseline` and opened PR #6, `chore: establish phase 0 project baseline`: https://github.com/componere/incus-spire-attestor/pull/6
Session 002 remains in progress for the next implementation slice.

## 2026-08-25 09:31 — Phase 0 merged
PR #6 passed CI and was squash-merged into `master` as `26607b9572908329744bcd530adee1d9e94153ca`.
Next: implement the first Phase 1 pure-domain slice from the updated default branch.

## 2026-08-25 09:36 — Domain foundation started
Next slice boundary: PLAN.md Phase 1.1 only, implementing the pure `internal/attest` domain rules and behavior tests. Phase 1.2 wire codecs and Phase 1.3 configuration remain separate.
Branch/worktrees: `feat/domain-foundation` integrates isolated `agent/domain-production` and `agent/domain-tests` work. A bounded Reviewer agent is checking the API contract while both Programmer agents implement against the same fixed contract.
Dependencies: added `github.com/google/uuid` v1.6.0 and `github.com/stretchr/testify` v1.11.1 as slice prerequisites; `go mod tidy` will set their final direct/indirect classification after integration.

## 2026-08-25 09:59 — Domain foundation verified
Implemented `internal/attest` with canonical UUIDs, exact nonce-key grammar and attempt-key formatting, required guest/API claim validation, VM-only matching, fixed-size nonce construction and constant-time verification, and API-only identity/selectors with reserved-namespace, count, and value-byte limits.
Contract review corrected the selector byte metric to values only, required canonicalization at matching/identity boundaries, applied named domain field types, added the attempt-ID key constructor, and added defense-in-depth against nonce selectors. Code review found no blocker; its actionable findings were addressed by validating all API identity fields in `BuildAttributes`, reusing normalized UUID results inside `MatchClaims`, and adding producer/validator round-trip coverage.
Verification: `go test -race ./internal/attest`, `go test ./...`, and `golangci-lint run --config .golangci.yml ./internal/attest` all pass. Formatting is clean. The remaining review suggestions—introducing additional trust-domain/location/agent-ID types—were intentionally deferred because SPIRE core configuration owns trust-domain validation and those types are outside the approved Phase 1.1 surface.

## 2026-08-25 10:01 — Domain foundation PR opened
Pushed `feat/domain-foundation` and opened PR #7, `feat(attest): add pure domain foundation`: https://github.com/componere/incus-spire-attestor/pull/7
Session 002 remains in progress pending review and the next slice.

## 2026-08-25 10:13 — Phase 1.1 merged
PR #7 passed CI and was squash-merged into `master` as `2229c2b506be4f98520ee97e8cca345e90c88bd1`.
User directed completion of all remaining Phase 1 work before stopping. Phase 1.2 wire codecs and Phase 1.3 pure HCL configuration will proceed in parallel from the merged domain baseline, then land together through one reviewed integration PR.

## 2026-08-25 10:17 — Remaining Phase 1 parallelized
Created `feat/phase-1-complete` plus isolated production/test worktrees for `internal/wire` and `internal/config`. Four Programmer agents are implementing Phase 1.2 and 1.3 concurrently; two bounded Reviewer agents are checking the fixed contracts.
Wire contract: direct v1 JSON codecs over `attest` types with 64 KiB, UTF-8, exact-one-value, unknown-field, type/version, key, and RawURLEncoding nonce enforcement.
Config contract: strict HCL v2.24.0 decoding with pure separate validation, exact approved fields/defaults/limits, and no TLS-file or Incus I/O.

## 2026-08-25 10:58 — Phase 1 implementation verified
Integrated Phase 1.2 `internal/wire` and Phase 1.3 `internal/config` from four isolated Programmer worktrees. Wire provides direct strict v1 payload, challenge, and response codecs with a 64 KiB limit, UTF-8 and single-value checks, recursive duplicate-member rejection, exact type/version contracts, structural/domain error separation, canonical UUIDs, exact config keys, and canonical 16-byte unpadded base64url nonces. Config provides strict HCL decoding, presence-based duration defaults, pure accumulating validation, exact project/selector limits, reserved nonce-namespace rejection, and no filesystem or Incus I/O.
Bounded Reviewer passes found no blockers after corrections. Wire follow-up rejected null project hints, removed attacker-controlled values from errors, added exact-length strict nonce decoding, and removed a redundant full-message copy. Config review was PR-ready; whitespace-only required values and explicit empty optional agent projects remain non-blocking considerations outside the exact Phase 1.3 contract.
Verification: `go test ./...`, `go test -race ./internal/attest ./internal/wire ./internal/config`, `golangci-lint run --config .golangci.yml ./...`, `go mod tidy`, and `git diff --check` all pass.

## 2026-08-25 11:00 — Phase 1 pull request opened
Pushed `feat/phase-1-complete` and opened PR #8, `feat(phase1): add wire codecs and HCL configuration`: https://github.com/componere/incus-spire-attestor/pull/8
The implementation branch is clean. CI and GitHub Pages checks started after the PR opened.

## 2026-08-25 11:02 — Phase 1 pull request ready
PR #8 CI and GitHub Pages checks passed. GitHub reports the pull request as clean and mergeable.

## 2026-08-25 11:13 — Phase 1 merged
PR #8 passed review and CI and was squash-merged into `master` as `5f9ba223f6693a6c242f895badba01b0b7d456f1`.
Next: implement PLAN.md Phase 2 application ports and services from the updated default branch.

## 2026-08-25 11:19 — Phase 2 application work started
Created `feat/phase-2-applications` from merged `master`, added deterministic Mockery v3 configuration, and split Phase 2 into isolated `agent/phase2-agent` and `agent/phase2-server` worktrees.
Two Programmer agents own the complete `internal/agent` and `internal/server` slices. A bounded Reviewer is checking the fixed consumer-owned ports, timeout/cancellation behavior, resolution semantics, random challenge construction, cleanup precedence, and Phase 3 adapter compatibility while implementation proceeds.

## 2026-08-25 12:02 — Phase 2 applications verified
Implemented the pure `internal/agent` and `internal/server` application services with consumer-owned ports and Mockery-generated test doubles. Agent behavior now enforces payload-first exchange ordering, exact challenge decoding, bounded exponential config polling, context-first transient classification, and secret-safe config-read errors. Server behavior validates claims before lookup, resolves one allowlisted instance, uses serialized independent attempt-key/nonce pairs, applies stage-specific deadlines, guarantees exactly-once detached cleanup, preserves primary error classification with cleanup diagnostics, and emits attributes only after successful verification and cleanup.

Two bounded re-reviews report both packages PR-ready with all prior findings resolved. Verification passed: `mockery --config .mockery.yaml`, `go mod tidy`, `go test ./...`, `go test -race ./internal/agent/... ./internal/server/...`, `golangci-lint run --config .golangci.yml ./...`, and `git diff --check`.

## 2026-08-25 12:04 — Phase 2 pull request ready
Pushed `feat/phase-2-applications` and opened PR #9, `feat(phase2): add attestation application services`: https://github.com/componere/incus-spire-attestor/pull/9
GitHub CI and GitHub Pages checks passed. GitHub reports the pull request as clean and mergeable.

## 2026-08-25 12:15 — Phase 2 merged
PR #9 passed review and CI and was squash-merged into `master` as `17f66e6fef6799cb27ea664d08f95fc57076b262`.
Next: implement PLAN.md Phase 3 Incus guest and host adapters from the updated default branch.

## 2026-08-25 12:22 — Phase 3 Incus adapter work started
Created `feat/phase-3-incus-adapters` from merged `master` and isolated `agent/phase3-guest` and `agent/phase3-host` worktrees. Two Programmer agents own the independent guest Unix-socket and host Incus v7.3 adapter slices; one bounded Reviewer is checking the fixed contract while implementation proceeds.

Grounded the guest contract against Incus v7.3 `dev-incus` documentation: `/1.0` is raw JSON, `/1.0/meta-data` is plain cloud-init metadata, config values are plain text, and the instance name comes from `local-hostname`. Grounded host construction against the pinned client source: loaded CA material maps to `ConnectionArgs.TLSCA`, per-operation contexts require concrete `ProtocolIncus.WithContext`, and idle replacement closes only the underlying HTTP client's idle connections.

## 2026-08-25 13:09 — Phase 3 Incus adapters verified
Implemented `internal/incus/guest` with a real Unix-socket HTTP transport, bounded raw guest API parsing, first-generated metadata precedence over appended operator metadata, canonical DMI UUID claims, exact escaped config-key reads, explicit not-found/transient/permanent classification, and secret-safe context errors. Implemented `internal/incus/host` over `github.com/lxc/incus/v7` v7.3.0 with detached API mapping, project-scoped context clones, ETag copy-on-write exact-key mutations, operation waits, context-bounded retries, target revalidation, cleanup-safe unset semantics, nonce-safe diagnostics, and idle-only connection closure.

Correction to the 12:22 note: pinned v7.3 `ProtocolIncus.WithContext` mutates its receiver rather than cloning it. Each host operation must call `UseProject` first to obtain a real clone and then apply `WithContext` through the package-private contextual seam; calling `WithContext` on the shared runtime client would race concurrent attestations.

The bounded contract review found and corrected this context-order issue plus guest metadata precedence and exact status classification before implementation completed. Two bounded package reviews and re-reviews report both adapters PR-ready with all findings resolved. Verification passed: `go mod tidy`, `go test ./...`, `go test -race ./internal/incus/guest/... ./internal/incus/host/...`, `golangci-lint run --config .golangci.yml ./...`, and `git diff --check`.

## 2026-08-25 13:11 — Phase 3 pull request opened
Committed the reviewed adapter integration as `ea0399a` and opened PR #10, `feat(incus): add guest and host adapters`: https://github.com/componere/incus-spire-attestor/pull/10. GitHub reports the pull request mergeable; CI and GitHub Pages checks started.

## 2026-08-25 13:16 — Phase 3 merged
PR #10 passed CI and GitHub Pages checks and was squash-merged into `master` as `ce79ffd408a7be28e790258992ab8f9203a04257`.
Next: complete PLAN.md Phase 4 plugin entrypoints and configuration registration from the updated default branch.

## 2026-08-25 13:29 — Phase 4 SPIRE service work started
Created `feat/phase-4-spire-services` from merged `master` and isolated four bounded Programmer worktrees for shared immutable runtime/config construction, agent SDK translation, server SDK translation, and command cutover. The fixed contract requires Config Validate to remain side-effect free; Configure to serialize complete runtime builds and publish one atomic snapshot; attestation RPCs to load one snapshot; the server receive deadline to use one handler-local goroutine; superseded server clients to close idle connections only; and template CLI/Cobra/Viper code to be removed without compatibility aliases.

## 2026-08-25 14:08 — Phase 4 SPIRE services verified
Implemented `internal/spire` with SPIRE Plugin SDK v1.15.0 agent/server NodeAttestor and Config v1 services. Validate performs only HCL/core validation; Configure serializes complete runtime construction, atomically publishes immutable snapshots, preserves the old runtime on failure, and retires only superseded idle Incus connections. Agent and server stream adapters translate protobuf streams into the application-owned exchanges; the server challenge response uses one handler-local receive goroutine so its service deadline and cancellation remain enforceable. SDK `plugintest` coverage exercises both Config and NodeAttestor clients, fail-closed startup, stream ordering, reconfiguration failure/success/concurrency, in-flight snapshot retention, idle retirement, server-side receive deadline, and cancellation.

Replaced the template command with `incus-agent` and `incus-server`, removed Cobra/Viper/template metadata paths, and minimally retargeted the existing Moon build task to both commands so Phase 4 remains CI-compatible before the fuller Phase 5 build rewrite. Three bounded reviews and re-reviews report the agent, server, and runtime/command slices PR-ready with all findings resolved.

Verification passed: `go mod tidy`, `go test -race ./internal/spire`, `go test ./...`, `go build ./cmd/incus-agent ./cmd/incus-server`, `moon run root:build`, `golangci-lint run --config .golangci.yml ./...`, and `git diff --check`.

## 2026-08-25 14:09 — Phase 4 pull request opened
Pushed `feat/phase-4-spire-services` at `a10af3e` and opened PR #11, `feat(spire): add plugin services and command entrypoints`: https://github.com/componere/incus-spire-attestor/pull/11. GitHub reports the pull request mergeable; CI and GitHub Pages checks queued.

## 2026-08-25 14:12 — Phase 4 merged
PR #11 passed CI and GitHub Pages checks and was squash-merged into `master` as `9886bb2d32a3db6cf1b9c864005081c8360aee69`.
Next: complete PLAN.md Phase 5 build, packaging, release, and security-scan cutover for both plugin binaries.

## 2026-08-25 14:28 — Phase 5 build and release work started
Started the approved PLAN.md Phase 5 cutover after PR #11 merged. Scope is limited to dual-architecture builds for both plugin commands, raw GoReleaser binaries plus native packages, removal of the template container path, release workflow removal of OCI publication, and binary filesystem vulnerability scans. A bounded contract review is checking one discovered mismatch: the pinned reusable pre-publish workflow does not upload raw `formats: [binary]` artifacts.

## 2026-08-25 14:55 — Phase 5 build and release cutover verified
Implemented Phase 5 on `feat/phase-5-build-release`. Moon now builds `incus-agent` and `incus-server` as static Linux `amd64` and `arm64` executables. Removed Melange, apko, the `image-local` task, their pins/lock entries, generated-artifact ignores, and the OCI release jobs.

GoReleaser now produces four checksum-addressable raw Linux binaries, one APK/DEB/RPM per architecture with both executables under `/usr/bin`, binary/package SBOMs, checksums, and the checksum Sigstore bundle. Release Please was reset to `incus-spire-attestor` `0.0.0`; inherited template changelog history was removed. Removed the inherited `Apache-2.0 OR MIT` package claim because the project license remains unresolved.

The pinned reusable pre-publish workflow cannot upload raw `formats: [binary]` artifacts. The bounded repository-local resolution keeps the pinned reusable GitHub Release publisher but stages GoReleaser assets in `release.yml`, resolves raw upload names to nested paths through `dist/artifacts.json`, enforces exactly four raw binaries, and assembles only checksum-covered payloads. Snapshot verification produced 20 checksum entries: four binaries, four binary SBOMs, six native packages, and six package SBOMs; the closed bundle contained those 20 payloads plus `checksums.txt` and its Sigstore bundle, and every checksum verified.

Three bounded reviews and targeted re-reviews report the build, package, and workflow slices PR-ready after fixes for Moon failure propagation/metadata, unsigned-default nFPM templates, Linux-only targets, truthful license metadata, and raw binary path resolution. Verification passed: four-architecture `file` assertions, `mise lock` regeneration, `go mod tidy`, `go test ./...`, `moon run root:build`, `golangci-lint run --config .golangci.yml ./...`, `goreleaser check`, unsigned `goreleaser release --snapshot --clean`, native APK content inspection, release bundle checksum verification, JSON/YAML parsing, and `git diff --check`. Dispatched Security Scan run 32903450590 on the branch; both `amd64` and `arm64` matrix jobs cross-built both plugins, completed Trivy filesystem/library scans, and uploaded distinct SARIF results: https://github.com/componere/incus-spire-attestor/actions/runs/32903450590.

Next: open the Phase 5 pull request, then complete PLAN.md Phase 6 documentation and template-metadata removal after merge.

## 2026-08-25 14:56 — Phase 5 pull request opened
Pushed `feat/phase-5-build-release` at `ad451a8` and opened PR #12, `build(release): package and scan both plugins`: https://github.com/componere/incus-spire-attestor/pull/12. GitHub reports the pull request mergeable; CI and GitHub Pages checks queued.

## 2026-08-25 14:58 — Phase 5 pull request checks passed
PR #12 passed CI, GitHub Pages, and Trivy checks. The pull request remains mergeable and ready for review.

## 2026-08-25 15:01 — Phase 5 merged
PR #12 was squash-merged into `master` as `83c786b3910b8856d76a1e1117dc2b4540f3b7da`.
Next: complete PLAN.md Phase 6 user documentation and remove remaining template-facing repository metadata.

## 2026-08-25 15:06 — Phase 6 contract and slices bounded
Reviewed PLAN.md Phase 6 against the merged implementation and repository state. Three disjoint programmer slices own root repository documentation and `DELETE_ME.md` removal, the MkDocs site and docs tool metadata, and GitHub repository settings/tests. The documentation must preserve API-only identity authority, the fixed `incus` plugin name, the configuration defaults and timeout margin, Incus 7.3 `can_edit` blast radius, and the limit that v1 does not prove exclusive guest residency. The absent license and supported release remain explicit in README.md and SECURITY.md rather than being invented.
Created `feat/phase-6-docs` plus isolated `agent/phase-6-repo-docs`, `agent/phase-6-site-docs`, and `agent/phase-6-repo-settings` worktrees.

## 2026-08-25 15:25 — Phase 6 documentation verified
Completed PLAN.md Phase 6 on `feat/phase-6-docs`. Root documentation now describes the two external plugins, local contribution flow, private vulnerability reporting, absent supported release line, and absent project license without inventing either. The MkDocs site now provides deployment, configuration reference, and security-model pages with exact v1 HCL, API-authority, project resolution, identity/selectors, timeout margin, credential placement, `can_edit` blast radius, nonce response, cleanup, and trust limitations. Repository settings now set `is_template = false`; stale template onboarding, apko/melange skills, and template metadata are removed, and the mise skill reflects the current release toolchain.

Three Programmer agents implemented disjoint slices. Three bounded Review agents found and re-reviewed the support-status wording, nonce stream description, project resolution and guest metadata prerequisites, and stale local skills; all findings are resolved and all reviewers report PR-ready.

Verification passed: `mise exec -- moon run root:check`, five `.github/scripts/test_configure_github_repo.py` tests, `git diff --check`, and the required repository-wide placeholder search. Browser verification loaded the built MkDocs site, followed navigation from Home to Deploy, Configuration, and Security model, and confirmed the configuration fields, exclusive-residency limitation, and Incus `can_edit` warning.

Next: commit the integrated review fixes, push `feat/phase-6-docs`, and open the Phase 6 pull request.

## 2026-08-25 15:27 — Phase 6 pull request opened
Pushed `feat/phase-6-docs` at `7560f82` and opened PR #13, `docs: add product documentation and remove template remnants`: https://github.com/componere/incus-spire-attestor/pull/13. GitHub reports the pull request mergeable. CI and GitHub Pages checks passed; the deploy job correctly skipped for the pull request.

## 2026-08-25 15:39 — Phase 6 merged
PR #13 was squash-merged into `master` as `bc5d559c1d3cf34c8120e5f6ec37fdfd235568bd`.
Next: complete PLAN.md Phase 7 full local verification and release rehearsal.

## 2026-08-25 15:45 — Phase 7 verification contract bounded
Created `feat/phase-7-verification` from merged `master` plus isolated `agent/phase-7-moon`, `agent/phase-7-diagnostics`, and `agent/phase-7-release` worktrees. Phase 7 remains verification-first: fix only observed failures in owning files.

A bounded review found one source defect before execution: Moon’s `releaseConfig` still points at the removed `.github/workflows.disabled/` directory instead of the live workflows. Programmer slices own that fix, nonce-safe diagnostic/cause-retention inspection, and release artifact contract inspection.

Safety decision: run the local GoReleaser snapshot with `--skip=sign`. Snapshot mode prevents GitHub publication, but the configured Cosign stage can still initiate keyless signing and an irreversible public Rekor entry. Keep RPM/APK signing-key variables unset, require no Sigstore bundle in `dist/`, and inspect unsigned snapshot artifacts only. The unresolved license continues to block a real tag or release, not this local rehearsal.

## 2026-08-25 15:59 — Phase 7 verification and rehearsal passed
Completed PLAN.md Phase 7 on `feat/phase-7-verification`. Fixed the only observed repository defect: `moon.yml` now tracks `.github/workflows/**/*.yml` in `releaseConfig`, so workflow-only changes affect the `root:check` gate.

Verification passed from the clean worktree with mise’s locked toolchain: mockery v3.7.4 regenerated all four mocks twice with no diff; the focused attest/wire/config suite, both focused race suites, and `moon run root:check` passed; `go mod tidy` produced no diff; the targeted dependency/config search found no Cobra, Viper, Melange, apko, or template remnants.

`goreleaser check` and the safe unsigned snapshot rehearsal passed. `dist/artifacts.json` resolved exactly four uploadable raw Linux binaries. All 20 checksum entries were verified, including those four binaries through the same artifact-name-to-build-path mapping used by the release workflow. Six APK/DEB/RPM packages were inspected directly; every package for both architectures contains `/usr/bin/incus-agent` and `/usr/bin/incus-server`. No Sigstore bundle or module-proxy tree was produced.

Nonce/error inspection found no path that emits nonce bytes. Existing passing tests assert redaction across wire, agent, Incus guest/host, server, and SPIRE boundaries and preserve `context.Canceled`/`context.DeadlineExceeded` causes or their gRPC status codes. Three Programmer agents completed disjoint audits; two bounded final Review agents report the one-line patch PR-ready. Release evidence is intentionally scoped to unsigned asset layout and checksum/package closure: snapshot mode did not exercise tagged module-proxy builds or Cosign/Rekor signing, and the unresolved license still forbids a real release.

Next: push `feat/phase-7-verification` and open the Phase 7 pull request.

## 2026-08-25 16:02 — Phase 7 pull request opened
Pushed `feat/phase-7-verification` at `3f46d4e` and opened PR #14, `fix(moon): track active workflows in release checks`: https://github.com/componere/incus-spire-attestor/pull/14. GitHub reports the pull request clean and mergeable. CI and GitHub Pages checks passed; the deploy job correctly skipped for the pull request.

## 2026-08-25 16:06 — Phase 7 merged
PR #14 was squash-merged into `master` as `6367b0713448d4050e31e396d5a131ac04af5103`.
Next: execute PLAN.md Phase 8 sandbox end-to-end verification against `spike`, retain only approved evidence, and restore all sandbox state.

## 2026-08-25 16:12 — Phase 8 sandbox contract bounded
Inventory confirmed `sandbox01` runs Incus client/server 7.3 and one running `x86_64` VM, `default/spike`; no SPIRE binaries or services are installed. The VM exposes `/dev/incus/sock`, has matching cloud-init hostname `spike`, profile `default`, UUID `e90499b4-4395-418f-8dfb-f39ab7108f93`, a nonempty cloud-init instance ID, and existing selector key `user.spire.test=hello`.

Phase 8 will run SPIRE 1.15.0 in isolated temporary host/guest directories and transient systemd units. The Incus client certificate will be restricted to project `default`, Incus HTTPS will bind loopback only, and the credential files will remain host-only. Selector proof uses the existing `user.spire.test` plus one absent configured key; no test selector will be written.

The bounded contract review found six execution ambiguities in PLAN.md. Safe interpretation: freeze and hash the project-qualified API config baseline; use separate temporary SPIRE processes and data; prove timeout by both failure status and prefix-wide nonce-key absence; treat container rejection as agent-side pre-send validation; use a canonical alternate DMI UUID for server-side mismatch; and enumerate/remediate every mutation before sign-off. Any baseline divergence blocks completion.

## 2026-08-25 16:42 — Phase 8 sandbox verification completed
The first live attempt failed closed and exposed a real standalone-Incus defect: guest `/1.0` reported location `sandbox01`, while the host instance API used the standalone sentinel `none`. The server plugin now reads `environment.server_name` from host `/1.0` during Configure, caches it, and substitutes it only for standalone instance records. Clustered records remain authoritative. Focused tests cover standalone normalization, construction failures, metadata caching, and rejection of the standalone sentinel on a clustered server. User documentation now states the cache and refresh boundary.

The corrected `linux/amd64` plugin checksums were `fcfb87f7da72...654c2d01e6` for `incus-agent` and `fe65dfbe8d90...9300bd4e87` for `incus-server`. Real SPIRE 1.15.0 and Incus 7.3 attested `spike` as `spiffe://phase8.test/spire/agent/incus/e90499b4-4395-418f-8dfb-f39ab7108f93`. Re-attestation produced a new serial with `can_reattest=true`. The selectors were exactly location `sandbox01`, name `spike`, profile `default`, project `default`, `user.spire.test=hello`, and the canonical UUID; the configured absent `user.phase8.absent` selector was omitted.

Negative cases failed at the intended boundary. A disposable container was denied by agent-side virtual-machine validation before payload send. A canonical alternate DMI UUID was denied server-side as an instance UUID mismatch before nonce creation. A 1 ns challenge-response deadline returned `DeadlineExceeded` after challenge issuance and left no nonce-prefix key. Deterministic cancellation was triggered only when the fake guest API received `/1.0/config/<challenge-key>`, proving the nonce had been written; stopping the agent returned `Canceled`, and synchronous server cleanup completed with no nonce-prefix key. The normal path was rerun afterward and again left no nonce key.

All transient SPIRE processes, data, plugin binaries, trust identities, credential files, the loopback HTTPS listener, and the disposable container were removed. `sandbox01` again has one running instance, `default/spike`, with its approved UUID, cloud-init identity, `default` profile, `user.spire.test=hello`, and `root`, `eth0`, and `vtpm` devices. No Phase 8 paths or processes remain. A byte-for-byte baseline containing the full config and expanded-config maps matched before and after the final normal attestation and cleanup.

An earlier monolithic baseline was retained only as a hash and diverged after the first execution, so it could not identify the changing field. Work stopped until the sandbox was enumerated and every approved invariant and mutation was reconciled. The stable projected comparison and explicit device/process/path checks provide the completion evidence, but future sandbox runs must retain the baseline JSON rather than only its hash and must include devices in the same comparison.

The temporary `can_edit` certificate was acceptable for this isolated rehearsal because HTTPS was loopback-only, the identity was project-restricted and short-lived, and every mutation was enumerated and reversed. It is not automatically acceptable for a shared production project: Incus 7.3 cannot restrict `can_edit` to the nonce prefix, so deployment requires a deliberately isolated project or explicit acceptance of project-wide instance mutation authority.

Two bounded reviewers found no blocker or high-severity issue and judged Phase 8 complete. Their medium source finding was fixed with a clustered-sentinel regression test; documentation was narrowed to the actual Configure-time cache semantics. `go test -race ./internal/incus/host`, `git diff --check`, and `moon run root:check` pass. The existing MkDocs 2.0 advisory is informational. The unresolved repository license still blocks a real release.

Next: commit and push `feat/phase-8-sandbox`, open its pull request, and resolve the repository license before any production release.

## 2026-08-25 16:44 — Phase 8 pull request opened
Committed the standalone location fix as `8af987a`, pushed `feat/phase-8-sandbox`, and opened PR #15, `fix(host): normalize standalone instance location`: https://github.com/componere/incus-spire-attestor/pull/15. GitHub reports the branch mergeable. CI and GitHub Pages checks passed; the Pages deploy job correctly skipped for the pull request.

## 2026-08-25 16:56 — Phase 8 merged
PR #15 was squash-merged into `master` as `46707ebc14cc7f42fc930dd05fe2e2cc53d2adbd`.
Next: open a separate pull request that dual-licenses the repository under Apache-2.0 and MIT.

## 2026-08-25 17:04 — Dual-license pull request opened
Created `docs/dual-license` from merged `master`, committed `3319af7`, and opened PR #16, `docs: dual-license under Apache-2.0 and MIT`: https://github.com/componere/incus-spire-attestor/pull/16.

The change adds canonical `LICENSE-APACHE` and `LICENSE-MIT` texts with copyright held by Componere, lets recipients choose either license, applies both grants to future contributions unless explicitly stated otherwise, and preserves separately noted third-party terms. Native package metadata now uses the SPDX expression `Apache-2.0 OR MIT`.

The canonical terms match the Apache and SPDX sources. `moon run root:check`, `goreleaser check`, and an unsigned snapshot rehearsal passed. The snapshot built all APK, DEB, and RPM variants, and APK metadata reports `license = Apache-2.0 OR MIT`. GitHub reports PR #16 clean and mergeable; CI and GitHub Pages passed, and the Pages deploy job correctly skipped.

## 2026-08-25 17:08 — Dual licensing merged
PR #16 was squash-merged into `master` as `b74df32671caaa10045bb4a05796e80434a88484`.
Next: remove repository-template `MEIGMA_` prefixes from release workflow secret and variable names, then apply the committed GitHub repository settings.

## 2026-08-25 17:16 — Release credential PR opened; repository settings partially applied
Created `fix/release-credential-names` from merged `master`, committed `0e84978`, and opened PR #17, `fix(release): remove template credential prefixes`: https://github.com/componere/incus-spire-attestor/pull/17. Both release workflows now use `vars.RELEASE_APP_CLIENT_ID` and `secrets.RELEASE_APP_PRIVATE_KEY`; a repository-wide search finds no remaining `MEIGMA_` credential reference. `moon run root:check` and `git diff --check` pass. GitHub reports the PR clean and mergeable; CI and GitHub Pages passed.

The repository had no repository-scoped Actions secrets or variables. Organization-level names could not be enumerated because the current token lacks organization Actions secret/variable administration. The public client ID for `meigma-release-please` was resolved from GitHub and stored as repository variable `RELEASE_APP_CLIENT_ID`. The private key remains unavailable and was not fabricated or copied.

Ran `.github/scripts/configure_github_repo.py plan`, reviewed its exact operations, then ran `apply`. GitHub accepted the general repository settings, immutable releases, private vulnerability reporting, automated security fixes, and the active `Default branch` ruleset. The branch ruleset requires pull requests, squash merges, linear history, verified signatures, and the `ci` status check; it blocks deletion and non-fast-forward updates, with repository administrators as the explicit bypass.

Creation of the `Default tags` ruleset failed with HTTP 422 because GitHub reports that the `meigma-release-please` integration is not installed for this repository or its owner organization. A second plan confirms that this tag ruleset is the only supported configuration drift left. The script also continues to report its nine documented manual/unsupported settings.

Next: an app owner must install `meigma-release-please` for `componere/incus-spire-attestor` and provision `RELEASE_APP_PRIVATE_KEY`. Then rerun `configure_github_repo.py apply` to create the protected-tag ruleset and merge PR #17.

## 2026-08-25 17:25 — Correction: release credentials use the COMPONERE prefix
The 17:16 note inferred the wrong replacement names and incorrectly treated the inherited credentials as absent. The user confirmed that the organization-wide names and repository access already exist. PR #17 now uses `vars.COMPONERE_RELEASE_APP_CLIENT_ID` and `secrets.COMPONERE_RELEASE_APP_PRIVATE_KEY` in both release workflows. The mistakenly created repository variable `RELEASE_APP_CLIENT_ID` was deleted. Commit `c172a70` contains the correction; `moon run root:check`, `git diff --check`, CI, and GitHub Pages pass, and the PR is clean and mergeable.

Retried `configure_github_repo.py apply` after the clarification. GitHub still rejects the tag-ruleset bypass with HTTP 422: `Actor meigma-release-please integration must be part of the ruleset source or owner organization`. This response is independent of the Actions credential names. The already-applied repository settings and branch ruleset remain correct; the `Default tags` ruleset is still the only supported drift.

Next: reconcile the ruleset API’s integration-ownership response with the installed app identity or installation scope, then rerun `configure_github_repo.py apply`.

## 2026-08-25 17:31 — Release credential PR merged
The user accepted PR #17 and explicitly chose to ignore the remaining tag-ruleset configuration error. PR #17 was squash-merged into `master` as `32b73077a1c49eb3177628d68c9b2961a90cdbce`. The merged workflows use the established `COMPONERE_RELEASE_APP_CLIENT_ID` and `COMPONERE_RELEASE_APP_PRIVATE_KEY` organization settings. The rejected `Default tags` ruleset was not applied.

## 2026-08-25 17:39 — Dependabot triage completed; merges awaiting OAuth scope
Triaged all five open Dependabot pull requests and accepted each update: PR #1 `actions/checkout` 7.0.1, PR #2 `jdx/mise-action` 4.2.5, PR #3 `mkdocs-material` 9.7.7, PR #4 `actions/cache` 6.1.0, and PR #5 `github/codeql-action/upload-sarif` 4.37.8. Every PR is clean and mergeable with CI and GitHub Pages passing. The four action commit pins match their release tags, including the dereferenced annotated CodeQL tag. The MkDocs lockfile hashes match the non-yanked PyPI release, which reports no vulnerabilities.

Manually dispatched the `Security Scan` workflow from PR #5. Both amd64 and arm64 jobs built and scanned the plugin binaries and uploaded SARIF successfully. GitHub then rejected the first squash-merge attempt because the current `gh` OAuth token lacks the `workflow` scope needed to merge workflow-file changes. Device authorization is pending; no Dependabot PR has been merged yet.

## 2026-08-25 17:50 — Dependabot queue cleared
The GitHub device authorization completed and added the required `workflow` OAuth scope. All five accepted Dependabot PRs were squash-merged: #1 as `0e4641c460054d22849c62ba9dce16cb8d1b903a`, #2 as `10a99ec68c6e982fb3c365dcf4ae9a11a116ac5e`, #3 as `be0ade254bb3d1122dea951f5f894ff2ad47f565`, #4 as `a328e088365ac6a79e3290d821261fb7d7c50929`, and #5 as `80dd5c316dc98c4ed5758f82aa5907b5ba18832f`. No open Dependabot pull requests remain. On the final `master` head, CI, GitHub Pages, and Release Please completed successfully.
