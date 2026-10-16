package vault

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path"
	"sort"
	"strings"

	"github.com/hashicorp/vault/api"

	"github.com/diablo02000/vault-tui/internal/config"
)

// Client is a Reader backed by the official Vault API client.
type Client struct {
	api *api.Client
}

var _ Reader = (*Client)(nil)

// Connect builds a Client from the application config and a token.
func Connect(cfg config.Config, token string) (*Client, error) {
	apiCfg := api.DefaultConfig()
	if apiCfg.Error != nil {
		return nil, fmt.Errorf("vault client config: %w", apiCfg.Error)
	}
	apiCfg.Address = cfg.Address
	// api.DefaultConfig also reads the ambient environment. Neutralise the
	// settings that could send the token somewhere other than cfg.Address:
	// VAULT_AGENT_ADDR silently overrides the address, and the client would
	// otherwise follow a server-supplied redirect carrying the X-Vault-Token header.
	apiCfg.AgentAddress = ""
	apiCfg.DisableRedirects = true
	if err := apiCfg.ConfigureTLS(
		&api.TLSConfig{CACert: cfg.CACert, Insecure: cfg.SkipTLSVerify},
	); err != nil {
		return nil, fmt.Errorf("configure TLS: %w", err)
	}
	c, err := api.NewClient(apiCfg)
	if err != nil {
		return nil, fmt.Errorf("create vault client: %w", err)
	}
	c.SetToken(token)
	if cfg.Namespace != "" {
		c.SetNamespace(cfg.Namespace)
	}
	return New(c), nil
}

// New wraps an existing API client.
func New(c *api.Client) *Client { return &Client{api: c} }

// wrapErr maps Vault HTTP errors onto the package sentinel errors.
func wrapErr(err error) error {
	var re *api.ResponseError
	if errors.As(err, &re) {
		switch re.StatusCode {
		case http.StatusForbidden:
			return fmt.Errorf("%w: %s", ErrPermissionDenied, re.Error())
		case http.StatusNotFound:
			return fmt.Errorf("%w: %s", ErrNotFound, re.Error())
		}
	}
	return err
}

func (c *Client) ListMounts(ctx context.Context) ([]Mount, error) {
	out, err := c.api.Sys().ListMountsWithContext(ctx)
	if err != nil {
		return nil, wrapErr(err)
	}
	mounts := make([]Mount, 0, len(out))
	for p, m := range out {
		mounts = append(mounts, Mount{
			Path:        p,
			Type:        m.Type,
			Description: m.Description,
			Accessor:    m.Accessor,
			Version:     kvVersion(m.Type, m.Options),
		})
	}
	sort.Slice(mounts, func(i, j int) bool { return mounts[i].Path < mounts[j].Path })
	return mounts, nil
}

// kvVersion returns the KV engine version for kv/generic mounts and 0 otherwise.
func kvVersion(typ string, opts map[string]string) int {
	switch typ {
	case "kv":
		if opts["version"] == "2" {
			return 2
		}
		return 1
	case "generic":
		return 1
	}
	return 0
}

// kvPath builds the API path for a KV operation ("metadata"/"data" on v2).
func kvPath(m Mount, v2Prefix, p string) string {
	if m.Version == 2 {
		return m.Path + v2Prefix + "/" + p
	}
	return m.Path + p
}

func (c *Client) ListSecrets(ctx context.Context, mount Mount, dir string) ([]string, error) {
	if !mount.Browsable() {
		return nil, fmt.Errorf("%w: mount type %q is not browsable", ErrUnsupported, mount.Type)
	}
	if err := checkPath(dir); err != nil {
		return nil, err
	}
	return c.list(ctx, strings.TrimSuffix(kvPath(mount, "metadata", dir), "/"))
}

