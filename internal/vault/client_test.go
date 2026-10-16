package vault_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/vault/api"

	"github.com/diablo02000/vault-tui/internal/config"
	"github.com/diablo02000/vault-tui/internal/vault"
)

// newTestClient starts a fake Vault whose responses are looked up by "METHOD path".
// LIST requests arrive as GET with ?list=true, which the handler normalises to LIST.
func newTestClient(t *testing.T, routes map[string]any) *vault.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := r.Method
		if r.URL.Query().Get("list") == "true" || method == "LIST" {
			method = "LIST"
		}
		key := method + " " + strings.TrimPrefix(r.URL.Path, "/v1/")
		resp, ok := routes[key]
		if !ok {
			t.Logf("unrouted request: %s", key)
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"errors":[]}`))
			return
		}
		switch v := resp.(type) {
		case int:
			w.WriteHeader(v)
			_, _ = w.Write([]byte(`{"errors":["denied"]}`))
		default:
			_ = json.NewEncoder(w).Encode(v)
		}
	}))
	t.Cleanup(srv.Close)

	cfg := api.DefaultConfig()
	cfg.Address = srv.URL
	cfg.MaxRetries = 0
	c, err := api.NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	c.SetToken("test")
	return vault.New(c)
}

func data(m map[string]any) map[string]any { return map[string]any{"data": m} }

func keys(k ...string) map[string]any { return data(map[string]any{"keys": k}) }

func TestListMounts(t *testing.T) {
	c := newTestClient(t, map[string]any{
		"GET sys/mounts": map[string]any{
			"data": map[string]any{
				"secret/": map[string]any{
					"type":        "kv",
					"description": "v2",
					"accessor":    "a1",
					"options":     map[string]any{"version": "2"},
				},
				"legacy/":  map[string]any{"type": "kv", "accessor": "a2", "options": nil},
				"generic/": map[string]any{"type": "generic", "accessor": "a3"},
				"pki/":     map[string]any{"type": "pki", "accessor": "a4"},
			},
		},
	})
	got, err := c.ListMounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	versions := map[string]int{}
	for _, m := range got {
		versions[m.Path] = m.Version
	}
	want := map[string]int{"secret/": 2, "legacy/": 1, "generic/": 1, "pki/": 0}
	if !reflect.DeepEqual(versions, want) {
		t.Errorf("versions = %v, want %v", versions, want)
	}
	if got[0].Path != "generic/" {
		t.Errorf("mounts not sorted: first = %s", got[0].Path)
	}
	if (vault.Mount{Version: 0}).Browsable() {
		t.Error("non-KV mount must not be browsable")
	}
}

func TestListSecrets(t *testing.T) {
	c := newTestClient(t, map[string]any{
		"LIST secret/metadata/app":  keys("web", "db/"),
		"LIST secret/metadata":      keys("app/"),
		"LIST legacy/team":          keys("x"),
		"LIST secret/metadata/none": 403,
	})
	ctx := context.Background()
	v2 := vault.Mount{Path: "secret/", Type: "kv", Version: 2}
	v1 := vault.Mount{Path: "legacy/", Type: "kv", Version: 1}

	got, err := c.ListSecrets(ctx, v2, "app/")
	if err != nil || !reflect.DeepEqual(got, []string{"db/", "web"}) {
		t.Errorf("v2 nested: got %v, %v", got, err)
	}
	got, err = c.ListSecrets(ctx, v2, "")
	if err != nil || !reflect.DeepEqual(got, []string{"app/"}) {
		t.Errorf("v2 root: got %v, %v", got, err)
	}
	got, err = c.ListSecrets(ctx, v1, "team/")
	if err != nil || !reflect.DeepEqual(got, []string{"x"}) {
		t.Errorf("v1: got %v, %v", got, err)
	}
	if _, err = c.ListSecrets(ctx, v2, "none/"); !errors.Is(err, vault.ErrPermissionDenied) {
		t.Errorf("want ErrPermissionDenied, got %v", err)
	}
	if got, err = c.ListSecrets(ctx, v2, "missing/"); err != nil || len(got) != 0 {
		t.Errorf("404 on list should be empty, got %v, %v", got, err)
	}
	if _, err = c.ListSecrets(
		ctx,
		vault.Mount{Path: "pki/", Type: "pki"},
		"",
	); !errors.Is(
		err,
		vault.ErrUnsupported,
	) {
		t.Errorf("want ErrUnsupported, got %v", err)
	}
}

func TestReadSecret(t *testing.T) {
	c := newTestClient(t, map[string]any{
		"GET secret/data/app/web": data(map[string]any{
			"data":     map[string]any{"user": "bob"},
			"metadata": map[string]any{"version": 3},
		}),
		"GET secret/data/deleted": data(map[string]any{
			"data":     nil,
			"metadata": map[string]any{"deletion_time": "2024-01-01T00:00:00Z"},
		}),
		"GET legacy/app": data(map[string]any{"password": "pw"}),
	})
	ctx := context.Background()
	v2 := vault.Mount{Path: "secret/", Type: "kv", Version: 2}
	v1 := vault.Mount{Path: "legacy/", Type: "kv", Version: 1}

	s, err := c.ReadSecret(ctx, v2, "app/web")
	if err != nil {
		t.Fatal(err)
	}
	if s.Path != "secret/app/web" || s.Data["user"] != "bob" || s.Metadata["version"] == nil {
		t.Errorf("unexpected v2 secret: %+v", s)
	}

	s, err = c.ReadSecret(ctx, v1, "app")
	if err != nil || s.Data["password"] != "pw" || s.Metadata != nil {
		t.Errorf("unexpected v1 secret: %+v, %v", s, err)
	}

	if _, err = c.ReadSecret(ctx, v2, "deleted"); !errors.Is(err, vault.ErrNotFound) {
		t.Errorf("deleted version: want ErrNotFound, got %v", err)
	}
	if _, err = c.ReadSecret(ctx, v2, "nope"); !errors.Is(err, vault.ErrNotFound) {
		t.Errorf("missing: want ErrNotFound, got %v", err)
	}
}

func TestPolicies(t *testing.T) {
	c := newTestClient(t, map[string]any{
		"LIST sys/policies/acl": data(map[string]any{"keys": []string{"root", "default"}}),
		"GET sys/policies/acl/default": data(map[string]any{
			"name": "default", "policy": `path "x" { capabilities = ["read"] }`,
		}),
		"GET sys/policies/acl/denied": 403,
	})
	ctx := context.Background()

	names, err := c.ListPolicies(ctx)
	if err != nil || !reflect.DeepEqual(names, []string{"default", "root"}) {
		t.Errorf("ListPolicies = %v, %v", names, err)
	}
	rules, err := c.ReadPolicy(ctx, "default")
	if err != nil || !strings.Contains(rules, "capabilities") {
		t.Errorf("ReadPolicy = %q, %v", rules, err)
	}
	if _, err = c.ReadPolicy(ctx, "denied"); !errors.Is(err, vault.ErrPermissionDenied) {
		t.Errorf("want ErrPermissionDenied, got %v", err)
	}
}

func TestAuth(t *testing.T) {
	c := newTestClient(t, map[string]any{
		"GET sys/auth": map[string]any{"data": map[string]any{
			"userpass/": map[string]any{
				"type":           "userpass",
				"accessor":       "auth_up",
				"description":    "users",
				"plugin_version": "v1",
			},
			"token/": map[string]any{
				"type":                   "token",
				"accessor":               "auth_tk",
				"running_plugin_version": "v2",
			},
			"saml/": map[string]any{"type": "saml", "accessor": "auth_sm"},
			"ldap/": map[string]any{"type": "ldap", "accessor": "auth_ld"},
		}},
		"LIST auth/userpass/users": keys("bob", "alice"),
		"GET auth/userpass/users/bob": data(map[string]any{
			"policies": []string{"default"},
		}),
		"LIST auth/ldap/users":  403,
		"LIST auth/ldap/groups": keys("admins"),
	})
	ctx := context.Background()

	methods, err := c.ListAuthMethods(ctx)
	if err != nil || len(methods) != 4 {
		t.Fatalf("ListAuthMethods = %v, %v", methods, err)
	}
	byPath := map[string]vault.AuthMount{}
	for _, m := range methods {
		byPath[m.Path] = m
	}
	if byPath["userpass/"].Version != "v1" || byPath["token/"].Version != "v2" {
		t.Errorf("versions wrong: %+v", byPath)
	}

	roles, err := c.ListAuthRoles(ctx, byPath["userpass/"])
	if err != nil || len(roles) != 2 || roles[0].Name != "alice" || roles[0].Kind != "users" {
		t.Fatalf("ListAuthRoles = %+v, %v", roles, err)
	}
	d, err := c.ReadAuthRole(ctx, roles[1])
	if err != nil || d["policies"] == nil {
		t.Errorf("ReadAuthRole = %v, %v", d, err)
	}

	// Partial failure: users forbidden but groups readable.
	roles, err = c.ListAuthRoles(ctx, byPath["ldap/"])
	if err != nil || len(roles) != 1 || roles[0].Kind != "groups" {
		t.Errorf("ldap roles = %+v, %v", roles, err)
	}

	if _, err = c.ListAuthRoles(ctx, byPath["saml/"]); !errors.Is(err, vault.ErrUnsupported) {
		t.Errorf("want ErrUnsupported, got %v", err)
	}
	if _, err = c.ReadAuthRole(
		ctx,
		vault.AuthRole{Mount: byPath["userpass/"], Kind: "bogus", Name: "x"},
	); !errors.Is(
		err,
		vault.ErrUnsupported,
	) {
		t.Errorf("want ErrUnsupported for unknown kind, got %v", err)
	}
	if _, err = c.ReadAuthRole(
		ctx,
		vault.AuthRole{Mount: byPath["userpass/"], Kind: "users", Name: "ghost"},
	); !errors.Is(
		err,
		vault.ErrNotFound,
	) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestCheckPathBlocksTraversal(t *testing.T) {
	c := newTestClient(t, nil)
	ctx := context.Background()
	v2 := vault.Mount{Path: "secret/", Type: "kv", Version: 2}
	bad := []string{"../sys/seal-status", "a/../../b", "..", "a/./b", "a\x00b", "a\nb"}
	for _, p := range bad {
		if _, err := c.ReadSecret(ctx, v2, p); !errors.Is(err, vault.ErrInvalidPath) {
			t.Errorf("ReadSecret(%q) err = %v, want ErrInvalidPath", p, err)
		}
		if _, err := c.ListSecrets(ctx, v2, p); !errors.Is(err, vault.ErrInvalidPath) {
			t.Errorf("ListSecrets(%q) err = %v, want ErrInvalidPath", p, err)
		}
		if _, err := c.ReadPolicy(ctx, p); !errors.Is(err, vault.ErrInvalidPath) {
			t.Errorf("ReadPolicy(%q) err = %v, want ErrInvalidPath", p, err)
		}
		role := vault.AuthRole{
			Mount: vault.AuthMount{Path: "userpass/", Type: "userpass"},
			Kind:  "users",
			Name:  p,
		}
		if _, err := c.ReadAuthRole(ctx, role); !errors.Is(err, vault.ErrInvalidPath) {
			t.Errorf("ReadAuthRole(%q) err = %v, want ErrInvalidPath", p, err)
		}
	}
	// Legitimate but unusual names stay readable.
	for _, p := range []string{"my secret", "100%", "what?", "a#b", "dots.in.name", "..hidden"} {
		if _, err := c.ReadSecret(ctx, v2, p); errors.Is(err, vault.ErrInvalidPath) {
			t.Errorf("ReadSecret(%q) wrongly rejected", p)
		}
	}
}

// A hostile server must not be able to obtain the token by redirecting the client.
func TestConnectDoesNotFollowRedirectsWithToken(t *testing.T) {
	var leaked atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Vault-Token") != "" {
			leaked.Store(true)
		}
	}))
	defer target.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", target.URL+r.URL.Path)
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer redirector.Close()

	c, err := vault.Connect(config.Config{Address: redirector.URL}, "s.secret-token")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = c.ListPolicies(context.Background())
	if leaked.Load() {
		t.Fatal("token was forwarded to the redirect target")
	}
}

// The ambient VAULT_AGENT_ADDR must not override the address the user sees.
func TestConnectIgnoresAmbientAgentAddress(t *testing.T) {
	var hit atomic.Bool
	agent := httptest.NewServer(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hit.Store(true) }),
	)
	defer agent.Close()
	t.Setenv("VAULT_AGENT_ADDR", agent.URL)

	var served atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		served.Store(true)
		_, _ = w.Write([]byte(`{"data":{"keys":[]}}`))
	}))
	defer srv.Close()

	c, err := vault.Connect(config.Config{Address: srv.URL}, "s.tok")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = c.ListPolicies(context.Background())
	if hit.Load() || !served.Load() {
		t.Fatalf(
			"request went to the wrong server (agent hit=%v, configured hit=%v)",
			hit.Load(),
			served.Load(),
		)
	}
}

func TestConnectErrors(t *testing.T) {
	if _, err := vault.Connect(
		config.Config{Address: "http://x", CACert: "/nonexistent/ca.pem"},
		"t",
	); err == nil {
		t.Error("missing CA file must fail")
	}
}
