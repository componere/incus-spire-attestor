# incus-spire-attestor v1 implementation plan

## Execution rules and dependency order

- Treat `.wt/journal-jmgilman/.journal/001/ARCHITECTURE.md` as the contract. Do not add TPM/IncusOS evidence, AgentStore/TOFU, metrics, admission control, nonce scavenging, production OCI images, or additional plugin configuration.
- Preserve the existing Go 1.26.6, mise, Moon, golangci-lint, MkDocs, Release Please, and reusable `meigma/release` conventions unless a cutover step below explicitly removes template-only machinery.
- Keep every package documented with `doc.go`; document every declaration and struct field per `AGENTS.md`.
- Dependency chain: **Phase 0 → Phase 1 → Phases 2 and 3 → Phase 4 → Phases 5 and 6 → Phase 7 → Phase 8**. Steps explicitly marked parallel may proceed concurrently after their prerequisites land.

## Phase 0 — Establish the project baseline

### 0.1 Rename the module without breaking the temporary template command

**Files:** `go.mod`, `go.sum`, `.golangci.yml`, `cmd/template-go/main.go`, `internal/cli/root.go`, `internal/config/config.go`

- Change the module to `github.com/componere/incus-spire-attestor` and update the existing internal imports immediately so the repository stays buildable during the staged cutover.
- Change `formatters.settings.goimports.local-prefixes` in `.golangci.yml` to the new module path.
- Keep Go at 1.26.6. Add `github.com/spiffe/spire-plugin-sdk` at `v1.15.0` and `github.com/lxc/incus/v7` at `v7.3.0` when their first importing packages are added. Add direct test/decoder dependencies only when used, then let `go mod tidy` determine the indirect graph.

**Why:** this creates one import namespace before new packages appear, while postponing deletion of the still-working template executable until both real composition roots exist.

**Verify:** `go test ./...` and `golangci-lint fmt --config .golangci.yml --diff` pass after the rename.

### 0.2 Add reproducible mock generation

**Files:** `mise.toml`, `mise.lock`, new `.mockery.yaml`, later generated files under `internal/agent/mocks/` and `internal/server/mocks/`

- Pin mockery in `mise.toml` and regenerate `mise.lock` for the four platforms already carried by the lock (`linux-x64`, `linux-arm64`, `macos-x64`, `macos-arm64`); never hand-edit `mise.lock`.
- Configure `.mockery.yaml` to generate only the four application-port mocks defined in Phase 2: `agent.GuestEvidence`, `agent.Exchange`, `server.Incus`, and `server.Exchange`. Fix output names and packages so regeneration is deterministic.
- Do not create mocks for domain values or private helpers. The stateful fake required for the official Incus `InstanceServer` adapter test is a fake, not an additional production port.

**Why:** the approved package layout names these mock packages, and repository rules prohibit handwritten mocks.

**Verify:** once Phase 2 interfaces exist, run `mise exec -- mockery`; a second run must produce no diff, and `go test ./internal/agent/... ./internal/server/...` must compile the generated mocks.

## Phase 1 — Implement the pure foundation

### 1.1 Domain rules (`internal/attest`) — independent of 1.3

**Files:** new `internal/attest/doc.go`, `types.go`, `claims.go`, `nonce.go`, `attributes.go`, `errors.go`, and corresponding `*_test.go` files

