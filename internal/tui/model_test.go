package tui_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/diablo02000/vault-tui/internal/tui"
	"github.com/diablo02000/vault-tui/internal/vault"
	"github.com/diablo02000/vault-tui/internal/vault/vaulttest"
)

func fakeVault() *vaulttest.Fake {
	kv2 := vault.Mount{Path: "secret/", Type: "kv", Version: 2, Description: "app secrets"}
	return &vaulttest.Fake{
		Mounts: []vault.Mount{kv2, {Path: "pki/", Type: "pki"}},
		Keys: map[string][]string{
			"secret/":     {"app/", "top"},
			"secret/app/": {"db"},
		},
		Secrets: map[string]*vault.Secret{
			"secret/app/db": {
				Path:     "secret/app/db",
				Data:     map[string]any{"password": "hunter2"},
				Metadata: map[string]any{"version": 2},
			},
		},
		Policies: map[string]string{
			"default": `path "sys/*" { capabilities = ["read"] }`,
			"admin":   `path "*" { capabilities = ["sudo"] }`,
		},
		AuthMethods: []vault.AuthMount{
			{
				Path:        "userpass/",
				Type:        "userpass",
				Accessor:    "auth_userpass_1",
				Description: "humans",
			},
			{Path: "saml/", Type: "saml", Accessor: "auth_saml_1"},
		},
		Roles: map[string][]vault.AuthRole{
			"userpass/": {
				{
					Mount: vault.AuthMount{Path: "userpass/", Type: "userpass"},
					Kind:  "users",
					Name:  "bob",
				},
			},
		},
		RoleData: map[string]map[string]any{
			"userpass/users/bob": {"token_policies": []any{"default"}},
		},
	}
}

// driver feeds messages to the model and runs the resulting commands synchronously.
type driver struct {
	t *testing.T
	m tea.Model
}

func newDriver(t *testing.T, r vault.Reader) *driver {
	t.Helper()
	d := &driver{
		t: t,
		m: tui.New(r, tui.Info{Address: "http://vault:8200", TokenSource: "VAULT_TOKEN"}),
	}
	d.send(tea.WindowSizeMsg{Width: 100, Height: 30})
	d.run(d.m.Init())
	return d
}

func (d *driver) send(msg tea.Msg) {
	var cmd tea.Cmd
	d.m, cmd = d.m.Update(msg)
	d.run(cmd)
}

func (d *driver) run(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	// Timer commands (status timeouts) block; give each command a short window
	// and drop it if it has not produced a message by then.
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	var result tea.Msg
	select {
	case result = <-done:
	case <-time.After(100 * time.Millisecond):
		return
	}
	if strings.HasPrefix(fmt.Sprintf("%T", result), "cursor.") {
		return // blink timers re-arm themselves forever
	}
	switch msg := result.(type) {
	case nil:
	case tea.BatchMsg:
		for _, c := range msg {
			d.run(c)
		}
	default:
		d.send(msg)
	}
}

func (d *driver) key(s string) {
	switch s {
	case "enter":
		d.send(tea.KeyMsg{Type: tea.KeyEnter})
	case "esc":
		d.send(tea.KeyMsg{Type: tea.KeyEsc})
	case "tab":
		d.send(tea.KeyMsg{Type: tea.KeyTab})
	case "shift+tab":
		d.send(tea.KeyMsg{Type: tea.KeyShiftTab})
	default:
		d.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)})
	}
}

func (d *driver) view() string { return d.m.View() }

func (d *driver) wantView(substrs ...string) {
	d.t.Helper()
	v := d.view()
	for _, s := range substrs {
		if !strings.Contains(v, s) {
			d.t.Errorf("view missing %q:\n%s", s, v)
		}
	}
}

func (d *driver) wantNotView(substrs ...string) {
	d.t.Helper()
	v := d.view()
	for _, s := range substrs {
		if strings.Contains(v, s) {
			d.t.Errorf("view must not contain %q:\n%s", s, v)
		}
	}
}

