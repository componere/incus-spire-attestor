---
id: 004
title: New session
started: 2026-08-25
---

## 2026-08-25 20:42 — Kickoff
Goal for the session: not yet stated; user requested a fresh session and will
provide the actual task next.
Current state of the world: v1 attestor implemented, merged to master
(`80dd5c3`), and functionally verified end-to-end on sandbox01 and the glab
cluster (session 003, zero product defects). No production release exists;
Release Please PR #18 was closed by user decision. Open threads: document the
restricted-certificate deployment pattern, sandbox01 cannot resolve `glab.lol`,
MkDocs Material 9.x EOL 2026-11-05, tags ruleset unapplied.
Plan: await the user's request.

## 2026-08-25 21:20 — Docs review/overhaul complete, PR #19 open
Goal became: docs review/overhaul (diataxis alignment, fewer/richer docs, no
wrong claims, language-style adherence).
Done: conformance agent verified every falsifiable claim in the three docs
pages against internal//cmd/ — zero WRONG verdicts. Fixed two loosely scoped
statements (server-vs-agent /1.0 reads; incus_timeout vs cleanup_timeout
scope). Added: source-build install path (no release exists), restricted TLS
certificate deployment pattern (grounded in Incus authorization docs:
`incus config trust edit` → restricted:true + projects; evidence from 003
leg C), and the previously undocumented ETag-guarded nonce write. Style pass
on deploy.md (double-negation intro, misplaced prereq rules, macOS shasum
aside removed). Structure kept: 4 pages, one per needed diataxis quadrant.
README/CONTRIBUTING/SECURITY reviewed — current, no changes.
Verified: `moon run docs:build` (strict) passes in worktree .wt/docs-overhaul.
PR: https://github.com/componere/incus-spire-attestor/pull/19 (branch
docs/overhaul, commit b8a56cd). Next: await review/merge; then remove
worktree. This closes session 003's open thread on documenting the
restricted-cert pattern.

## 2026-08-25 21:35 — README rewritten into PR #19
User asked for a README pass in the same PR: too bare, intro led with plugin
interface trivia. Rewrote it to lead with purpose (node attestation for Incus
VMs lacking cloud metadata / provisioned TPM identity), then the challenge
flow, agent ID + selector example, migration behavior (C3 evidence), v1
guarantee boundary, source-build getting-started, and existing dev/license
sections. Dropped interface names from the intro (reference doc covers them).
Commit 8c452f3 pushed; PR #19 body updated.

## 2026-08-25 21:45 — PR #19 merged, cleaned up
Squash-merged as c0d26d3 (checks green: ci, GitHub Pages). Local master
fast-forwarded; docs/overhaul worktree, local branch, and remote branch
removed. Release Please refreshed its release branch after the merge; no
release published (unchanged posture from session 002). Docs overhaul is
complete: three site pages verified/corrected, restricted-cert pattern
documented, README rewritten. Session goal fulfilled.

## 2026-08-25 22:10 — Release scope decided; upstream multi-binary work started
User wants releases = binaries + containers, no brew/scoop (agreed: Linux-only
plugins, no workstation install surface). Research (agent ReleaseCliContract,
evidence from meigma/release@0dee66f source): the pinned release unit is
strictly single-application/single-binary — release-cli stage fails on a
duplicate linux/<arch> Binary record, oci-build-inputs v1 requires exactly two
entries with one shared name, image build stages one file as
sources/<arch>/application, verifier hardcodes one entrypoint + one layer
entry. So this repo can adopt neither go-pre-publish nor the OCI leg today
(explains session 002's bespoke staging job). Also: go-pre-publish asset
upload has no raw-binary glob → adoption requires tar.gz archives.
User chose: upstream multi-binary support in meigma/release (over a repo-local
OCI leg or deferring). Container shape decided: carrier image
ghcr.io/componere/incus-spire-attestor with both plugins under /usr/bin,
init-container copy pattern for containerized spire-server; entrypoint rule
upstream = must name one staged binary.
Started: worktree meigma/release/.wt/feat-multi-binary-images; contract fixed
(schemas bump to v2: oci-build-inputs, image-build, verify results; staging by
real binary name; per-(arch,name) canonical digests; name-set equality across
arches; one-pass multi-entry layer hashing). Two programmer agents running:
MultiBinaryCore (Go + tests), MultiBinaryDocsFlow (workflows/docs/self-release
melange + examples). incusos-builder migration note: its melange.yaml must
rename application → incusos-builder when it re-pins.

## 2026-08-25 22:35 — Upstream implemented, rehearsed, PR open; attestor branch staged
meigma/release PR #69 (feat/multi-binary-images, https://github.com/meigma/release/pull/69):
SelectBinaries keeps every linux Binary record with per-(arch,name) dedup and
name-set equality; schemas bumped (oci-build-inputs/v2, image-build/v2,
image-verify/v2); staging by real binary name (application sentinel removed,
incl. repo self-release melange.yaml -> release-cli); verify derives names
from work/sources, hashes all usr/bin/<name> entries in one tar pass,
entrypoint must name one staged binary; go-oci-build.yml no longer extracts
.result.binary; docs/example/tutorial updated with migration note. Full
moon root:check green; CI + Nix flake + Kusari green on the PR.
Functional rehearsal (local, macOS + Docker): built attestor binaries via
moon, hand-wrote a v2 projection, ran the new release-cli image build with
the attestor melange.yaml/apko.yaml (melange 0.59.1, apko 1.2.37 from mise)
then image verify — both passed; per-binary layer digests == canonical
digests on both platforms; image-digest.txt written. This validates upstream
AND the attestor configs together, pre-merge.
Attestor branch feat/oci-image pushed (92c504f): tar.gz archives, melange/
apko carrier configs, mise pins+lock for melange/apko, four-job release.yml
with MULTIBINARY_RELEASE_UNIT_SHA placeholder, deploy.md/README image docs.
sign-native-packages=false (repo has no RPM/APK signing secrets; incusos-
builder holds repo-level keys — decide separately).
Blocked on user: merge meigma/release #69, then its Release Please release
PR; pin that release commit SHA in attestor release.yml; open attestor PR.
Note: incusos-builder must rename application->incusos-builder in its
melange.yaml when it re-pins the unit.

## 2026-08-25 23:05 — Upstream released, secrets set, attestor PR #21 open
meigma/release: #69 squash-merged (1fd191d); Release Please PR #70 merged ->
release commit 09d6be776ca4a9d533101bae1fbc790fb42050a8 = unit 0.1.18; its own
v0.1.18 tag Release run succeeded end-to-end on the new code (all jobs green;
non-fatal artifact-metadata warnings observed). GitHub release v0.1.18 exists.
Signing: generated producer keys and set four repo-level secrets on
componere/incus-spire-attestor (RPM_SIGNING_KEY = armored OpenPGP RSA-4096,
fingerprint 62669F2556A504495335054F67C7CFAC4625C77D, uid
incus-spire-attestor-rpm-001; APK_SIGNING_KEY = AES-256-encrypted RSA-4096
PEM; both passphrase secrets set). Private material never printed; temp dir
shredded; publics derivable from stored privates.
Attestor PR #21 (feat/oci-image, b632465): pinned 0.1.18 everywhere,
sign-native-packages=true with secret mapping, carrier melange/apko, tar.gz
archives, mise melange/apko pins, docs. CI green. Awaiting user review/merge.
After merge: Release Please will open the attestor release PR; first tagged
release is the full-pipeline proof. Follow-up unchanged: incusos-builder
melange.yaml application->incusos-builder rename when it re-pins.