- Define the approved domain types: `ProjectName`, `InstanceName`, `InstanceUUID`, `InstanceType`, `ConfigKey`, `Nonce`, `Claims`, `Instance`, and `Attributes`. `Instance` must contain only the authoritative Incus snapshot fields needed by the contract: project, name, UUID, type, location, cloud-init ID, profiles, and expanded config.
- Add `NewInstanceUUID`/UUID normalization that accepts a valid UUID representation and stores canonical lowercase form.
- Add `NewConfigKey` enforcing exact equality to `user.spire.attestor.nonce.` plus 32 lowercase hexadecimal characters. Reject uppercase hex, wrong lengths, extra segments, slash, query/fragment characters, percent ambiguity, and prefix collisions before any URL is built.
- Add pure claim validation and `MatchClaims`: required guest claims must be present, both guest and API types must be exactly `virtual-machine`, the optional project is compared only when present, and name, canonical UUID, location, cloud-init ID, and type must match. Return only the approved inspectable denial class (`attest.ErrDenied`) for evidence/identity denial.
- Add nonce construction/validation for exactly 16 bytes and `VerifyNonce`, which rejects unequal lengths before using `crypto/subtle.ConstantTimeCompare` on equal-length byte slices.
- Add `BuildAttributes` to construct `spiffe://<trust-domain>/spire/agent/incus/<canonical-api-uuid>`, set `can_reattest=true`, and derive only the approved selectors from the API `Instance`: project, name, location, UUID, each profile, and configured `user.*` values from `expanded_config`. Sort and deduplicate the final strings. Omit missing selected keys; reject over 100 selectors or over 32 KiB in aggregate selector-value bytes rather than truncating.
- Keep operational errors wrapped with context and do not add inspectable error categories beyond configuration, wire invalid/unsupported, and attestation denial.

**Tests:** table-driven tests for UUID canonicalization/malformed UUIDs; missing claims; guest/API container denial; every individual mismatch; absent project versus asserted project; nonce exact length/match/mismatch; challenge-key grammar; API-only identity/selectors; missing selected user keys; profile/selector sorting and deduplication; reserved limits at, below, and above 100 selectors and 32 KiB.

**Verify:** `go test ./internal/attest`.

### 1.2 Strict v1 codecs (`internal/wire`) — depends on 1.1

**Files:** new `internal/wire/doc.go`, `payload.go`, `challenge.go`, `response.go`, `decode.go`, `errors.go`; new `internal/wire/testdata/payload-v1.json`, `challenge-v1.json`, `response-v1.json`; corresponding `*_test.go`

- Implement direct concrete v1 encode/decode switches using `json.RawMessage` or an equivalent simple two-stage decode. Do not add a handler registry or negotiation layer.
- Expose focused functions such as `EncodePayload`/`DecodePayload`, `EncodeChallenge`/`DecodeChallenge`, and `EncodeResponse`/`DecodeResponse`; translate to and from `internal/attest` types rather than leaking JSON structs into application packages.
- Enforce 64 KiB on every opaque message, valid UTF-8, one JSON value with no trailing data, unknown-field rejection at every level, all required fields, outer version 1, exact body type/version pairs, exactly one `incus-guest-claims` evidence item, and no implicit downgrade.
- Omit `project` only when no guest project hint exists. Encode/decode response nonces with unpadded base64url and require exactly 16 decoded bytes on the server path.
- Define only `wire.ErrInvalid` and `wire.ErrUnsupported` as the inspectable wire classes; wrap the concrete decode cause without including nonce material.

**Tests/fixtures:** use the three approved JSON examples as round-trip goldens. Table-drive invalid UTF-8, byte 65,537, trailing JSON, unknown fields at each nesting level, missing fields, duplicate/wrong evidence items, wrong outer/body/type versions, padded or malformed base64url, and wrong decoded nonce sizes. Generate the oversized case in the test rather than committing a large fixture.

**Verify:** `go test ./internal/wire`.

### 1.3 HCL configuration (`internal/config`) — parallel with 1.1/1.2

**Files:** new `internal/config/doc.go`, `agent.go`, `server.go`, `validate.go`, `errors.go`, `agent_test.go`, `server_test.go`; remove the template contents of `internal/config/config.go` during Phase 4

- Define `Agent` with only `project` and `poll_timeout`, defaulting `poll_timeout` to 5 seconds.
- Define `Server` with only the approved endpoint, three TLS paths, `projects`, `user_selectors`, and the three deadlines; default them to 5 seconds, 10 seconds, and 5 seconds respectively.
- Add `DecodeAgent`, `DecodeServer`, and pure validation functions. Validate required core configuration, non-empty required server values, one to 32 distinct projects, positive durations, no more than 32 distinct `user.*` selector keys, and rejection of the reserved `user.spire.attestor.nonce.` namespace. Do not read TLS files or contact Incus here.
- Return only `config.ErrInvalid` as the inspectable configuration class. Preserve parsing/validation detail through wrapping or Config `ValidateResponse.notes` later.

