// Package vaulttest provides an in-memory vault.Reader for tests.
package vaulttest

import (
	"context"
	"fmt"

	"github.com/diablo02000/vault-tui/internal/vault"
)

// Fake is a canned vault.Reader. Err, when set, is returned by every call.
type Fake struct {
	Err error

	Mounts      []vault.Mount
	Keys        map[string][]string // key: mount path + dir
	Secrets     map[string]*vault.Secret
	Policies    map[string]string
	AuthMethods []vault.AuthMount
	Roles       map[string][]vault.AuthRole // key: auth mount path
	RoleData    map[string]map[string]any   // key: auth mount path + kind + "/" + name
}

var _ vault.Reader = (*Fake)(nil)

func (f *Fake) ListMounts(context.Context) ([]vault.Mount, error) { return f.Mounts, f.Err }

func (f *Fake) ListSecrets(_ context.Context, m vault.Mount, dir string) ([]string, error) {
	return f.Keys[m.Path+dir], f.Err
}

func (f *Fake) ReadSecret(_ context.Context, m vault.Mount, p string) (*vault.Secret, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	if s, ok := f.Secrets[m.Path+p]; ok {
		return s, nil
	}
	return nil, vault.ErrNotFound
}

func (f *Fake) ListPolicies(context.Context) ([]string, error) {
	names := make([]string, 0, len(f.Policies))
	for n := range f.Policies {
		names = append(names, n)
	}
	sortStrings(names)
	return names, f.Err
}

func (f *Fake) ReadPolicy(_ context.Context, name string) (string, error) {
	if f.Err != nil {
		return "", f.Err
	}
	if p, ok := f.Policies[name]; ok {
		return p, nil
	}
	return "", vault.ErrNotFound
}

func (f *Fake) ListAuthMethods(context.Context) ([]vault.AuthMount, error) {
	return f.AuthMethods, f.Err
}

func (f *Fake) ListAuthRoles(_ context.Context, m vault.AuthMount) ([]vault.AuthRole, error) {
	return f.Roles[m.Path], f.Err
}

func (f *Fake) ReadAuthRole(_ context.Context, r vault.AuthRole) (map[string]any, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	if d, ok := f.RoleData[fmt.Sprintf("%s%s/%s", r.Mount.Path, r.Kind, r.Name)]; ok {
		return d, nil
	}
	return nil, vault.ErrNotFound
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