func (c *Client) ReadSecret(ctx context.Context, mount Mount, p string) (*Secret, error) {
	if !mount.Browsable() {
		return nil, fmt.Errorf("%w: mount type %q is not browsable", ErrUnsupported, mount.Type)
	}
	if err := checkPath(p); err != nil {
		return nil, err
	}
	s, err := c.api.Logical().ReadWithContext(ctx, kvPath(mount, "data", p))
	if err != nil {
		return nil, wrapErr(err)
	}
	if s == nil {
		return nil, ErrNotFound
	}
	secret := &Secret{Path: mount.Path + p}
	if mount.Version == 2 {
		data, _ := s.Data["data"].(map[string]any)
		if data == nil {
			return nil, fmt.Errorf(
				"%w: secret has no data (deleted or destroyed version?)",
				ErrNotFound,
			)
		}
		secret.Data = data
		secret.Metadata, _ = s.Data["metadata"].(map[string]any)
	} else {
		secret.Data = s.Data
	}
	return secret, nil
}

func (c *Client) ListPolicies(ctx context.Context) ([]string, error) {
	names, err := c.api.Sys().ListPoliciesWithContext(ctx)
	if err != nil {
		return nil, wrapErr(err)
	}
	sort.Strings(names)
	return names, nil
}

func (c *Client) ReadPolicy(ctx context.Context, name string) (string, error) {
	if err := checkPath(name); err != nil {
		return "", err
	}
	rules, err := c.api.Sys().GetPolicyWithContext(ctx, name)
	if err != nil {
		return "", wrapErr(err)
	}
	if rules == "" {
		return "", ErrNotFound
	}
	return rules, nil
}

func (c *Client) ListAuthMethods(ctx context.Context) ([]AuthMount, error) {
	out, err := c.api.Sys().ListAuthWithContext(ctx)
	if err != nil {
		return nil, wrapErr(err)
	}
	methods := make([]AuthMount, 0, len(out))
	for p, m := range out {
		version := m.RunningVersion
		if version == "" {
			version = m.PluginVersion
		}
		methods = append(methods, AuthMount{
			Path:        p,
			Type:        m.Type,
			Description: m.Description,
			Accessor:    m.Accessor,
			Version:     version,
		})
	}
	sort.Slice(methods, func(i, j int) bool { return methods[i].Path < methods[j].Path })
	return methods, nil
}

func (c *Client) ListAuthRoles(ctx context.Context, mount AuthMount) ([]AuthRole, error) {
	kinds, ok := roleKinds[mount.Type]
	if !ok {
		return nil, fmt.Errorf("%w: listing entries of auth type %q", ErrUnsupported, mount.Type)
	}
	var roles []AuthRole
	var firstErr error
	for _, k := range kinds {
		names, err := c.list(ctx, "auth/"+path.Join(mount.Path, k.listPath))
		if err != nil {
			// Keep going: e.g. ldap "groups" may be readable while "users" is not.
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		for _, n := range names {
			roles = append(roles, AuthRole{Mount: mount, Kind: k.label, Name: n})
		}
	}
	if len(roles) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return roles, nil
}

func (c *Client) ReadAuthRole(ctx context.Context, role AuthRole) (map[string]any, error) {
	if err := checkPath(role.Name); err != nil {
		return nil, err
	}
	for _, k := range roleKinds[role.Mount.Type] {
		if k.label != role.Kind {
			continue
		}
		s, err := c.api.Logical().
			ReadWithContext(ctx, "auth/"+path.Join(role.Mount.Path, k.readPath, role.Name))
		if err != nil {
			return nil, wrapErr(err)
		}
		if s == nil {
			return nil, ErrNotFound
		}
		return s.Data, nil
	}
	return nil, fmt.Errorf(
		"%w: reading %q of auth type %q",
		ErrUnsupported,
		role.Kind,
		role.Mount.Type,
	)
}

// list performs a LIST and returns the sorted keys (empty when nothing exists).
func (c *Client) list(ctx context.Context, p string) ([]string, error) {
	s, err := c.api.Logical().ListWithContext(ctx, p)
	if err != nil {
		return nil, wrapErr(err)
	}
	if s == nil || s.Data == nil {
		return nil, nil
	}
	raw, _ := s.Data["keys"].([]any)
	keys := make([]string, 0, len(raw))
	for _, k := range raw {
		if str, ok := k.(string); ok {
			keys = append(keys, str)
		}
	}
	sort.Strings(keys)
	return keys, nil
}