func TestViewBeforeResize(t *testing.T) {
	m := tui.New(&vaulttest.Fake{}, tui.Info{})
	if m.View() == "" {
		t.Error("View() should not return an empty string")
	}
}

func TestInitialScreen(t *testing.T) {
	d := newDriver(t, fakeVault())
	d.wantView(
		"1 Secrets",
		"2 Policies",
		"3 Auth",
		"secret/",
		"pki/",
		"http://vault:8200",
		"token:VAULT_TOKEN",
	)
}

func TestTabSwitching(t *testing.T) {
	d := newDriver(t, fakeVault())

	d.key("2")
	d.wantView("admin", "default")
	d.key("3")
	d.wantView("userpass/", "auth_userpass_1", "saml/")
	d.key("tab")
	d.wantView("secret/") // wraps back to the first tab
	d.key("shift+tab")
	d.wantView("userpass/") // and backwards
}

func TestSecretsNavigation(t *testing.T) {
	d := newDriver(t, fakeVault())

	d.key("enter") // secret/
	d.wantView("app/", "top", "folder")
	d.key("enter") // app/
	d.wantView("db")
	d.wantView("secret/app/") // breadcrumb
	d.key("enter")            // db
	d.wantView("password", "********", "version")
	d.wantNotView("hunter2")

	d.key("v")
	d.wantView("hunter2")
	d.key("v")
	d.wantNotView("hunter2")

	d.key("esc") // back to secret/app/
	d.wantView("db")
	d.key("esc") // back to secret/
	d.wantView("app/", "top")
	d.key("esc") // back to mounts
	d.wantView("pki/")
}

func TestSecretsNonKVMountIsNotBrowsable(t *testing.T) {
	d := newDriver(t, fakeVault())
	d.key("j") // pki/ (second mount)
	d.key("enter")
	d.wantView("only KV engines")
	d.wantView("pki/") // still on the mounts list
}

func TestPolicyDetail(t *testing.T) {
	d := newDriver(t, fakeVault())
	d.key("2")
	d.key("enter") // admin (sorted first)
	d.wantView("sudo", "policies/admin")
	d.key("esc")
	d.wantView("default")
	d.wantNotView("sudo")
}

func TestAuthExploration(t *testing.T) {
	d := newDriver(t, fakeVault())
	d.key("3")
	d.key("enter") // userpass/ (first method)
	d.wantView("bob", "users")
	d.key("enter")
	d.wantView("token_policies", "default", "auth/userpass/users/bob")
	d.key("esc")
	d.wantView("bob")
	d.key("esc")
	d.wantView("saml/")
}

func TestErrorsAreDisplayed(t *testing.T) {
	d := newDriver(t, &vaulttest.Fake{Err: vault.ErrPermissionDenied})
	d.wantView("permission denied")
	d.key("2")
	d.wantView("permission denied")
	d.key("3")
	d.wantView("permission denied")
}

func TestFilterCapturesKeys(t *testing.T) {
	d := newDriver(t, fakeVault())
	d.key("2")
	d.key("/")
	d.key("q") // typed into the filter, must not quit
	d.key("2") // must not switch tabs either
	d.wantView("2 Policies")
	d.wantNotView("admin") // filter "q2" matches nothing

	d.send(tea.KeyMsg{Type: tea.KeyEsc})
	d.wantView("admin", "default")
}

func TestQuit(t *testing.T) {
	d := newDriver(t, fakeVault())
	_, cmd := d.m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil || cmd() != tea.Quit() {
		t.Error("q should quit")
	}
	_, cmd = d.m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil || cmd() != tea.Quit() {
		t.Error("ctrl+c should quit")
	}
}

func TestHelpToggle(t *testing.T) {
	d := newDriver(t, fakeVault())
	d.wantNotView("shift+tab")
	d.key("?")
	d.wantView("shift+tab", "reveal values")
	d.key("?")
	d.wantNotView("shift+tab")
}