**Tests:** defaults, explicit values, malformed duration, zero/negative duration, absent endpoint/TLS path/projects, project count and duplicates, selector count and duplicates, non-`user.*` keys, reserved nonce namespace, and missing core trust domain. Include a case proving that syntactically valid nonexistent TLS paths pass pure validation.

**Verify:** `go test ./internal/config`.

## Phase 2 — Implement application flows and generate mocks

These two steps are parallel after Phase 1.

### 2.1 Agent application (`internal/agent`)

**Files:** new `internal/agent/doc.go`, `ports.go`, `service.go`, `poll.go`, `service_test.go`; generated `internal/agent/mocks/doc.go`, `GuestEvidence.go`, `Exchange.go`

- In `ports.go`, define the consumer-owned `GuestEvidence` port for reading claims and one challenged config value, and `Exchange` for sending the initial payload, receiving one challenge, and sending one response. Do not add syscall-sized interfaces.
- Implement `Service`, `New`, and `Service.Attest(ctx, exchange)`. The flow must read/validate VM claims, encode/send the payload first, strictly decode one challenge before calling the guest adapter, poll a not-yet-visible key or transient guest error with an internal bounded backoff until `poll_timeout` or cancellation, and return the value in the v1 response without logging it.
- Keep `/dev/incus/sock`, DMI path, polling cadence, and nonce prefix out of plugin HCL. A constructor-only alternate timing function is permitted for deterministic tests; it is not a production configuration knob.
- Use an unexported retry classifier over standard transport/status behavior so no additional inspectable error class is introduced. Permanent guest errors fail immediately; the final timeout wraps the standard context deadline cause.

**Mock tests:** assert payload-before-receive ordering; VM rejection before any send; malformed/wrong-version/key challenge before any config read; absent/transient reads followed by success; bounded timeout; stream cancellation; permanent read failure; and no nonce text in returned errors. Assert mock expectations rather than private helper calls.

**Verify:** regenerate mocks, then `go test ./internal/agent/...`.

### 2.2 Server application (`internal/server`)

**Files:** new `internal/server/doc.go`, `ports.go`, `service.go`, `resolve.go`, `cleanup.go`, `service_test.go`; generated `internal/server/mocks/doc.go`, `Incus.go`, `Exchange.go`

- Define the `Incus` port with exactly `Lookup`, `SetNonce`, and `UnsetNonce`; make lookup absence explicit in the return values rather than adding a not-found sentinel. Define `Exchange` for the initial payload, one challenge/response, and the terminal attributes response.
- Implement `Service`, `New`, and `Service.Attest(ctx, exchange)` around an injected `io.Reader` (production passes `crypto/rand.Reader`). Decode the payload before any mutation.
- With a project hint: require allowlist membership and do one qualified lookup. Without a hint: search every allowed project under one total `incus_timeout`; continue only on not-found, abort operational errors, complete the full search even after a match, and require exactly one record that matches all claims.
- Recheck API VM type and every claim before mutation. Derive all final attributes from the retained resolved API snapshot.
- Read exactly 16 attempt-ID bytes and 16 independent nonce bytes with `io.ReadFull`; format the key suffix as 32 lowercase hex digits and the stored nonce as unpadded base64url.
- Arm deferred cleanup immediately before `SetNonce`, including the unknown-write-outcome case. Send only the key, then accept exactly one response under `challenge_response_timeout`; reject close, cancellation, timeout, malformed/wrong contract, invalid nonce, or mismatch.
- Run `UnsetNonce` with `context.WithTimeout(context.WithoutCancel(rpcCtx), cleanup_timeout)`. The host port owns transient/ETag retries within that context and treats absence as success. If verification succeeded, cleanup failure prevents the terminal attributes response. If verification failed, preserve the primary RPC class/cause while retaining the cleanup error for diagnostics, with no nonce value in either error or log data.
- Build and send the terminal `Attributes` only after successful verification and cleanup.

