# Contributing to vault-tui

Thanks for helping improve vault-tui, a read-only terminal UI for exploring HashiCorp Vault.
This guide explains how to set up your environment, the checks your change must pass, and how
releases are produced.

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->
**Table of Contents**

- [Getting started](#getting-started)
- [Project layout](#project-layout)
- [Development workflow](#development-workflow)
  - [Quality checks](#quality-checks)
  - [Pre-commit hooks](#pre-commit-hooks)
- [Commit messages](#commit-messages)
- [Releases](#releases)
- [Repository settings](#repository-settings)
- [Security rules](#security-rules)
- [Tests](#tests)
- [Code style](#code-style)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Getting started

[mise](https://mise.jdx.dev/) manages every tool version (Go, linters, pre-commit, ...), so a
fresh checkout needs only mise and git:

```sh
mise install     # installs the tools pinned in .mise.toml
mise run hooks   # installs the git hooks (pre-commit, commit-msg, pre-push)
mise run check   # runs the full local quality gate
```

Please do not rely on a globally installed Go or linter: the pinned versions are the ones CI uses.

## Project layout

| Path | Purpose |
|------|---------|
| `cmd/` | CLI wiring: flags, configuration, starting the TUI |
| `internal/config/` | Configuration from environment and flags |
| `internal/vault/` | Vault access (`Reader` interface), token resolution |
| `internal/vault/vaulttest/` | In-memory fake `Reader` for tests |
| `internal/tui/` | Bubble Tea application, views, components, key bindings |
| `internal/tui/safe/` | Neutralises untrusted text before it reaches the terminal |
| `scripts/` | Helper scripts (for example seeding a dev Vault) |

## Development workflow

1. Fork and create a branch from `main`.
2. Make your change, with tests.
3. Run `mise run check` (see [Quality checks](#quality-checks)).
4. Commit using [Conventional Commits](#commit-messages).
5. Open a pull request. Its title must also be a Conventional Commit, because pull requests are
   squash-merged and the title becomes the commit message that drives the release.

### Quality checks

| Task | What it does |
|------|--------------|
| `mise run test` | Unit tests with the race detector, shuffled |
| `mise run test-integration` | Tests against a real dev Vault (needs `VAULT_ADDR` and `VAULT_TOKEN`) |
| `mise run lint` | golangci-lint, including gosec |
| `mise run vuln` | govulncheck on dependencies and the standard library |
| `mise run hygiene` | Fails on machine-specific paths, real-looking emails, token-like strings |
| `mise run secrets` | Scans the git history for committed secrets (gitleaks) |
| `mise run pre-commit` | Runs every pre-commit hook on all files |
| `mise run check` | Everything above that is mandatory, plus the tests |

To run the integration tests locally, start a dev server and seed it:

```sh
vault server -dev -dev-root-token-id=dev-root-token &
export VAULT_ADDR=http://127.0.0.1:8200 VAULT_TOKEN=dev-root-token
scripts/seed-dev-vault.sh
mise run test-integration
```

### Pre-commit hooks

Hooks run automatically after `mise run hooks`:

- **On commit**: Markdown TOC and lint, spelling, secret detection, GitHub Actions lint, file
  hygiene, Go formatting and lint, shell lint, and the data hygiene scan.
- **On commit message**: Conventional Commit format (commitizen).
- **On push**: unit tests and `govulncheck`, which are slower.

A hook that rewrites a file (formatting, TOC) fails the commit once: review the change, stage it,
and commit again. Never bypass hooks with `--no-verify`; CI runs the same checks.

## Commit messages

Releases are fully automated by [semantic-release](https://semantic-release.gitbook.io/), which
reads commit messages on `main`. Follow [Conventional Commits](https://www.conventionalcommits.org/):

```text
<type>(<optional scope>): <short summary>

<optional body>

<optional footer, e.g. BREAKING CHANGE: ...>
```

| Type | Release | Use for |
|------|---------|---------|
| `feat` | minor | A user-visible feature |
| `fix` | patch | A bug fix |
| `perf` | patch | A performance improvement |
| `refactor` | patch | Code change that neither fixes a bug nor adds a feature |
| `build(deps)` | patch | Dependency updates |
| `docs`, `test`, `ci`, `chore`, `style` | none | Everything that does not change the binary |
| any type with `!` or a `BREAKING CHANGE:` footer | major | Incompatible changes |

Examples:

```text
feat(auth): list ldap groups next to users
fix(secrets): handle KV v2 secrets with a deleted latest version
feat!: drop support for Vault < 1.12
```

## Releases

You never tag or edit the version by hand. When commits land on `main`, the `Release` workflow:

1. computes the next version from the commit types,
2. tags the commit and publishes the GitHub release, with notes generated from the commits,
3. builds the binaries (linux and darwin, amd64 and arm64) and attaches them to the release,
   with checksums and a build-provenance attestation.

The changelog is the list of GitHub releases. A commit that is not a feature, fix or other
releasable type does not create a release.

## Repository settings

These settings were applied once, by hand, in the GitHub UI. Re-check them after any change of
owner or plan.

- **Rulesets:**
  - `main`: no deletion, no force-push, linear history, PR required (squash only, 0 approvals,
    conversations resolved), required checks `CI passed` and `Conventional Commit title`.
  - `release-tags`: `v*` tags cannot be deleted or updated.
  - The repository admin role is the only bypass actor.
- **Merging:** squash only, the PR title and body become the commit message, branches are deleted
  after merge.
- **Security:** secret scanning with push protection, Dependabot alerts (not security updates,
  Renovate covers those), private vulnerability reporting, immutable releases.
- **Actions:** SHA pinning required, only GitHub-owned actions and the pinned third-party ones
  allowed, default token read-only, fork PRs need approval.

Releases need no repository secret: the workflow uses the default `GITHUB_TOKEN`, which can create
tags and releases but cannot bypass the rulesets, and it does not need to.

## Security rules

vault-tui handles Vault tokens and secret values, so these rules are not optional:

- **Treat everything from Vault as hostile text.** Pass any server-provided string through
  `internal/tui/safe` before rendering it. New views must do the same, and tests must cover it.
- **Never log, print or pass tokens in arguments.** There is deliberately no `--token` flag.
- **Keep the client read-only.** Write operations belong in a separate interface, not in `Reader`.
- **Keep the repository generic.** No personal paths, emails, hostnames or real tokens, in code,
  tests or docs; use `example.com` and obviously fake values. `mise run hygiene` enforces this.
- **Validate input that reaches an API path.** See `internal/vault/paths.go`.
- Report vulnerabilities privately through the repository's security advisories, not in a public issue.

## Tests

- Table-driven tests with `t.Run` are preferred.
- Tests must be hermetic: no real home directory, no ambient `VAULT_*` variables, no network.
  Inject dependencies (see `vault.TokenDeps`) or use `t.Setenv` and `t.TempDir`.
- Use `internal/vault/vaulttest` to fake Vault in TUI tests, and `httptest` for the client.
- A bug fix should come with a test that fails without the fix.
- Tests that need a real Vault carry the `integration` build tag.

## Code style

- `gofmt`, `goimports` and `golines` are applied by `golangci-lint fmt`; the hook does it for you.
- Exported identifiers get doc comments; keep comments for the *why*, not the *what*.
- Keep functions small and wrap errors with context (`fmt.Errorf("...: %w", err)`).
