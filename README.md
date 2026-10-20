# vault-tui

An interactive, read-only terminal UI for exploring HashiCorp Vault, built with [Bubble Tea](https://github.com/charmbracelet/bubbletea).

[![CI](https://github.com/diablo02000/vault-tui/actions/workflows/ci.yml/badge.svg)](https://github.com/diablo02000/vault-tui/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/diablo02000/vault-tui)](https://github.com/diablo02000/vault-tui/releases/latest)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/diablo02000/vault-tui/badge)](https://scorecard.dev/viewer/?uri=github.com/diablo02000/vault-tui)
[![License: MIT](https://img.shields.io/github/license/diablo02000/vault-tui)](LICENSE)

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->
**Table of Contents**

- [Features](#features)
- [Requirements](#requirements)
- [Getting started](#getting-started)
- [Authentication](#authentication)
- [Configuration](#configuration)
- [Key bindings](#key-bindings)
- [Security model](#security-model)
- [Development](#development)
- [Verifying a release](#verifying-a-release)
- [Continuous integration](#continuous-integration)
- [License](#license)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Features

- **Secrets**: browse secret engines, then KV v1 and v2 paths and secrets (values hidden until you press `v`)
- **Policies**: list ACL policies and read their HCL
- **Auth**: list auth methods (like `vault auth list`), then their roles, users and groups
- Keyboard-driven navigation with filtering (`/`)

## Requirements

- [mise](https://mise.jdx.dev/): manages the Go version, linter and task runner
- A terminal with [OSC 52](https://invisible-island.net/xterm/ctlseqs/ctlseqs.html#h3-Operating-System-Commands) support, used to copy secrets to the clipboard (some terminals require enabling it)
- When running inside tmux (3.3 or later), passthrough must be enabled in `~/.tmux.conf`, otherwise copying silently does nothing:

  ```tmux
  set -g allow-passthrough on
  ```

## Getting started

```sh
mise install          # installs the Go version and tools pinned in .mise.toml
mise run build        # builds ./bin/vault-tui
./bin/vault-tui --address https://vault.example.com
```

## Authentication

The token is resolved like the Vault CLI does, first match wins:

1. `VAULT_TOKEN` environment variable
2. The token helper: the `token_helper` command configured in `~/.vault` (or the file named by `VAULT_CONFIG_PATH`), run with the argument `get`
3. `~/.vault-token`

There is deliberately no `--token` flag: command-line arguments are visible to other users through `ps`.

## Configuration

| Variable            | Flag                | Description                                   | Default                  |
|---------------------|---------------------|-----------------------------------------------|--------------------------|
| `VAULT_ADDR`        | `-a, --address`     | Vault server address                          | `https://127.0.0.1:8200` |
| `VAULT_NAMESPACE`   | `-n, --namespace`   | Vault namespace                               |                          |
| `VAULT_CACERT`      | `--ca-cert`         | Path to a CA certificate                      |                          |
| `VAULT_SKIP_VERIFY` | `--tls-skip-verify` | Skip TLS verification (insecure)              | `false`                  |
| `VAULT_TOKEN`       |                     | Vault token                                   |                          |

Flags override environment variables.

## Key bindings

| Key              | Action                          |
|------------------|---------------------------------|
| `↑/k`, `↓/j`     | Move up / down                  |
| `enter`          | Open                            |
| `esc`            | Back (first clears a filter)    |
| `/`              | Filter the current list         |
| `tab`, `1` `2` `3` | Switch tab                    |
| `v`              | Reveal / hide secret values     |
| `y`, `c`        | Copy the selected secret value to the clipboard |
| `s`              | Search secrets (opens a pane over the browser) |
| `r`              | Refresh                         |
| `?`              | Toggle help                     |
| `q`, `ctrl+c`    | Quit                            |

## Security model

The Vault server is trusted with your token but **not** trusted to render text:

- **Terminal injection**: every string coming from Vault (secret names and values, policies, descriptions, errors) is sanitised before display. Control characters, escape sequences and bidirectional overrides are replaced, and newlines in values are shown as `\n`, so a hostile value cannot rewrite the screen or fake extra rows.
- **Clipboard**: `y` copies the selected value without displaying it, using the OSC 52 terminal escape sequence, so it works over SSH and needs no `xclip`, `xsel` or `wl-clipboard`. Your terminal must support OSC 52 (most do; some ask you to enable it) and, inside tmux, `set -g allow-passthrough on`. The value is sent base64-encoded and is never kept in memory by the clipboard layer. OSC 52 cannot read the clipboard back, so the clipboard is overwritten with an empty value 30 seconds after the copy and on quit, even if you copied something else in the meantime. The copied value is raw, so paste untrusted secrets into a terminal with care.
- **Search**: `s` opens a search pane with three fields, and `enter` runs the search.
  - *Path scope* is a glob matched level by level from the mount, where `*` is one level and `**` any depth. `secrets/kubernetes/*/smart-entry/smart-entry-vault/` searches only the `int`, `sta` and `prd` folders; folders that cannot match are never listed, and everything below the matched folder is searched.
  - *Path regex* filters the full secret path, and *Key regex* matches secret key names. Both are RE2 (no catastrophic backtracking) and case-insensitive. Without a key regex, secrets are matched on their path alone and never read.
  - Results show paths and matching key names only, never values; open one with `enter` and `esc` returns to the results. A search is bounded (3000 Vault calls, 1000 results, 8 concurrent calls, 2 minutes), can be cancelled with `esc`, and reports skipped (e.g. permission denied) and truncated searches.
- **Token confinement**: redirects are never followed (the Vault client would re-send the token to the redirect target), and the ambient `VAULT_AGENT_ADDR` is ignored so requests always go to the address shown in the status bar.
- **Address**: URLs containing credentials are rejected. Plain HTTP to a non-loopback host and `--tls-skip-verify` display a warning in the status bar.
- **Token handling**: tokens are validated (no whitespace or control characters, bounded length), the token helper runs with a timeout and without a shell, and `~/.vault` / `~/.vault-token` are read only if they are small regular files. Tokens are never logged.
- **Secrets in memory**: secret values are masked by default, and are dropped when you leave the secret.
- **Request paths**: names containing `..` or control characters are refused before they reach an API path.

## Development

```sh
mise run test              # unit tests (race detector, shuffled)
mise run test-coverage     # tests + HTML coverage report
mise run test-integration  # against a dev Vault: needs VAULT_ADDR and VAULT_TOKEN
mise run fuzz              # fuzz the output sanitiser
mise run lint              # golangci-lint, including gosec
mise run vuln              # govulncheck on dependencies and the standard library
mise run hygiene           # fails on machine-specific paths, real-looking emails, token-like strings
mise run security          # hygiene + module integrity + lint + vuln
mise run check             # security + tests
mise run build             # static, reproducible binary (-trimpath, no embedded local paths)
```

Run `mise run check` before every commit. The Go version is pinned to a patched release in `.mise.toml` and `go.mod`; `mise run vuln` fails when the toolchain or a dependency has a known vulnerability.

## Verifying a release

Each release page has `vault-tui_<version>_<os>_<arch>.tar.gz` archives and `checksums.txt`.
Archives carry a GitHub build-provenance attestation:

```sh
sha256sum --check --ignore-missing checksums.txt
gh attestation verify vault-tui_<version>_linux_amd64.tar.gz --repo <owner>/vault-tui
```

## Continuous integration

| Workflow | Trigger | What it does |
|----------|---------|--------------|
| `ci.yml` | PR, push to `main` | lint, `go.mod` integrity, workflow lint, unit tests (Linux + macOS, race detector), integration tests against a real Vault dev server, build. The `CI passed` job is the single check to require. |
| `security.yml` | PR, push, weekly | `govulncheck`, hygiene scan, CodeQL, dependency review |
| `scorecard.yml` | push, weekly | OpenSSF Scorecard: grades supply-chain practices, results in the Security tab |
| `pr-title.yml` | PR | enforces a [Conventional Commit](https://www.conventionalcommits.org/) title |
| `release.yml` | push to `main` | [semantic-release](https://semantic-release.gitbook.io/) computes the next version from Conventional Commits, tags it and publishes the GitHub release with generated notes, then calls `build-binaries.yml`. It needs no secret. |
| `build-binaries.yml` | `v*` tag push, manual | builds linux and darwin binaries for amd64 and arm64 with GoReleaser, attaches the archives and `checksums.txt` to the release page, and adds build-provenance attestations |
| Renovate (hosted GitHub App) | PRs and dashboard | [Renovate](https://docs.renovatebot.com/) reads `renovate.json` and opens PRs for Go libraries, pinned action SHAs, the Vault image, npm release tooling, pre-commit hooks and the tools in `.mise.toml`. Security fixes are opened immediately; other updates on Monday mornings. It runs outside GitHub Actions, so it needs no secret. |

Every action is pinned to a commit SHA, and Renovate keeps the pins current. CI runs the same `mise run` tasks as local development, so `mise run check` reproduces it.

## License

MIT
