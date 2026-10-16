# Project structure

`vault-tui` is a read-only terminal UI for exploring HashiCorp Vault, built on
[Bubble Tea](https://github.com/charmbracelet/bubbletea) (Elm architecture in Go).

## Directory tree

```text
vault-tui/
├── main.go                      # Entry point, delegates to cmd.Execute()
├── cmd/root.go                  # CLI flags, config, token resolution, starts the TUI
├── internal/
│   ├── config/                  # Config from environment and flags, validation, warnings
│   ├── vault/
│   │   ├── reader.go            # Reader interface and domain types (Mount, Secret, ...)
│   │   ├── client.go            # Reader implementation on the official Vault API client
│   │   ├── auth.go              # Token resolution: env, token helper, ~/.vault-token
│   │   ├── roles.go             # Auth method type -> listable roles/users/groups
│   │   ├── paths.go             # Rejects names that could escape an API path
│   │   └── vaulttest/           # In-memory fake Reader for tests
│   └── tui/
│       ├── model.go             # Root model: tabs, layout, global keys
│       ├── keys/                # Key bindings
│       ├── safe/                # Neutralises untrusted text before display
│       ├── components/          # Status bar and help bar
│       └── views/               # Secrets, Policies and Auth tabs
├── scripts/seed-dev-vault.sh    # Seeds a dev Vault for the integration tests
├── .mise.toml                   # Pinned tools and tasks
├── .pre-commit-config.yaml      # Git hooks
├── .releaserc.json              # semantic-release configuration
├── renovate.json                # Dependency update policy (hosted Renovate app)
├── .goreleaser.yaml             # Binary builds attached to releases
└── .github/                     # CI, security and release workflows
```

## Packages

### `cmd`

Parses flags and the environment, resolves the token, builds the Vault client and
starts the TUI. It holds no business logic.

### `internal/config`

`Config` holds the address, token, CA certificate, namespace and TLS options.
`Validate` rejects unusable addresses (including credentials in the URL) and
`Warnings` reports insecure settings such as plain HTTP to a remote host.

### `internal/vault`

`Reader` is the only way the TUI talks to Vault. It is read-only on purpose:
write operations will be a separate interface. `Client` implements it with
redirects disabled and the ambient agent address ignored, so the token only goes
to the configured address. KV v1 and v2 are both supported.

### `internal/tui`

The root `Model` owns the tabs, the layout and the global key bindings. Each tab
is a `views.View` that loads its data asynchronously and renders lists and
detail panes. Every string that comes from Vault goes through `safe` first.

## Data flow

1. A view issues a `tea.Cmd` that calls the `Reader` with a timeout.
2. The result returns as a typed message, tagged with a sequence number so that
   stale responses are dropped.
3. The root model delivers load messages to every view and all other messages to
   the active view only.

## Key bindings

All bindings are declared once in `internal/tui/keys`. Views receive the key map
at construction instead of hard-coding keys.
