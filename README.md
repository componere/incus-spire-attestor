# incus-spire-attestor

SPIRE node attestation for [Incus](https://linuxcontainers.org/incus/) virtual machines.

SPIRE only issues identities to workloads on nodes it has attested, and its
built-in node attestors lean on platform evidence — cloud provider metadata
or a provisioned TPM identity — that a VM on an Incus or IncusOS cluster
does not present out of the box. This project fills that gap: it proves to
the SPIRE Server that a SPIRE Agent runs inside a specific Incus VM, using
the Incus API as the authority and the instance's own configuration channel
as the proof of residency. It requires no TPM enrollment, no guest secrets,
and no changes to Incus.

The proof is a challenge: the server writes a single-use random nonce into
the instance's `user.*` configuration, and only a reader inside that VM (via
`/dev/incus/sock`) or an Incus principal with access to that instance's
configuration can return it.

## How it works

Two Linux SPIRE NodeAttestor plugins share the logical name `incus`:
`incus-agent` runs beside the SPIRE Agent in the guest, `incus-server` runs
beside the SPIRE Server.

1. The agent plugin reads identity claims from inside the guest: instance
   type, name, DMI product UUID, location, and cloud-init instance ID.
2. The server plugin looks the instance up through the Incus API within an
   allowlisted set of projects and requires an exact match on every claim.
   Containers are denied; only one matching VM may exist.
3. The server writes the nonce to the instance configuration, the guest
   observes it through `/dev/incus/sock` and returns it, and the server
   compares it in constant time, then removes the key.
4. SPIRE records the agent ID
   `spiffe://<trust-domain>/spire/agent/incus/<lowercase-volatile.uuid>`
   and selectors of type `incus`, built from the Incus API record — never
   from guest claims:

   ```text
   project:default
   name:web-01
   location:server02
   uuid:2f6c0f4a-70c1-4b6a-9f16-6b1d6dbe6f21
   profile:default
   user.environment:production
   ```

Re-attestation repeats the API lookup, so selectors follow the live Incus
record: a VM migrated to another cluster member keeps its agent ID while its
`location:` selector moves with it.

## What this proves

A successful attestation proves the guest claims matched one allowed Incus
VM and that the responder could read a fresh nonce delivered through that
instance's configuration channel. It does not prove exclusive guest
residency, guest OS integrity, or workload identity, and it attests virtual
machines only. The [security model](docs/docs/explanation/security-model.md)
states the exact guarantees, the trusted computing base, and how a restricted
Incus certificate bounds the server credential to its allowed projects.

## Getting started

Releases publish per-platform archives, APK/DEB/RPM packages, and a
carrier image (`ghcr.io/componere/incus-spire-attestor`) for
containerized SPIRE Servers. To build from source instead:

```sh
mise install
moon run root:build
```

- [Deploy the plugins](docs/docs/how-to/deploy.md) — install, restrict the
  Incus client identity, configure both NodeAttestors, and verify.
- [Configuration reference](docs/docs/reference/configuration.md) — HCL
  fields, defaults, bounds, identity, and selectors.
- [Security model](docs/docs/explanation/security-model.md) — what v1 proves
  and what it does not.

## Local development

Install [mise](https://mise.jdx.dev), then provision the pinned toolchain:

```sh
mise install
```

Moon is the project task front door:

```sh
moon run root:format
moon run root:lint
moon run root:test
moon run root:build
moon run root:check
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for contribution expectations, local commands, and the pull request process.

## Security

See [SECURITY.md](SECURITY.md) for the current support status and private vulnerability reporting path.

## License

Except where otherwise noted, this repository is licensed under either of the
following licenses, at your option:

- [Apache License, Version 2.0](LICENSE-APACHE)
- [MIT License](LICENSE-MIT)
