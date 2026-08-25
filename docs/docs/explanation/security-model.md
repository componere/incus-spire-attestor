---
title: Security model
description: What the Incus NodeAttestor proves and what remains outside v1.
---

# Security model

v1 answers a narrow question: did a responder present guest claims that
match one allowed Incus virtual machine at attestation time, and could
it read a fresh nonce written through the Incus configuration channel?
It does not prove exclusive guest residency, guest OS integrity, or
workload identity.

## Guest claims locate; the API decides

The agent reads instance type, location, name, cloud-init ID, and DMI
product UUID from the guest, and may add an optional configured project
hint. Those claims are a locator. They never become the agent ID or the
selector set.

The server looks up Incus through the official API, using the configured
project allowlist. It accepts the attempt only when the API record is a
`virtual-machine` and the name, canonical `volatile.uuid`, location,
cloud-init instance ID, and type all match the claims. When the guest
supplies a project hint, that project must be allowed and must equal the
API project. Without a hint, the server searches every allowed project
and requires exactly one matching instance. The identity string and
every selector are built from that API snapshot.
Re-attestation repeats the lookup, so selectors follow current Incus
state rather than the first observation.

## The nonce challenge

After the records match, the server stores a 128-bit random nonce under
a new key `user.spire.attestor.nonce.<32 lowercase hex digits>`. The
suffix is an independent 128-bit attempt ID. The nonce is never sent in
the challenge and is not logged. The agent returns it once as the
challenge response, where the server compares it as equal-length bytes
in constant time.

The agent accepts only that exact key grammar, then reads the value
through `/dev/incus/sock`. Host-set `user.*` keys are visible on the
guest configuration channel, which is why the nonce can move from Incus
to the agent without a second control plane.

The nonce is single-use. A later attempt uses a different key and a
different expected value, so a leftover key cannot satisfy a new
challenge.

## Same VM, not exclusive residency

A successful response shows that some principal could read a fresh
value that Incus had just written for that instance. The reader might be
the intended guest. It might also be guest root, another guest principal
with `/dev/incus/sock`, or any Incus principal that can view that
instance’s configuration.

v1 therefore does not prove that the SPIRE Agent is the only resident of
the VM, or that only the guest could have answered. Every principal with
`can_view` on allowed instance configuration is in the trusted computing
base.

## Virtual machines only

The plugins attest `virtual-machine` instances only. A container is
denied before the server writes a nonce. That boundary is a type check
against guest metadata and the Incus API record, not a general Incus
workload attestor.

## Credentials and the Incus client

The server plugin’s TLS CA, client certificate, and client key are part
of the trusted computing base. They exist only beside `incus-server`.
Anyone who can use that client identity can look up allowed instances
and participate in the same configuration channel the challenge uses.

Compromise of Incus, SPIRE, either plugin, or those files can satisfy or
subvert attestation.

## The Incus 7.3 `can_edit` blast radius

Incus 7.3 requires `can_edit` for the instance update that writes the
nonce. That entitlement is not limited to the `user.spire.attestor.nonce.`
prefix. The same identity can also modify, rename, or delete authorized
instances.

Limit the client to the smallest practical projects and instances, keep
the certificate and key only on the server plugin host, and restrict its
network path to the Incus API. If that blast radius is unacceptable,
do not deploy v1. There is no v1 configuration that narrows mutation to
the nonce prefix.

## Cleanup and leftover keys

The server unsets the exact attempt key after each attempt, including
cancellation and an unknown write outcome. Cleanup uses its own deadline
so RPC cancellation cannot skip the unset.

A process crash can still leave a key behind. Residual keys are inert:
they do not contain a nonce that a later attempt will accept, because
each attempt chooses a new suffix and a new nonce. v1 does not scan for
or scavenge residue. Whether leftover keys later justify an out-of-band
maintenance tool is outside v1.

## What v1 does not do

v1 does not include TPM-signed documents, EK-certificate validation,
IncusOS Security API policy or selectors, AgentStore or TOFU, custom
metrics, automatic project discovery, a nonce-residue controller,
production OCI images, or an automated IncusOS lifecycle harness.

It also does not establish secure boot state, guest OS integrity, TPM
state, exclusive control of the guest socket, or identity for workloads
running inside the VM. First observation adds no evidence beyond the
current Incus record, which is why v1 sets `can_reattest = true` and
does not persist a TOFU store.
