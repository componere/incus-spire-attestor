# incus-spire-attestor v1 architecture

## Scope and invariants

v1 consists of two external SPIRE plugins with the shared logical name `incus`:

- `incus-agent` implements `agent/nodeattestor/v1` and Config v1 inside an Incus VM.
- `incus-server` implements `server/nodeattestor/v1` and Config v1 beside the SPIRE server.

Both binaries use `pluginmain.Serve`. SPIRE launches them with `plugin_cmd` and verifies each executable with `plugin_checksum`. The common plugin name is fixed because SPIRE uses it as the selector type and agent-ID namespace.

The implementation must preserve these invariants:

- Payload claims locate an instance but never determine identity or selectors. The server derives both only from an Incus API record.
- The guest and API records must describe the same allowed `virtual-machine`; containers are denied before any mutation.
- Each attempt uses an independent 128-bit random key suffix and 128-bit random nonce. The nonce is single-use, is never sent in the challenge or logged, and is compared as equal-length bytes in constant time.
- The agent accepts only the exact v1 challenge-key grammar before constructing a URL.
- Host mutations use ETag-protected copy-on-write updates so concurrent attempts and operator changes do not overwrite one another.
- The server unsets the exact attempt key on every reachable path after a set might have occurred, including cancellation and an unknown set outcome.
- Opaque messages are versioned, strictly decoded, and bounded.

v1 excludes TPM, EK-certificate, and IncusOS posture evidence. Those features depend on evidence or API reachability not established by the spike.

## Components and boundaries

```text
cmd/
  incus-agent/             agent composition root
  incus-server/            server composition root
internal/
  attest/                  domain types and pure attestation rules
  wire/                    bounded v1 JSON codecs
  agent/                   agent application flow and consumer-owned ports
    mocks/                 mockery-generated port mocks
  server/                  server application flow and consumer-owned ports
    mocks/                 mockery-generated port mocks
  config/                  HCL decoding and pure validation
  incus/
    guest/                 guest socket, metadata, and DMI adapter
    host/                  official Incus client adapter
  spire/                   agent/server SDK services and Config services
```

`internal/attest` owns domain types such as `ProjectName`, `InstanceName`, `InstanceUUID`, `InstanceType`, `ConfigKey`, `Nonce`, `Claims`, `Instance`, and `Attributes`. It performs UUID normalization, claim comparison, VM-only enforcement, nonce comparison, and attribute construction without I/O.

`internal/agent` consumes two narrow ports: `GuestEvidence`, which reads claims and a challenged config value, and `Exchange`, which preserves the payload/challenge/response sequence. `internal/server` consumes an `Incus` port with only lookup, set-nonce, and unset-nonce operations; a server `Exchange`; and an `io.Reader` backed by `crypto/rand.Reader`. Keeping related operations on one dependency port avoids interfaces and packages for individual syscalls while leaving every side effect replaceable in tests.

`internal/incus/host` is a thin adapter over `github.com/lxc/incus/v7/client`, pinned to the Incus 7.3 compatibility line. It uses `ConnectIncusWithContext`, `UseProject`, `GetInstance`, `UpdateInstance`, and operation waits. It owns domain mapping and exact-key mutation, not TLS or generic REST behavior. A mutation reads the current writable record and ETag, copies local `config`, changes only the requested key, and submits `UpdateInstance`. On an ETag conflict it refetches, verifies that project, name, UUID, and VM type still identify the resolved target, reapplies the one-key change, and retries within the stage deadline. Unsetting an absent key succeeds.

The SPIRE adapters translate SDK streams to application exchanges. The server adapter bounds blocking `Recv` with one adapter-local receive goroutine and a select on the challenge deadline and RPC context. Returning from the handler cancels the gRPC stream and releases a blocked receive; this is not a reusable worker subsystem.

`Configure` locks at entry, decodes and validates HCL, loads credentials, and builds a complete immutable runtime before publishing it through an atomic pointer. A failed build leaves the current pointer unchanged; a nil pointer fails closed. Each RPC loads one snapshot. In-flight calls naturally retain an older snapshot after reconfiguration. After a successful swap, close idle connections on the superseded client without terminating active requests.

All packages have `doc.go`, and declarations and fields follow the repository Godoc rules. The only inspectable error classes are invalid configuration, invalid or unsupported wire contract, and attestation denied; operational errors retain wrapped context, and context deadlines retain their standard causes.

The repository cuts over cleanly from `github.com/meigma/template-go` to `github.com/componere/incus-spire-attestor`: remove the template command, Cobra/Viper paths, and metadata. `.goreleaser.yaml`, `moon.yml`, packaging, and `.github/workflows/security-scan.yml` must build and scan both Linux `amd64` and `arm64` executables. Releases publish checksum-addressable binaries for both plugins. Production OCI images are not part of v1.

## Wire contract and evidence seam

SPIRE treats payload and challenge bytes as opaque. v1 uses UTF-8 JSON with a 64 KiB limit per opaque message.

The agent’s first response is:

```json
{
  "version": 1,
  "evidence": [{
    "type": "incus-guest-claims",
    "version": 1,
    "data": {
      "instance_name": "vm-01",
      "project": "default",
      "instance_type": "virtual-machine",
      "uuid": "<canonical UUID>",
      "location": "member-01",
      "cloud_init_id": "<cloud-init instance-id>"
    }
  }]
}
```

`project` is omitted when the guest has no configured project hint; every other field is required. v1 accepts exactly one `incus-guest-claims` item.

The server stores the nonce under a fresh key and sends only that key:

```json
{
  "version": 1,
  "challenge": {
    "type": "incus-config-nonce",
    "version": 1,
    "data": {
      "config_key": "user.spire.attestor.nonce.<32 lowercase hex digits>"
    }
  }
}
```

The suffix encodes an independent 16-byte random attempt ID. The agent requires exact equality to `user.spire.attestor.nonce.` followed by 32 lowercase hexadecimal digits. It rejects extra segments, slashes, queries, percent-encoded ambiguity, and prefix collisions before URL-escaping the key.

The response is:

```json
{
  "version": 1,
  "response": {
    "type": "incus-config-nonce",
    "version": 1,
    "data": { "nonce": "<unpadded base64url>" }
  }
}
```

The nonce is 16 random bytes. The server requires that exact decoded length before constant-time comparison.

Codecs reject invalid UTF-8 or JSON, trailing data, unknown fields, missing or invalid fields, and unsupported envelope, type, or body versions. There is no negotiation or implicit downgrade. `internal/wire` implements direct v1 type/version switches over concrete bodies, using `json.RawMessage` or equivalent for the typed data. A later release can add `tpm-signed-document` or `tpm-ek-certificate` cases and verification rules without changing the SPIRE stream adapters; v1 does not prebuild a handler registry.

## Attestation flow and failures

### Agent

1. Through `/dev/incus/sock`, read `/1.0` and `/1.0/meta-data` for instance type, location, instance name, and cloud-init ID. Add the optional configured project hint.
2. Read and canonicalize `/sys/class/dmi/id/product_uuid`. Reject missing or malformed required claims and any guest type other than `virtual-machine`.
3. Encode and send the payload as the first `AidAttestation` response.
4. For the one v1 challenge, validate the exact key grammar and read `/1.0/config/<escaped-key>`.
5. If the key is not yet visible or the guest transport returns a transient error, poll with an internal bounded backoff until `poll_timeout` or stream cancellation. Return the value without logging it.

The guest socket path, DMI path, poll cadence, and nonce prefix are v1 implementation constants. Constructors may accept alternate paths or timing dependencies for tests.

### Server

1. Decode the initial payload before mutation. If the claim includes a project, require it in the configured allowlist and perform one project-qualified lookup. Otherwise search every allowed project under one total `incus_timeout`; require the complete search to succeed and exactly one API record to match all claims. A not-found result may continue the search, but authentication, authorization, malformed responses, or timeout abort it.
2. Require the API record to be a `virtual-machine`. Compare its project, name, canonical `volatile.uuid`, location, `volatile.cloud-init.instance-id`, and type with the claims. Any mismatch denies attestation before mutation.
3. Generate independent attempt-ID and nonce values with `crypto/rand`. Arm cleanup before calling `SetNonce`, because a transport failure can leave the write outcome unknown. Store the nonce under the per-attempt key with the ETag-safe algorithm, then send the key as the challenge.
4. Wait for exactly one response under `challenge_response_timeout`. Reject timeout, cancellation, stream closure, malformed or wrong-version data, invalid nonce encoding, and nonce mismatch.
5. Unset the exact key before returning. Cleanup uses `context.WithoutCancel(rpcCtx)` plus `cleanup_timeout`, so RPC cancellation cannot suppress the attempt. It retries only transient transport/service errors and ETag conflicts; absence is success. If verification succeeded, cleanup failure prevents attributes. If verification already failed, preserve the primary RPC status and retain the cleanup cause for server diagnostics without nonce material.
6. Build identity and selectors from the authoritative API snapshot resolved in steps 1–2 and return one terminal `AgentAttributes` response.

Normal Incus lookups and mutations retry only transient transport/service failures and ETag conflicts within their enclosing `incus_timeout`. Authentication, authorization, malformed responses, a replaced target, and evidence mismatches are not retryable. No global admission semaphore is added; gRPC cancellation, stage deadlines, and the standard Incus HTTP transport bound work.

A process crash cannot run cleanup. Residual keys are inert because later attempts use different random keys and expected nonces. Maintenance may remove exact keys under `user.spire.attestor.nonce.`; routine attestation does not scan or scavenge them.

## Configuration

SPIRE’s outer plugin configuration supplies `plugin_cmd`, `plugin_checksum`, and `CoreConfiguration.trust_domain`. Trust domain is not duplicated in `plugin_data`.

Agent configuration:

```hcl
plugin_data {
  project      = "default" # Optional; preferred in production.
  poll_timeout = "5s"
}
```

Server configuration:

```hcl
plugin_data {
  incus_endpoint = "https://incus.example.invalid:8443"
  tls_ca_path    = "/run/secrets/incus/ca.pem"
  tls_cert_path  = "/run/secrets/incus/client.pem"
  tls_key_path   = "/run/secrets/incus/client-key.pem"

  projects       = ["default"]
  user_selectors = ["user.environment", "user.role"]

  incus_timeout              = "5s"
  challenge_response_timeout = "10s"
  cleanup_timeout            = "5s"
}
```

The endpoint, TLS files, and one to 32 distinct projects are required. Durations must be positive. `user_selectors` defaults empty, accepts at most 32 distinct `user.*` keys, and rejects the reserved nonce namespace. The server response default exceeds the agent poll default; deployments that tune either value must preserve that margin.

`Validate` checks HCL and required core configuration without reading credentials. `Configure` additionally reads the TLS files and establishes the immutable runtime.

## Identity and selectors

The server constructs the agent ID from the API UUID and SPIRE core trust domain:

```text
spiffe://<trust-domain>/spire/agent/incus/<lowercase volatile.uuid>
```

`can_reattest = true`. The UUID survives VM reboot, and every re-attestation repeats API resolution and a fresh nonce challenge. v1 does not use AgentStore or TOFU because first observation adds no evidence beyond the current Incus record.

Selector type is `incus`. Values come only from the resolved API snapshot and are sorted and deduplicated:

```text
project:<project>
name:<instance-name>
location:<cluster-member>
uuid:<lowercase volatile.uuid>
profile:<profile>                    # one per profile
user.<suffix>:<value>                # selected key from expanded_config
```

Missing configured user keys emit no selector. Reject, rather than truncate, more than 100 selectors or more than 32 KiB of aggregate selector-value bytes. The snapshot defines attributes for that attestation; re-attestation refreshes them.

## Trust, trade-offs, and non-goals

v1 proves that a responder supplied claims matching one allowed Incus VM and could read a fresh value visible through either that VM’s guest configuration channel or an authorized Incus configuration reader. It is not exclusive proof of guest residency. Every principal with `can_view` over allowed instance configuration is therefore in the trusted computing base.

Incus 7.3 requires `can_edit` for the instance update used by the nonce challenge; that entitlement is not restricted to the nonce prefix and can also modify, rename, or delete authorized instances. Limit the client identity to the smallest practical projects and instances, mount its certificate and key only into `incus-server`, and restrict its network path to the Incus API. A deployment that cannot accept this credential blast radius cannot deploy v1.

Guest root, another guest principal with access to `/dev/incus/sock`, an allowed Incus configuration reader, or compromise of Incus, SPIRE, or either plugin can satisfy or subvert the check. v1 does not establish guest OS integrity, secure boot state, TPM state, workload identity, or exclusive control of the guest socket.

Key trade-offs are deliberate:

- JSON keeps the small opaque contract inspectable; Protobuf adds generation without negotiation or stronger transport safety.
- A per-attempt key prevents same-instance attempts from overwriting one another; a fixed key is unsafe.
- The official Incus client owns TLS and REST mechanics; the adapter retains only attestation-specific mapping and safe mutation.
- Attributes use the checked API snapshot. A final re-read would move, not eliminate, the time-of-check/time-of-use window while adding another availability dependency.

Non-goals for v1 are TPM-signed documents, EK-certificate validation, IncusOS Security API policy or selectors, AgentStore/TOFU, custom metrics, automatic project discovery, a nonce-residue controller, production OCI images, and an automated IncusOS lifecycle harness.

### Open deployment questions

- Can the Incus client certificate be scoped narrowly enough for the `can_edit` blast radius to pass deployment review?
- Do production cluster latency measurements require different values for the three exposed deadlines?
- Does observed crash residue justify an out-of-band maintenance tool after v1?

### Residual risks

Incus API unavailability fails attestation closed. Control-plane state can change after the accepted snapshot and remains reflected in selectors until re-attestation. Process termination can leave a non-replayable nonce key. These are accepted v1 availability and operational risks, not additional evidence claims.

## Testing strategy

- **Pure unit tests:** `internal/attest`, `internal/wire`, and `internal/config` prove claim comparison, VM rejection, API-only attributes, key grammar, strict bounds and versions, and configuration validation.
- **Application integration tests:** mockery-generated `agent` and `server` port mocks prove stream ordering, complete project resolution, transient retry, same-instance concurrency behavior, challenge timeout, and cleanup after cancellation or uncertain set outcome.
- **Adapter tests:** Unix-socket fixtures cover guest reads and path escaping. A fake Incus `InstanceServer` covers domain mapping, project scoping, one-key preservation, operation waits, ETag conflict retry, and target replacement. SPIRE SDK `plugintest` covers both services and configuration snapshot replacement.
- **End-to-end tests:** a later sandbox suite runs real SPIRE and Incus 7.3, attests and re-attests a VM, verifies identity and selectors, rejects a container and mismatched claims, and observes timeout and cleanup behavior.