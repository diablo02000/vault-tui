package vault

import (
	"context"
	"errors"
)

// Sentinel errors returned by Reader implementations.
var (
	ErrNotFound         = errors.New("not found")
	ErrPermissionDenied = errors.New("permission denied")
	ErrUnsupported      = errors.New("not supported")
)

// Mount is a secret engine mount, as listed by `vault secrets list`.
type Mount struct {
	Path        string // always with a trailing slash, e.g. "secret/"
	Type        string
	Description string
	Accessor    string
	Version     int // KV engine version (1 or 2); 0 for non-KV mounts
}

// Browsable reports whether the mount's contents can be explored (KV engines only).
func (m Mount) Browsable() bool { return m.Version > 0 }

// Secret is the content of a KV secret.
type Secret struct {
	Path     string
	Data     map[string]any
	Metadata map[string]any // KV v2 only
}

// AuthMount is an auth method mount, as listed by `vault auth list`.
type AuthMount struct {
	Path        string // always with a trailing slash, e.g. "userpass/"
	Type        string
	Description string
	Accessor    string
	Version     string
}

// AuthRole is an entry (role, user, group, ...) exposed by an auth method.
type AuthRole struct {
	Mount AuthMount
	Kind  string // e.g. "roles", "users", "groups"
	Name  string
}

// Reader is the read-only view of Vault used by the TUI.
// Write operations will be added as a separate interface.
type Reader interface {
	ListMounts(ctx context.Context) ([]Mount, error)
	// ListSecrets lists the keys under dir ("" or "a/b/"). Folders end with "/".
	ListSecrets(ctx context.Context, mount Mount, dir string) ([]string, error)
	ReadSecret(ctx context.Context, mount Mount, path string) (*Secret, error)

	ListPolicies(ctx context.Context) ([]string, error)
	ReadPolicy(ctx context.Context, name string) (string, error)

	ListAuthMethods(ctx context.Context) ([]AuthMount, error)
	ListAuthRoles(ctx context.Context, mount AuthMount) ([]AuthRole, error)
	ReadAuthRole(ctx context.Context, role AuthRole) (map[string]any, error)
}