func TestRefreshReloadsData(t *testing.T) {
	f := fakeVault()
	d := newDriver(t, f)
	d.key("2")
	f.Policies["new-policy"] = `path "x" {}`
	d.wantNotView("new-policy")
	d.key("r")
	d.wantView("new-policy")
}

func TestErrorThenRecovery(t *testing.T) {
	f := fakeVault()
	d := newDriver(t, f)
	f.Err = errors.New("boom")
	d.key("r") // fails
	d.wantView("boom")
	f.Err = nil
	d.key("r") // recovers
	d.wantNotView("boom")
}

// A hostile server controls every string below; none may reach the terminal raw.
func TestHostileServerDataIsNeutralised(t *testing.T) {
	const esc = "\x1b[2J\x1b]0;pwned\x07"
	kv := vault.Mount{Path: "secret/", Type: "kv", Version: 2, Description: "d" + esc}
	f := &vaulttest.Fake{
		Mounts: []vault.Mount{kv},
		Keys:   map[string][]string{"secret/": {"evil" + esc}},
		Secrets: map[string]*vault.Secret{
			"secret/evil" + esc: {
				Path: "secret/evil" + esc,
				Data: map[string]any{
					"k" + esc:   "v" + esc,
					"multiline": "line1\npassword  spoofed",
				},
				Metadata: map[string]any{"m": esc},
			},
		},
		Policies: map[string]string{
			"p" + esc: "path \"x\" {\n\tcapabilities = [\"read\"" + esc + "]\n}",
		},
		AuthMethods: []vault.AuthMount{
			{Path: "a/", Type: "userpass", Accessor: "acc" + esc, Description: esc},
		},
		Roles: map[string][]vault.AuthRole{
			"a/": {
				{
					Mount: vault.AuthMount{Path: "a/", Type: "userpass"},
					Kind:  "users",
					Name:  "r" + esc,
				},
			},
		},
		RoleData: map[string]map[string]any{"a/users/r" + esc: {"x": esc}},
	}
	d := newDriver(t, f)

	assertClean := func(step string) {
		t.Helper()
		// The test environment has no colour profile, so any ESC/BEL/C1 byte in
		// the output can only have come from the server data.
		for _, r := range d.view() {
			if r == 0x1b || r == 0x07 || (r >= 0x80 && r <= 0x9f) {
				t.Fatalf("%s: raw control character %U in view:\n%q", step, r, d.view())
			}
		}
	}

	assertClean("mounts")
	d.key("enter")
	assertClean("keys")
	d.key("enter")
	assertClean("secret masked")
	d.key("v")
	assertClean("secret revealed")
	d.wantView(`line1\npassword  spoofed`) // newline is shown literally, cannot fake a row
	d.wantNotView("\npassword  spoofed")

	d.key("2")
	assertClean("policies list")
	d.key("enter")
	assertClean("policy detail")

	d.key("3")
	assertClean("auth methods")
	d.key("enter")
	assertClean("auth roles")
	d.key("enter")
	assertClean("auth role detail")
}

func TestErrorTextIsNeutralised(t *testing.T) {
	d := newDriver(t, &vaulttest.Fake{Err: errors.New("boom\x1b[2J")})
	if strings.ContainsRune(d.view(), 0x1b) {
		t.Errorf("raw ESC in error line: %q", d.view())
	}
}

func TestSecretIsForgottenWhenLeavingDetail(t *testing.T) {
	d := newDriver(t, fakeVault())
	d.key("enter")
	d.key("enter")
	d.key("enter")
	d.key("v")
	d.wantView("hunter2")
	d.key("esc")
	d.wantNotView("hunter2")
	d.key("enter") // reopen: must be masked again
	d.wantView("********")
	d.wantNotView("hunter2")
}

func TestInfoWarningsAreShown(t *testing.T) {
	m := tui.New(
		&vaulttest.Fake{},
		tui.Info{
			Address:  "http://vault.example.com",
			Warnings: []string{"token sent over plain HTTP"},
		},
	)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	if !strings.Contains(next.View(), "token sent over plain HTTP") {
		t.Errorf("warning not visible:\n%s", next.View())
	}
}
