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

## 2026-08-25 23:15 — Attestor PR #21 merged
Squash-merged as 1be1406; master fast-forwarded; feat/oci-image worktree and
branches removed. Release pipeline now: reusable unit 0.1.18, tar.gz archives,
signed native packages, carrier image, require-oci-image=true.
Release Please PR #20 (chore(master): release 1.0.0) is open and now includes
the docs overhaul + release adoption. Publishing v1.0.0 is a user decision
(session 002 precedent: PR #18 closed unpublished) — awaiting explicit call.

## 2026-08-26 09:25 — v0.1.0 versioning fix; GitHub Actions outage
User wants first release v0.1.0, not v1.0.0. Root cause: attestor
release-please-config.json lacks "initial-version" (incusos-builder sets
0.1.0); manifest 0.0.0 counts as no prior release -> default 1.0.0.
Fix: PR #22 (fix/initial-version, adds "initial-version": "0.1.0").
Blocker: GitHub partial system outage (status indicator=major) — Actions runs
are not being scheduled, so the required ci check never reports and the
ruleset blocks merge. Auto-merge armed on #22; background monitor polls
githubstatus and force-push-retriggers CI on recovery. After merge, Release
Please must regenerate PR #20 as "release 0.1.0" (also Actions-dependent).

## 2026-08-26 09:50 — initial-version fix merged; release PR now 0.1.0
PR #22 merged (f49cd5c) adding "initial-version": "0.1.0" to
release-please-config.json. GitHub outage friction: pull_request webhooks
lagged; recovered runs attached to a pre-amend SHA; resolved by resetting the
branch to the already-validated identical-tree SHA (7a752eb) so the required
ci check applied — no admin bypass. Post-merge Release Please webhook also
lagged; manually dispatched (workflow_dispatch). PR #20 regenerated as
"chore(master): release 0.1.0". Awaiting user decision to publish v0.1.0.

## 2026-08-26 10:35 — v0.1.0 released and verified end-to-end
PR #20 merged (ce190ee) after outage-induced retriggers (close/reopen to get
ci onto regenerated head 9361979). Tag v0.1.0 created by Release Please.
First Release run FAILED at staging: nfpm APK signer rejects PKCS#8
"ENCRYPTED PRIVATE KEY"; my openssl genrsa -aes256 key was PKCS#8. Fix:
regenerated APK key as traditional PKCS#1 ("RSA PRIVATE KEY", Proc-Type
4,ENCRYPTED via openssl rsa -traditional -aes256), updated both APK secrets,
gh run rerun --failed -> all four jobs green.
Verification battery (all pass):
- Release v0.1.0 published: 2 tar.gz, 6 native packages (+SBOMs), per-binary
  SBOMs, checksums.txt + sigstore bundle.
- Image binaries byte-identical to release-archive binaries (arm64 checked);
  archive hash matches checksums.txt; entrypoint /usr/bin/incus-server,
  user 65532, amd64+arm64.
- cosign verify-blob (checksums bundle): Verified OK. cosign keyless verify
  (image): verified.
- gh attestation verify (image + tar.gz): pass with
  --signer-repo meigma/release (cross-org reusable signer; incusos-builder
  behaves identically; without --signer-repo it fails by policy design).
- APK contains .SIGN.RSA.incus-spire-attestor-001.rsa.pub (signed, expected
  key name). Image tags 0.1.0/0.1/0/latest -> same digest; :1 absent.
Follow-up landed: PR #23 pins deploy.md init-container example to :0.1.0
(auto-merge armed; GitHub incident still delaying checks).
Upstream doc gap noted: meigma/release adopt guide should state the APK key
must be traditional PKCS#1 PEM, not PKCS#8 — candidate upstream docs PR.

## 2026-08-26 10:45 — Session wrap-up items landed
Attestor PR #23 merged (4630b06): deploy.md example pinned to :0.1.0 (tag :1
does not exist pre-1.0; live tags 0.1.0/0.1/0/latest confirmed same digest).
meigma/release PR #71 merged: adopt guide now states the APK signing key must
be traditional PKCS#1 PEM (with openssl -traditional recipe) — the exact
failure v0.1.0's first run hit. All implementation worktrees removed in both
repos; masters/mains fast-forwarded. GitHub incident (partial outage) was
active throughout; caused webhook lag only, no data issues.
v0.1.0 is live and fully verified. Remaining open thread: incusos-builder
application->incusos-builder melange rename at its next unit re-pin.

## 2026-08-26 10:50 — incusos-builder migration tracked
Filed componere/incusos-builder#42: rename melange staged binary
application -> incusos-builder when re-pinning the release unit to >=0.1.18.
Open thread closed on this side.

## 2026-08-26 10:27 — Close
Session closed. All work merged: attestor PRs #19, #21, #22, #23, release PR
#20 (tag v0.1.0, ce190ee); meigma/release PRs #69, #70 (unit 0.1.18), #71.
v0.1.0 fully verified (see 10:35 entry). incusos-builder#42 tracks the
melange rename at re-pin. SUMMARY.md written; INDEX row complete; TECH_NOTES
refreshed (release unit, signing keys, v0.1.0, restricted-cert docs status).
Handoff: repo is released and documented; next candidates are the
componere/pkgs receiver leg and the MkDocs Material migration.
