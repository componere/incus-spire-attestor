# Technical Notes

- Use hexagonal architecture at all times. Keep business logic isolated from CLI, filesystem, network, storage, and other external adapters.
- Prefer functional testing before calling any feature complete. Unit tests are useful, but they do not prove the tool works the way the design intends.
- Take an agile approach to development. Avoid waterfall: underspecify when useful, prototype early, learn from the result, and refine from working behavior.
- v1 design (approved, session 001): SPIRE NodeAttestor plugin pair named `incus`; guest claims + server-side Incus API cross-check + `user.spire.attestor.nonce.<hex>` config-nonce challenge. Identity/selectors come only from the Incus API snapshot; agent ID suffix = lowercase `volatile.uuid`. See `.journal/001/ARCHITECTURE.md` and `PLAN.md`.
- Incus guest API (`/dev/incus/sock`) is read-only for config; host-set `user.*` keys are instantly guest-visible (the residency-proof channel). SMBIOS `product_uuid` == `volatile.uuid`.
- Incus runs swtpm without EK certificates — vTPM evidence needs enrollment infra (deferred). IncusOS has a TPM-resident signing key (Security API `tpm_public_key`) usable for a future v2 signed-instance-doc evidence type.
- Sandbox: `ssh josh@sandbox01` (Incus 7.3). Guest apt egress is blocked; work host-side. Approved steady state is one running `default/spike` VM with profile `default`, `user.spire.test=hello`, and `root`, `eth0`, and `vtpm` devices.
- The repository is dual-licensed under Apache-2.0 or MIT. Native package metadata uses `Apache-2.0 OR MIT`.
- v1 is implemented and live-verified with SPIRE 1.15.0 and Incus 7.3. Package boundaries are `internal/attest`, `wire`, `config`, `agent`, `server`, `incus/guest`, `incus/host`, and `spire`; binaries are `incus-agent` and `incus-server`.
- Incus v7.3 `ProtocolIncus.WithContext` mutates its receiver. Host operations must call `UseProject` to clone before applying a request context.
- Standalone host records report location `none`, while the guest reports the Incus server name. The host adapter caches `/1.0` `environment.server_name` at Configure time and substitutes it only for standalone records; clustered record locations remain authoritative.
- Releases contain four static Linux binaries plus native APK, DEB, and RPM packages for amd64/arm64, SBOMs, checksums, and a checksum Sigstore bundle. The inherited OCI image path was removed.
- Release workflows consume `vars.COMPONERE_RELEASE_APP_CLIENT_ID` and `secrets.COMPONERE_RELEASE_APP_PRIVATE_KEY`. General repository settings and the `Default branch` ruleset are applied; the `Default tags` ruleset remains unapplied because GitHub rejects the release-app bypass actor.
- No production release exists yet. Generated v1.0.0 release PR #18 was closed by user decision; Release Please can recreate a release PR after future changes.
- Material for MkDocs 9.x reaches end of life on 2026-11-05; plan a documentation toolchain migration.