**Mock tests:** project-hinted lookup; complete no-hint search; not-found continuation; early auth/malformed/timeout abort; zero and multiple matches; claim/container denial before mutation; deterministic independent random reads; short/random-reader failure; set success; uncertain set outcome cleanup; cleanup after RPC cancellation; challenge timeout/closure/malformed/mismatch; verification success plus cleanup failure; primary failure plus cleanup failure; exact-key cleanup; no terminal response on any failure; API-only ID/selectors; and two concurrent attempts against the same instance using distinct keys/nonces.

**Verify:** regenerate mocks, then `go test -race ./internal/server/...`.

## Phase 3 — Implement Incus adapters

These steps are parallel after domain types and application ports exist.

### 3.1 Guest adapter (`internal/incus/guest`)

**Files:** new `internal/incus/guest/doc.go`, `client.go`, `claims.go`, `config.go`, `errors.go`, `client_test.go`; new fixtures under `internal/incus/guest/testdata/` for a `/1.0` VM envelope, `/1.0/meta-data`, one config-value response, and `product_uuid`

- Implement a Unix-domain-socket HTTP client using the fixed `/dev/incus/sock` default and fixed `/sys/class/dmi/id/product_uuid` default. Constructor-only alternate paths/transport are allowed for tests.
- Read `/1.0` for instance type and location, `/1.0/meta-data` for cloud-init instance ID and local hostname, add only the optional configured project hint, and canonicalize DMI UUID through `internal/attest`.
- Reject missing/malformed required guest data and any type other than `virtual-machine` before the application sends a payload.
- For challenged config reads, accept only an already-validated `attest.ConfigKey`, apply `url.PathEscape`, and append it as one path segment to `/1.0/config/`. Never accept a raw challenge string and never log the returned value.
- Distinguish absence/eventual visibility and transient transport/service responses for the agent polling contract while preserving permanent operational errors with context.

**Adapter tests:** serve the committed responses through a real temporary Unix socket; prove claim mapping, optional project behavior, DMI malformed/missing handling, guest container rejection, transient and not-found signals, request cancellation, and the exact recorded request path with no slash/query/percent ambiguity.

**Verify:** `go test ./internal/incus/guest`.

### 3.2 Host adapter (`internal/incus/host`)

**Files:** new `internal/incus/host/doc.go`, `client.go`, `map.go`, `mutation.go`, `retry.go`, `client_test.go`

- Implement `Client` over `github.com/lxc/incus/v7/client` at v7.3.0. `New` must use `ConnectIncusWithContext` with loaded CA/client certificate/key material; application operations must clone the concrete client with its exported `WithContext(ctx)` before `UseProject`, `GetInstance`, or `UpdateInstance` so RPC/stage deadlines reach HTTP requests.
- `Lookup` must use `UseProject(project).GetInstance(name)` and map project, name, type, canonical `volatile.uuid`, location, `volatile.cloud-init.instance-id`, profiles, and `expanded_config` into a detached domain `Instance` snapshot. Map 404 to the port’s explicit not-found result; do not treat authentication, authorization, or malformed API data as absence.
- `SetNonce`/`UnsetNonce` must refetch the current instance and ETag, confirm project/name/UUID/VM type still identify the resolved target, copy the writable `config` map, change only the exact attempt key, call `UpdateInstance`, and `WaitContext` for the operation. Unsetting an absent key succeeds without update.
- On an ETag conflict, refetch, revalidate the target, reapply the one-key change, and retry inside the supplied stage context. Retry only transient transport/service failures and ETag conflicts; never retry auth/authz, malformed data, replacement, or evidence mismatch.
- Expose a concrete idle-connection closer for runtime replacement; do not call `Disconnect` on a superseded runtime because that could terminate active requests.

**Adapter tests:** use the approved stateful fake `incus.InstanceServer` and fake `Operation` to cover project scoping, mapping, writable-copy isolation, preservation of unrelated/operator keys, exact set/unset, operation wait, absent unset, transient retry, ETag refetch/retry, context deadline, unknown set outcome, and replacement of UUID or VM type during retry. Keep this fake in `_test.go`; do not add another production interface or handwritten expectation mock.

