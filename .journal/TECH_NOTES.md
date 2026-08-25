# Technical Notes

- Use hexagonal architecture at all times. Keep business logic isolated from CLI, filesystem, network, storage, and other external adapters.
- Prefer functional testing before calling any feature complete. Unit tests are useful, but they do not prove the tool works the way the design intends.
- Take an agile approach to development. Avoid waterfall: underspecify when useful, prototype early, learn from the result, and refine from working behavior.
- v1 design (approved, session 001): SPIRE NodeAttestor plugin pair named `incus`; guest claims + server-side Incus API cross-check + `user.spire.attestor.nonce.<hex>` config-nonce challenge. Identity/selectors come only from the Incus API snapshot; agent ID suffix = lowercase `volatile.uuid`. See `.journal/001/ARCHITECTURE.md` and `PLAN.md`.
- Incus guest API (`/dev/incus/sock`) is read-only for config; host-set `user.*` keys are instantly guest-visible (the residency-proof channel). SMBIOS `product_uuid` == `volatile.uuid`.
- Incus runs swtpm without EK certificates — vTPM evidence needs enrollment infra (deferred). IncusOS has a TPM-resident signing key (Security API `tpm_public_key`) usable for a future v2 signed-instance-doc evidence type.
- Sandbox: `ssh josh@sandbox01` (Incus 7.3). Guest apt egress is blocked; work host-side. `spike` test VM may exist from session 001.
- Unresolved before first release: repo LICENSE (template ships none; release metadata claims `Apache-2.0 OR MIT`).
