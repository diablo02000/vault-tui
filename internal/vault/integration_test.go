//go:build integration

package vault_test

import (
	"context"
	"os"
	"testing"

	"github.com/diablo02000/vault-tui/internal/config"
	"github.com/diablo02000/vault-tui/internal/vault"
)

// Run against a seeded dev server:
//
//	VAULT_ADDR=<dev-server-addr> VAULT_TOKEN=<dev-root-token> go test -tags integration ./internal/vault
func TestIntegration(t *testing.T) {
	addr, token := os.Getenv("VAULT_ADDR"), os.Getenv("VAULT_TOKEN")
	if addr == "" || token == "" {
		t.Skip("VAULT_ADDR and VAULT_TOKEN required")
	}
	c, err := vault.Connect(config.Config{Address: addr}, token)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	mounts, err := c.ListMounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]vault.Mount{}
	for _, m := range mounts {
		byPath[m.Path] = m
	}
	if byPath["secret/"].Version != 2 || byPath["legacy/"].Version != 1 ||
		byPath["sys/"].Browsable() {
		t.Fatalf("unexpected mounts: %+v", byPath)
	}

	keys, err := c.ListSecrets(ctx, byPath["secret/"], "app/")
	if err != nil || len(keys) != 1 || keys[0] != "db" {
		t.Fatalf("v2 keys = %v, %v", keys, err)
	}
	s, err := c.ReadSecret(ctx, byPath["secret/"], "app/db")
	if err != nil || s.Data["password"] != "hunter2" || s.Metadata["version"] == nil {
		t.Fatalf("v2 secret = %+v, %v", s, err)
	}
	s, err = c.ReadSecret(ctx, byPath["legacy/"], "old")
	if err != nil || s.Data["k"] != "v" {
		t.Fatalf("v1 secret = %+v, %v", s, err)
	}

	rules, err := c.ReadPolicy(ctx, "reader")
	if err != nil || rules == "" {
		t.Fatalf("policy = %q, %v", rules, err)
	}

	methods, err := c.ListAuthMethods(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var up vault.AuthMount
	for _, m := range methods {
		if m.Type == "userpass" {
			up = m
		}
	}
	roles, err := c.ListAuthRoles(ctx, up)
	if err != nil || len(roles) != 1 || roles[0].Name != "bob" {
		t.Fatalf("roles = %+v, %v", roles, err)
	}
	if d, err := c.ReadAuthRole(ctx, roles[0]); err != nil || d["token_policies"] == nil {
		t.Fatalf("role = %v, %v", d, err)
	}
}
