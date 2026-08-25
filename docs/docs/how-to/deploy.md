---
title: Deploy
description: Deploy the Incus SPIRE NodeAttestor plugins on Linux.
---

# Deploy the Incus NodeAttestor plugins

Place `incus-agent` and `incus-server` where SPIRE can launch them, record
SHA-256 checksums, restrict Incus credentials to the server plugin, and
configure both NodeAttestors under the logical name `incus`.

This procedure does not publish a release and does not run the plugin
binaries. Obtain matching Linux binaries for your architecture, then
configure SPIRE to launch them.

## Prerequisites

- Linux `amd64` or `arm64` hosts for both plugins. The plugins are not
  documented for other operating systems or architectures.
- A SPIRE Agent process inside an Incus virtual machine that is allowed
  by the server plugin `projects` list. That guest must expose
  `/dev/incus/sock` and `/sys/class/dmi/id/product_uuid` to the agent
  process. The guest cloud-init `local-hostname` must equal the Incus
  instance name, and the instance must have
  `volatile.cloud-init.instance-id` set.
- Network reachability from the SPIRE Server host to the Incus API
  endpoint you will set in `incus_endpoint`.
- An Incus client identity that can look up allowed instances and, on
  Incus 7.3, update instance configuration. If that `can_edit` blast
  radius is unacceptable, stop. See [Security model](../explanation/security-model.md).
- The Incus CA certificate, client certificate, and client key for that
  identity. Mount those files only beside `incus-server`. Do not copy
  them into the guest or onto any other host.

Set `plugin_cmd` and `plugin_checksum` on the outer SPIRE plugin block.
Do not put them in `plugin_data`. Put `trust_domain` only in the SPIRE
agent and server core blocks.

## Place the binaries

Install the `incus-agent` executable on the guest that runs SPIRE Agent.
Install the `incus-server` executable on the host that runs SPIRE Server.
Use paths that only the SPIRE process needs to read and execute.

Keep the TLS files on the server host only, in a directory that is not
shared with guests. Restrict filesystem permissions so only the SPIRE
Server account can read `tls_ca_path`, `tls_cert_path`, and
`tls_key_path`. Restrict network exposure so that client identity can
reach the Incus API and nothing else.

## Record SHA-256 checksums

Hash each installed file. Do not execute the plugin.

```sh
sha256sum /opt/spire/plugins/incus-agent
sha256sum /opt/spire/plugins/incus-server
```

On macOS or BSD userland, `shasum -a 256` produces the same digest.
Record the 64 hexadecimal digits for each file. Those values become
`plugin_checksum`.

Before you restart SPIRE, hash the files again and compare them with the
recorded digests. A mismatch means the file on disk is not the file you
intend to launch.

## Configure SPIRE Server

Put the trust domain in the server core block. Configure the NodeAttestor
named `incus` with `plugin_cmd`, `plugin_checksum`, and complete
`plugin_data`.

```hcl
server {
  trust_domain = "example.org"
}

plugins {
  NodeAttestor "incus" {
    plugin_cmd      = "/opt/spire/plugins/incus-server"
    plugin_checksum = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

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
  }
}
```

Replace the checksum placeholder with the SHA-256 of the installed
`incus-server` file. Replace the endpoint, TLS paths, and project list
with values for this deployment. `user_selectors` may be omitted when
you do not want configured `user.*` selectors.

Keep `challenge_response_timeout` greater than the agent `poll_timeout`.
The defaults are `10s` and `5s`. If you change either value, preserve
that margin so the guest can observe the nonce key before the server
gives up.

Field meanings, defaults, and bounds are in
[Configuration](../reference/configuration.md).

## Configure SPIRE Agent

Put the same trust domain in the agent core block. Configure the
NodeAttestor named `incus` with `plugin_cmd`, `plugin_checksum`, and
agent `plugin_data`.

```hcl
agent {
  trust_domain = "example.org"
}

plugins {
  NodeAttestor "incus" {
    plugin_cmd      = "/opt/spire/plugins/incus-agent"
    plugin_checksum = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

    plugin_data {
      project      = "default"
      poll_timeout = "5s"
    }
  }
}
```

Replace the checksum placeholder with the SHA-256 of the installed
`incus-agent` file. `project` is optional; set it to the guest’s Incus
project when you know it. `poll_timeout` may be omitted to use `5s`.

## Restart and verify

Restart SPIRE Server, then restart SPIRE Agent. SPIRE launches each
plugin from `plugin_cmd` and compares the executable with
`plugin_checksum`. A checksum mismatch prevents that plugin from
starting.

After a successful attestation, SPIRE records:

- agent ID `spiffe://<trust-domain>/spire/agent/incus/<lowercase-volatile.uuid>`
- `can_reattest = true`
- selector type `incus`, with values taken from the Incus API snapshot

Confirm those values against the live Incus record for the VM. Guest
claims only locate the instance; they do not define the ID or selectors.

If attestation fails closed, check that the guest is a
`virtual-machine` in an allowed project, that the agent can read the
guest socket and DMI UUID, and that the server can reach Incus with the
mounted credentials.
If the agent `project` hint is omitted, confirm that exactly one allowed
project contains a matching instance. Multiple matches are denied as
ambiguous.
On a standalone Incus server, guest `/1.0` location is the server name
while the instance API record reports `none`. The server plugin
substitutes the name it read from `/1.0` at Configure; confirm those
two names match.

## Roll back or revoke credentials

To roll back, restore the previous plugin paths, checksums, and
`plugin_data`, then restart SPIRE Server and SPIRE Agent.

If the Incus client certificate or key may have been exposed, revoke
that client identity on the Incus side, replace the files mounted beside
`incus-server`, update `tls_cert_path` and `tls_key_path` if the paths
changed, and restart SPIRE Server. Remove the old key material from the
server host.
