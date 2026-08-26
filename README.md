# incus-spire-attestor

This repository contains two Linux SPIRE NodeAttestor plugins: `incus-agent` implements `agent/nodeattestor/v1`, and `incus-server` implements `server/nodeattestor/v1`. Both plugins use the fixed logical name `incus`. Supported plugin targets are Linux `amd64` and `arm64`.

v1 attests that guest claims match one allowed Incus virtual machine at attestation time. It does not prove exclusive guest residency.

SPIRE launches the plugins. Set `plugin_cmd` and `plugin_checksum` in the outer SPIRE HCL, not in `plugin_data`.

## Documentation

- [Deploy the plugins](docs/docs/how-to/deploy.md)
- [Configuration reference](docs/docs/reference/configuration.md)
- [Security model](docs/docs/explanation/security-model.md)

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
