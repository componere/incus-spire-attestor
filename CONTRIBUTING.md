# Contributing

Contributions should stay focused on the two external SPIRE NodeAttestor plugins in this repository: `incus-agent` and `incus-server`.

Report vulnerabilities through [SECURITY.md](SECURITY.md). Do not use public issues, pull requests, or discussions for security reports.

## Reporting bugs

Use GitHub issues for non-sensitive bugs. Include the following when you can:

- version, commit, or environment details
- steps to reproduce
- expected behavior
- actual behavior
- logs or a minimal reproduction

If the report is a vulnerability, stop and follow [SECURITY.md](SECURITY.md).

## Pull requests

1. Keep the change scoped to one problem.
2. Add or update tests when behavior changes.
3. Update documentation when user-facing behavior changes.
4. Use Conventional Commit subjects, such as `feat: add config loader` or `fix: handle empty input`.
5. Run the checks below before you request review.

## Local setup

Provision the pinned toolchain, then use Moon for the project tasks:

```sh
mise install
moon run root:format
moon run root:lint
moon run root:test
moon run root:build
moon run root:check
```

`moon run root:check` is the aggregate local check.

## Focused Go commands

Use these when you are changing one plugin or an internal package. They compile or test the packages. Do not run the plugin binaries; SPIRE loads them as external plugins.

```sh
go test ./cmd/incus-agent
go test ./cmd/incus-server
go test ./internal/...
go build -o bin/incus-agent ./cmd/incus-agent
go build -o bin/incus-server ./cmd/incus-server
```

## Documentation

Validate the MkDocs site with:

```sh
moon run docs:build
```

## Security reports

Use [SECURITY.md](SECURITY.md) for private vulnerability reporting.
