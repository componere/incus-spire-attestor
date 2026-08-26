---
id: 004
title: Docs overhaul and v0.1.0 release
date: 2026-08-25
status: complete
repos_touched: [incus-spire-attestor, meigma/release, incusos-builder]
related_sessions: [001, 002, 003]
---

## Goal
Started open-ended; grew into two arcs: (1) a full docs review/overhaul
(Diátaxis alignment, factual verification against code, language-style pass,
README rewrite), and (2) shipping the first production release with binaries
plus a container image, which required upstream multi-binary support in the
meigma/release reusable release unit.

## Outcome
Both arcs complete. Docs: a conformance agent checked every falsifiable claim
in the three site pages against `internal/`/`cmd/` — zero wrong claims; two
loosely scoped statements fixed, the ETag-guarded nonce write and the
restricted-certificate deployment pattern documented, README rewritten to
lead with purpose (PRs #19, #23). Release: meigma/release gained multi-binary
staging/build/verify (its PR #69, unit 0.1.18 at `09d6be7`), this repo adopted
the four-job pinned pipeline with signed native packages and a carrier image
(PRs #21, #22), and **v0.1.0 is released and verified end-to-end**: image
binaries byte-identical to release-archive binaries, cosign keyless image
signature and checksums bundle verified, GitHub provenance attestations
verified (with `--signer-repo meigma/release`), APK signed with the expected
key, tags `0.1.0`/`0.1`/`0`/`latest` on one digest.

## Key Decisions
- Release surface: binaries + native packages + one multi-binary **carrier**
  image (init-container copy source; not runnable — SPIRE execs the plugins);
  no brew/scoop (no workstation install surface).
- Chose upstreaming multi-binary support over a repo-local OCI leg or
  deferral -> keeps the org on one reviewed release unit; contract fixed
  up front (schemas v2, staging by GoReleaser binary name, name-set equality
  across arches, entrypoint must name one staged binary).
- Rehearsed the new unit locally (built release-cli from the PR branch, ran
  `image build`/`image verify` against real attestor binaries and configs)
  before merging anything — validated upstream and adoption together.
- First release v0.1.0, not v1.0.0 -> added `initial-version: 0.1.0` to
  release-please config (matches incusos-builder).
- Native package signing keys are repo-level Actions secrets (documented
  producer pattern). The APK key must be traditional PKCS#1 PEM — PKCS#8
  broke the first release run; regenerated and documented upstream (#71).
- Docs deploy example pins the image to `0.1.0`; `:1` does not exist pre-1.0.

## Changes
- `docs/docs/{how-to/deploy,reference/configuration,explanation/security-model}.md`,
  `README.md` — verified/corrected docs, restricted-cert pattern, ETag guard,
  purpose-first README (#19, #23).
- `.goreleaser.yaml`, `melange.yaml`, `apko.yaml`, `mise.toml`, `mise.lock`,
  `.github/workflows/release.yml`, `release-please-config.json` — release
  adoption, carrier image, unit pin 0.1.18, signing, initial-version (#21, #22).
- `meigma/release` — multi-binary SelectBinaries/projection/build/verify
  (schemas v2), by-name staging, workflow/docs/example updates (#69), APK
  key-format docs (#71); released as 0.1.18.
- Repo secrets: `RPM_SIGNING_KEY/PASSPHRASE` (OpenPGP RSA-4096, fingerprint
  `62669F25…4625C77D`), `APK_SIGNING_KEY/PASSPHRASE` (PKCS#1 RSA-4096).
- Release v0.1.0 published: tag `v0.1.0` (`ce190ee`), GitHub release assets,
  `ghcr.io/componere/incus-spire-attestor` image digest `6c0a075a…ca2d31`.

## Open Threads
- incusos-builder must rename `application` -> `incusos-builder` in its
  melange.yaml when re-pinning the unit to >=0.1.18 — tracked as
  incusos-builder#42.
- This repo does not yet request componere/pkgs package-repository
  publication (incusos-builder does); add the receiver leg if apt/rpm/apk
  repo distribution is wanted.
- MkDocs Material 9.x EOL 2026-11-05 (unchanged from session 002).

## Lessons
- `gh attestation verify` fails by policy for cross-org reusable signers
  unless `--signer-repo meigma/release` is passed; this is expected, not a
  defect (incusos-builder identical).
- OpenSSL 3 `genrsa -aes256` emits PKCS#8 (`ENCRYPTED PRIVATE KEY`), which
  nFPM's APK signer rejects; use `openssl rsa -traditional -aes256` for
  PKCS#1.
- A GitHub partial outage manifests as missing/late workflow runs on new
  SHAs; re-pointing a branch at an already-validated identical-tree SHA
  satisfies required checks without an admin bypass.

## References
- PRs: [#19](https://github.com/componere/incus-spire-attestor/pull/19),
  [#21](https://github.com/componere/incus-spire-attestor/pull/21),
  [#22](https://github.com/componere/incus-spire-attestor/pull/22),
  [#23](https://github.com/componere/incus-spire-attestor/pull/23),
  release PR [#20](https://github.com/componere/incus-spire-attestor/pull/20);
  [meigma/release#69](https://github.com/meigma/release/pull/69),
  [meigma/release#70](https://github.com/meigma/release/pull/70),
  [meigma/release#71](https://github.com/meigma/release/pull/71);
  [incusos-builder#42](https://github.com/componere/incusos-builder/issues/42)
- Release: https://github.com/componere/incus-spire-attestor/releases/tag/v0.1.0
- `.journal/004/NOTES.md` — full session log incl. verification battery
- `.journal/003/RESULTS.md` — restricted-cert evidence the docs work cites