**Verify:** `go test -race ./internal/incus/host`.

## Phase 4 — SPIRE services, immutable runtimes, and clean command cutover

### 4.1 SPIRE adapters and Config services

**Files:** new `internal/spire/doc.go`, `agent.go`, `agent_exchange.go`, `server.go`, `server_exchange.go`, `runtime.go`, `agent_test.go`, `server_test.go`

- Implement `AgentPlugin` and `ServerPlugin`, embedding the generated unimplemented NodeAttestor and Config v1 servers from `spire-plugin-sdk v1.15.0`.
- Translate SDK streams to the application-owned exchanges without leaking protobuf types into `internal/agent` or `internal/server`.
- In the server exchange, bound the blocking challenge-response `Recv` with exactly one handler-local receive goroutine and a select over the challenge deadline and stream context. Do not create a pool or reusable worker subsystem; returning from the handler must cancel/release the blocked stream receive.
- Implement `Validate` as HCL/core validation only. It must not read TLS files or connect to Incus.
- Implement `Configure` with a per-plugin mutex acquired at entry. Decode/validate, read credentials where applicable, construct a complete immutable runtime, and only then store it through an atomic pointer. A failed build leaves the previous pointer unchanged; a nil pointer fails attestation closed. Each RPC loads one pointer exactly once.
- Agent runtime: validated agent config, guest adapter, and agent service. Server runtime: trust domain, validated server config, connected host adapter, and server service using `crypto/rand.Reader`.
- After a successful server swap, call `CloseIdleConnections` on the superseded Incus client only. Do not terminate active requests; in-flight RPCs retain their old runtime naturally.
- Map only the approved inspectable classes at the gRPC edge; preserve `context.Canceled` and `context.DeadlineExceeded` as standard causes. Keep operational context wrapped. Use SPIRE-provided diagnostics only where needed for cleanup causes and never include nonce material.

**SDK tests:** use `plugintest.ServeInBackground` for both NodeAttestor clients and Config clients. Cover Validate versus Configure I/O, nil runtime fail-closed behavior, valid flow translation, malformed stream ordering, failed reconfigure retaining the old runtime, successful snapshot replacement, an in-flight RPC completing on the old snapshot while a new RPC uses the new one, superseded idle-close behavior, serialized concurrent Configure calls, and blocked server `Recv` returning on deadline/cancellation.

**Verify:** `go test -race ./internal/spire`.

### 4.2 Replace the template executable cleanly

**Files:** new `cmd/incus-agent/doc.go`, `cmd/incus-agent/main.go`, `cmd/incus-server/doc.go`, `cmd/incus-server/main.go`; delete `cmd/template-go/`, `internal/cli/`, `internal/templateinfo/`, and the old starter contents/tests in `internal/config/config.go`

- Each `main` constructs its corresponding plugin and calls `pluginmain.Serve` with exactly one NodeAttestor plugin server and one Config v1 service server.
- Do not retain Cobra/Viper, flags, version output, compatibility commands, aliases, or template build metadata. Remove their module dependencies with `go mod tidy`.
- Ensure every current internal import uses `github.com/componere/incus-spire-attestor`; no old path or re-export remains.

**Verify:** `go test ./...`, `go build ./cmd/incus-agent ./cmd/incus-server`, and `go mod tidy` followed by a no-diff check for `go.mod`/`go.sum`.

## Phase 5 — Build, package, release, and security-scan both plugins

This phase depends on the final command paths. Steps 5.1–5.3 may proceed in parallel, then verify together.

### 5.1 Moon and mise cutover

**Files:** `moon.yml`, `mise.toml`, `mise.lock`, `.gitignore`

- Rename Moon project metadata to `incus-spire-attestor`.
- Replace the single `build` output with builds for both `cmd/incus-agent` and `cmd/incus-server` for Linux `amd64` and `arm64`, with deterministic output directories such as `bin/linux_amd64/` and `bin/linux_arm64/`. Keep `check` dependent on formatting, lint, tests, both-architecture builds, and docs.
- Remove the template-only `image-local` mise task, Melange/apko tool pins, and their generated-artifact ignore rules. Retain the new mockery pin and regenerate `mise.lock` rather than editing it.

