---
title: Configuration
description: HCL fields, identity, and selectors for the Incus NodeAttestor plugins.
---

# Configuration

v1 is two external SPIRE NodeAttestor plugins with the fixed logical name
`incus`. `incus-agent` implements `agent/nodeattestor/v1`. `incus-server`
implements `server/nodeattestor/v1`.

SPIRE supplies `plugin_cmd`, `plugin_checksum`, and
`CoreConfiguration.trust_domain` outside `plugin_data`. The trust domain
is not an agent or server plugin attribute. Unknown `plugin_data`
attributes are rejected.

Duration attributes are Go duration strings such as `5s` and `10s`.
Omitted optional durations take the defaults below. A duration must be
greater than zero. Zero, negative, and unparsable values are invalid.

`Validate` checks HCL and the required core trust domain. It does not
read TLS files or contact Incus. `Configure` also reads the server TLS
files, connects to Incus, and reads `/1.0` once to cache the standalone
server name. The server does not read `/1.0` again during attestation;
the agent reads guest `/1.0` on every attempt.

## Trust domain

`trust_domain` comes only from SPIRE `CoreConfiguration`, set in the
SPIRE agent and server core blocks. Both plugins require it to be
nonempty. A `trust_domain` attribute inside `plugin_data` is rejected
as unknown.

The server uses that value when it constructs the agent ID.

## Agent `plugin_data`

| Attribute | Required | Default | Bounds |
| --- | --- | --- | --- |
| `project` | no | empty | optional guest project hint; may be empty |
| `poll_timeout` | no | `5s` | positive duration |

When `project` is set, it locates the instance and must appear in the
server `projects` allowlist. It never becomes the agent ID or a selector
by itself. When `project` is omitted, the server searches every allowed
project and requires exactly one matching instance. Two matches are
denied as ambiguous.

## Server `plugin_data`

| Attribute | Required | Default | Bounds |
| --- | --- | --- | --- |
| `incus_endpoint` | yes | none | nonempty Incus API URL |
| `tls_ca_path` | yes | none | nonempty path to the Incus CA certificate |
| `tls_cert_path` | yes | none | nonempty path to the Incus client certificate |
| `tls_key_path` | yes | none | nonempty path to the Incus client key |
| `projects` | yes | none | 1–32 distinct nonempty project names |
| `user_selectors` | no | empty | at most 32 distinct nonempty `user.*` keys |
| `incus_timeout` | no | `5s` | positive duration |
| `challenge_response_timeout` | no | `10s` | positive duration |
| `cleanup_timeout` | no | `5s` | positive duration |

TLS paths belong only with `incus-server`.

`projects` is an allowlist. Duplicate or empty entries are invalid.

`user_selectors` entries must start with `user.` and must include a
suffix (`user.` alone is invalid). `user.spire.attestor.nonce` and any
key that starts with `user.spire.attestor.nonce.` are reserved and
rejected. Duplicate keys are invalid.

`incus_timeout` bounds each Incus API lookup and the nonce write.
`challenge_response_timeout` bounds the wait for the agent nonce
response. `cleanup_timeout` bounds removal of the nonce key after each
attempt.

Keep `challenge_response_timeout` greater than the agent
`poll_timeout`. The defaults already have that margin.

## Identity and selectors

The attested instance type is `virtual-machine` only. Containers are
denied.

Identity and selectors come only from the Incus API snapshot resolved
during attestation. Guest claims locate a candidate; they do not supply
these values.

On a clustered server, `location` is the instance cluster member from
that snapshot. On a standalone server, Incus reports instance location as
`none`. The server plugin reads `environment.server_name` from Incus
`/1.0` once during `Configure` and uses that name when the instance
record is `none`. Guest `/1.0` location is never copied into the
selector.

Agent ID:

```text
spiffe://<trust-domain>/spire/agent/incus/<lowercase-volatile.uuid>
```

`can_reattest` is always `true`.

Selector type is `incus`. Values are sorted and deduplicated:

```text
project:<project>
name:<instance-name>
location:<cluster-member>
uuid:<lowercase-volatile.uuid>
profile:<profile>
user.<suffix>:<value>
```

There is one `profile:` value per profile. Each configured
`user_selectors` key that exists in the API `expanded_config` becomes
`user.<suffix>:<value>` using that API value. A configured key that is
absent from `expanded_config` emits no selector.

More than 100 selectors, or more than 32 KiB of aggregate UTF-8
selector-value bytes, is rejected rather than truncated.

## Agent example

```hcl
plugin_data {
  project      = "default"
  poll_timeout = "5s"
}
```

An empty `plugin_data` block is valid and uses an empty project hint
with `poll_timeout = 5s`.

## Server example

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
