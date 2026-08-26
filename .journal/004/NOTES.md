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