**Verify:** `moon run root:build` produces four executable files, and `file bin/linux_*/*` reports the expected Linux architecture for both names.

### 5.2 GoReleaser and native package cutover

**Files:** `.goreleaser.yaml`, `release-please-config.json`, `.release-please-manifest.json`, `CHANGELOG.md`; delete `melange.yaml` and `apko.yaml`

- Set the project/repository metadata to `componere/incus-spire-attestor`.
- Define exactly two GoReleaser builds, `incus-agent` and `incus-server`, each from its command path and restricted to Linux `amd64`/`arm64` with the existing reproducibility flags. Remove obsolete main-package ldflags, macOS notarization, Windows/Darwin outputs, Homebrew, and Scoop metadata.
- Publish checksum-addressable raw binary assets for both plugins and both architectures in `checksums.txt`. Retain SBOM/checksum signing behavior supported by the current release unit.
- Retain the existing nfpm formats (`deb`, `rpm`, `apk`) as the boring packaging choice, but make each package install both `/usr/bin/incus-agent` and `/usr/bin/incus-server`; do not introduce separate package policy or new formats.
- Remove Melange/apko because they exist solely for the template OCI path and production OCI images are excluded from v1.
- Rename the Release Please package. As an implementer’s choice, reset the inherited template release baseline to `0.0.0` and replace the template changelog history with a project heading rather than claiming template releases as attestor releases.

**Verify:** `goreleaser check` and `goreleaser release --snapshot --clean` succeed; inspect `dist/artifacts.json` and `checksums.txt` to prove all four raw binaries exist, and inspect one generated native package to prove it contains both executable paths.

### 5.3 Release and security workflows

**Files:** `.github/workflows/release.yml`, `.github/workflows/security-scan.yml`

- Remove `oci-image` and `oci-publish` from `release.yml`; make GitHub Release publication depend only on the binary release-assets job and pass `require-oci-image: false`. Keep the current full-SHA-pinned reusable workflow revision and minimum permissions.
- Replace the container scan with a Linux architecture matrix (`amd64`, `arm64`). In each matrix leg cross-build both plugin commands, scan the directory containing both binaries with Trivy’s filesystem/library scan, and upload distinct SARIF categories per architecture. Do not build or publish a container.
- Do not enable additional publishers or publication behavior beyond the existing reviewed binary release path.

**Verify:** validate the workflow YAML, run the exact cross-build commands locally, and dispatch `Security Scan` on a branch before enabling a real release; both matrix legs must upload SARIF and report both binaries in their scan input.

## Phase 6 — Remove template-facing metadata and document only the approved product

This phase may run in parallel with Phase 5 after command/config names stabilize.

**Files:** `README.md`, `CONTRIBUTING.md`, `SECURITY.md`, `docs/mkdocs.yml`, `docs/moon.yml`, `docs/pyproject.toml`, regenerated `docs/uv.lock`, `docs/docs/index.md`; new `docs/docs/how-to/deploy.md`, `docs/docs/reference/configuration.md`, `docs/docs/explanation/security-model.md`; `.github/repository-settings.toml`, `.github/scripts/test_configure_github_repo.py`; delete `DELETE_ME.md`

- Rewrite `README.md` as a concise project entry point: the two external plugins and fixed logical name `incus`, Linux architecture support, trust limitation, links to deployment/config/security documentation, and local `moon` commands. Do not claim TPM, IncusOS posture, OCI images, or exclusive residency proof.
- Document the exact approved agent/server HCL, plugin command/checksum placement, trust-domain ownership by SPIRE core config, TLS credential handling, identity/selectors, timeout-margin requirement, and checksum verification. Do not add knobs or examples absent from the architecture.
- Put the trust model and `can_edit` blast radius in the explanation page; keep deployment steps in the how-to and the HCL/selector/wire-facing facts in reference material.
- Rename MkDocs/project metadata and navigation to `componere/incus-spire-attestor`; regenerate `docs/uv.lock` through uv.
- Update contributor commands for both packages. Replace template language in `SECURITY.md` without inventing a support window; retain GitHub private vulnerability reporting.
- Set `.github/repository-settings.toml` `is_template=false` and remove template-specific comments. Rename the repository fixture strings in `test_configure_github_repo.py` so a final placeholder search is clean while preserving test behavior.
- Delete `DELETE_ME.md` only after all onboarding items are addressed.

