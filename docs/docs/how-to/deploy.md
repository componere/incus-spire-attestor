---
title: Deploy
description: Deploy the Incus SPIRE NodeAttestor plugins on Linux.
---

# Deploy the Incus NodeAttestor plugins

This guide installs `incus-agent` and `incus-server` beside SPIRE,
restricts the Incus client identity that the server plugin uses, and
configures both NodeAttestors under the logical name `incus`. SPIRE
launches the plugins from `plugin_cmd`; do not run them directly.

## Prerequisites

- Linux `amd64` or `arm64` hosts for both plugins. Other operating
  systems and architectures are not supported.
- A SPIRE Agent inside an Incus virtual machine whose project the
  server plugin `projects` list allows. The guest must expose
  `/dev/incus/sock` and `/sys/class/dmi/id/product_uuid` to the agent
  process, its cloud-init `local-hostname` must equal the Incus
  instance name, and the instance must have
  `volatile.cloud-init.instance-id` set.
- Network reachability from the SPIRE Server host to the Incus API
  endpoint you will set in `incus_endpoint`.
- An Incus TLS client identity for the server plugin: the Incus CA
  certificate, a client certificate, and the client key. The identity
  needs instance lookup and instance-edit authorization in the allowed
  projects. Read [Security model](../explanation/security-model.md)
  for what that authority implies before you deploy.

## Build the binaries

Build both plugins from source:

```sh
mise install
moon run root:build
```

The build writes static Linux binaries to `bin/linux_amd64/` and
`bin/linux_arm64/`.

## Restrict the Incus client identity

Create a dedicated TLS client certificate for the server plugin and add
it to the Incus trust store. Then restrict it to the projects you will
allowlist in `projects`:

```sh
incus config trust edit <fingerprint>
```

Set `restricted: true` and list the allowed projects.

A restricted identity completes the whole attestation flow — instance
lookup, nonce write, and nonce removal — while every other project is
invisible to its list requests and returns HTTP 403 on direct access.
Within its allowed projects, the identity can still modify, rename, or
delete instances. If that remaining authority is unacceptable, stop; no
plugin configuration narrows it further.

## Place the binaries

Install the `incus-agent` executable on the guest that runs SPIRE Agent.
Install the `incus-server` executable on the host that runs SPIRE Server.
Use paths that only the SPIRE process needs to read and execute.

Keep the TLS files on the server host only, in a directory that is not
shared with guests. Restrict filesystem permissions so only the SPIRE
Server account can read `tls_ca_path`, `tls_cert_path`, and
`tls_key_path`. Restrict network exposure so that client identity can
reach the Incus API and nothing else. Do not copy the TLS files into
the guest or onto any other host.

## Record SHA-256 checksums

Hash each installed file. Do not execute the plugin.

```sh
sha256sum /opt/spire/plugins/incus-agent
sha256sum /opt/spire/plugins/incus-server
```

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

Keep `challenge_response_timeout` greater than the agent `poll_timeout`
so the guest can observe the nonce key before the server gives up. The
defaults, `10s` and `5s`, already have that margin.

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
`incus-agent` file. `project` is optional; set it to the guest's Incus
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

If the Incus client certificate or key may have been exposed, remove
that certificate from the Incus trust store, replace the files mounted
beside `incus-server`, update `tls_cert_path` and `tls_key_path` if the
paths changed, and restart SPIRE Server. Remove the old key material
from the server host.