**Verify:** `uv run --project docs --locked mkdocs build --strict`, the repository-settings Python tests, and a repository-wide search for `template-go`, `TEMPLATE_GO`, `github.com/meigma/template-go`, `cmd/template-go`, Cobra, and Viper return no product/template remnants.

## Phase 7 — Full local verification and release rehearsal

**Files:** no new production files; fix only failures in their owning files.

1. Regenerate mocks and confirm a second generation is clean.
2. Run the focused behavioral suites:
   - `go test ./internal/attest ./internal/wire ./internal/config`
   - `go test -race ./internal/agent/... ./internal/server/...`
   - `go test -race ./internal/incus/guest ./internal/incus/host ./internal/spire`
3. Run `moon run root:check` to prove formatting, lint, all Go tests, both Linux architecture builds, and strict docs build.
4. Run `goreleaser check` and `goreleaser release --snapshot --clean`; verify the four binary checksums and both-binary package contents.
5. Review test logs and generated errors for nonce leakage. Confirm there is no nonce value in diagnostics, and confirm failures retain standard cancellation/deadline causes.
6. Review the final dependency graph with `go mod tidy`; Cobra/Viper, Melange/apko configuration, and template packages must be absent.

The phase is complete only when all commands pass from a clean checkout with the mise-locked toolchain.

## Phase 8 — Sandbox end-to-end verification (last)

**Requires:** live SPIRE Server and Agent processes, deployment of both built plugins, an Incus 7.3 API client certificate/key with accepted `can_edit` scope, and permission to operate the existing `spike` VM through `ssh josh@sandbox01`. This is not a unit-test substitute and must not start until Phase 7 is green.

1. Inventory without mutation:
   - `ssh josh@sandbox01 'incus version && incus info spike'` must confirm Incus 7.3, VM type, project, location, profiles, UUID, and architecture.
   - Inside `spike`, confirm `/dev/incus/sock`, `/1.0`, `/1.0/meta-data`, and `/sys/class/dmi/id/product_uuid` agree. Record identifiers, never nonce values.
2. Build the matching Linux artifacts, copy `incus-server` beside the sandbox SPIRE Server and `incus-agent` into `spike`, calculate SHA-256 for each deployed executable, and configure both external NodeAttestors under the logical name `incus` with their exact `plugin_cmd` and `plugin_checksum`.
3. Configure the server plugin with the live Incus endpoint/TLS files and the narrow allowed project set; configure the agent plugin with the project hint and default poll timeout. Start SPIRE and attest `spike`.
4. Verify through SPIRE’s agent listing/API that the ID is exactly `spiffe://<trust-domain>/spire/agent/incus/<lowercase-volatile.uuid>`, `can_reattest` is true, and selectors exactly match the API snapshot, including configured profiles and selected `user.*` keys. Restart/re-attest and prove the ID is unchanged and selectors refresh from current API truth.
5. VM-only negative case: deploy the same SPIRE Agent/plugin configuration in a disposable Incus container in the allowed project and prove the agent plugin rejects `instance_type=container` before the server mutates any instance. Remove the disposable container after collecting the denial.
6. Mismatched-claim negative case: on a disposable SPIRE Agent service instance in `spike`, use a temporary systemd read-only bind of a wrong UUID file over `/sys/class/dmi/id/product_uuid`; attest and prove the server denies before mutation. Remove the override and restore the normal agent service. Do not change the production plugin’s fixed DMI path or add a test configuration knob.
7. Timeout/cleanup case: in a disposable SPIRE configuration, set an intentionally tiny positive `challenge_response_timeout` so the server times out after setting the key but before accepting a response. Restore the approved operating value afterward. Query only the Incus config **key names** and prove no key beginning `user.spire.attestor.nonce.` remains; do not print config values.
8. Cancellation/reattest smoke: cancel an in-progress disposable attestation, wait through `cleanup_timeout`, and again assert by key name only that the exact attempt key is absent. Then restore normal deadlines and prove a final attestation succeeds.
9. Stop disposable SPIRE processes, remove temporary service overrides/container/test user selectors, and verify `spike` and its Incus config match the captured pre-test state.

**Evidence to retain:** SPIRE server/agent versions, plugin binary SHA-256 values, Incus 7.3 version, successful ID/selectors, re-attestation result, denial statuses for container and mismatched UUID, timeout/cancellation status, and boolean “nonce-prefix key absent” checks. Never retain nonce contents or client private-key material.

## Risks and mitigations

- **Incus credential blast radius:** v7.3 needs `can_edit`, not prefix-scoped mutation. Mitigate with the smallest practical projects/instances, server-only key mounts, and restricted network reachability; fail deployment review if this remains unacceptable.
- **Lost concurrent/operator updates:** a stale full `InstancePut` can overwrite unrelated config. Mitigate with refetch + ETag, a fresh copied config map on every attempt, one exact-key mutation, operation wait, and target revalidation before every retry.
- **Unknown set outcome or canceled RPC:** transport failure may occur after Incus commits. Mitigate by arming cleanup before `SetNonce` and using `WithoutCancel` plus the independent cleanup deadline.
- **Same-instance concurrency:** a fixed key would cross-talk. Mitigate with independent 128-bit attempt IDs and nonces and tests that overlap two attempts.
- **Blocked stream receive:** an unbounded gRPC `Recv` can outlive the challenge. Mitigate with the one handler-local goroutine, deadline/context select, and plugintest cancellation coverage; do not generalize it into a worker system.
- **Runtime reconfiguration races:** mutating shared config/client state can mix generations. Mitigate with build-before-publish, atomic immutable snapshots, entry locking, and closing only superseded idle connections.
- **Wire expansion or nonce leakage:** permissive JSON or error formatting could weaken the contract. Mitigate with bounded staged decoding, exact type/version switches, golden/negative cases, and diagnostics tests that use redacted/non-secret context only.
- **Two-binary release assumptions:** the template reusable workflow was exercised for one application and OCI. Mitigate by inspecting the pinned `meigma/release` input/output contract during implementation, rehearsing with publication disabled, and requiring artifact/checksum inspection before any tag is published.
- **Sandbox mutation:** negative tests can disturb `spike` or leave credentials/config behind. Mitigate by capturing state first, using disposable SPIRE services and a disposable container, restoring each change immediately, and checking key names without exposing values.

## Bounded implementer’s choices

These details are intentionally not product configuration:

- Choose the simplest fixed bounded retry/poll cadence that fits the configured deadlines; keep it internal and test without wall-clock sleeps.
- Use `json.RawMessage` or an equivalent concrete two-stage decoder; do not add a registry.
- Choose the internal fake-`InstanceServer` mechanics needed to model ETags and operations; do not promote it into a production abstraction.
- Count the 32 KiB selector limit as UTF-8 byte lengths of the final selector-value strings.
- Keep one native package containing both cooperating binaries in each existing nfpm format.
- Pin the current compatible mockery release at implementation time and lock its artifacts through mise.

## Open questions

- Can the Incus client certificate be scoped narrowly enough for the `can_edit` blast radius to pass deployment review?
- Do production cluster latency measurements require different values for `incus_timeout`, `challenge_response_timeout`, and `cleanup_timeout` while preserving the server-response/agent-poll margin?
- Does observed crash residue justify an out-of-band maintenance tool after v1? This must not become v1 scope without a new architecture decision.
- Which license should the repository publish? Current release metadata claims `Apache-2.0 OR MIT`, but the template contains no project license; do not publish packages/releases until this is resolved.
- Does `sandbox01` already have a suitably isolated live SPIRE deployment and disposable container capacity, or must the executor provision temporary SPIRE Server/Agent instances before running Phase 8?