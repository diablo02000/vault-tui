package vault_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/diablo02000/vault-tui/internal/vault"
)

type fakeFS map[string]string

func (f fakeFS) read(name string) ([]byte, error) {
	if c, ok := f[name]; ok {
		return []byte(c), nil
	}
	return nil, fs.ErrNotExist
}

func deps(
	files fakeFS,
	env map[string]string,
	run func(string, ...string) ([]byte, error),
) vault.TokenDeps {
	return vault.TokenDeps{
		Getenv:   func(k string) string { return env[k] },
		HomeDir:  func() (string, error) { return "/fakehome", nil },
		ReadFile: files.read,
		Run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			return run(name, args...)
		},
	}
}

func noRun(string, ...string) ([]byte, error) { panic("helper must not run") }

func TestResolveToken(t *testing.T) {
	tests := []struct {
		name       string
		explicit   string
		files      fakeFS
		env        map[string]string
		run        func(string, ...string) ([]byte, error)
		wantToken  string
		wantSource string
		wantErr    error
	}{
		{
			name:       "explicit token wins",
			explicit:   " s.env \n",
			files:      fakeFS{"/fakehome/.vault-token": "s.file"},
			run:        noRun,
			wantToken:  "s.env",
			wantSource: vault.SourceFlagOrEnv,
		},
		{
			name:       "token file fallback",
			files:      fakeFS{"/fakehome/.vault-token": "s.file\n"},
			run:        noRun,
			wantToken:  "s.file",
			wantSource: vault.SourceTokenFile,
		},
		{
			name:    "nothing available",
			files:   fakeFS{},
			run:     noRun,
			wantErr: vault.ErrNoToken,
		},
		{
			name:    "empty token file",
			files:   fakeFS{"/fakehome/.vault-token": "  \n"},
			run:     noRun,
			wantErr: vault.ErrNoToken,
		},
		{
			name: "token helper from ~/.vault",
			files: fakeFS{
				"/fakehome/.vault":       "# config\ntoken_helper = \"/usr/bin/my-helper --flag\"\n",
				"/fakehome/.vault-token": "s.file",
			},
			run: func(name string, args ...string) ([]byte, error) {
				if name != "/usr/bin/my-helper" ||
					!reflect.DeepEqual(args, []string{"--flag", "get"}) {
					t.Errorf("unexpected helper call: %s %v", name, args)
				}
				return []byte("s.helper\n"), nil
			},
			wantToken:  "s.helper",
			wantSource: vault.SourceTokenHelper,
		},
		{
			name:  "token helper via VAULT_CONFIG_PATH",
			files: fakeFS{"/etc/vault.hcl": `token_helper = "helper"`},
			env:   map[string]string{"VAULT_CONFIG_PATH": "/etc/vault.hcl"},
			run: func(string, ...string) ([]byte, error) {
				return []byte("s.custom"), nil
			},
			wantToken:  "s.custom",
			wantSource: vault.SourceTokenHelper,
		},
		{
			name: "helper failure is not masked by token file",
			files: fakeFS{
				"/fakehome/.vault":       `token_helper = "helper"`,
				"/fakehome/.vault-token": "s.file",
			},
			run: func(string, ...string) ([]byte, error) {
				return nil, errors.New("boom")
			},
			wantErr: errors.New("boom"),
		},
		{
			name:    "helper returns empty token",
			files:   fakeFS{"/fakehome/.vault": `token_helper = "helper"`},
			run:     func(string, ...string) ([]byte, error) { return []byte("\n"), nil },
			wantErr: vault.ErrNoToken,
		},
		{
			name: "config without token_helper uses token file",
			files: fakeFS{
				"/fakehome/.vault":       `disable_mlock = true`,
				"/fakehome/.vault-token": "s.file",
			},
			run:        noRun,
			wantToken:  "s.file",
			wantSource: vault.SourceTokenFile,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, source, err := vault.ResolveToken(
				context.Background(),
				tt.explicit,
				deps(tt.files, tt.env, tt.run),
			)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error, got token %q", token)
				}
				if errors.Is(tt.wantErr, vault.ErrNoToken) && !errors.Is(err, vault.ErrNoToken) {
					t.Fatalf("error = %v, want ErrNoToken", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if token != tt.wantToken || source != tt.wantSource {
				t.Errorf("got (%q, %q), want (%q, %q)", token, source, tt.wantToken, tt.wantSource)
			}
		})
	}
}

func TestResolveTokenRejectsMalformedTokens(t *testing.T) {
	bad := map[string]string{
		"inner newline":  "s.abc\nX-Evil: 1",
		"inner space":    "s.abc def",
		"control char":   "s.abc\x1b[31m",
		"too long":       strings.Repeat("a", 5000),
		"tab in the mid": "s.a\tb",
	}
	for name, tok := range bad {
		t.Run(name, func(t *testing.T) {
			_, _, err := vault.ResolveToken(context.Background(), tok, deps(fakeFS{}, nil, noRun))
			if err == nil {
				t.Fatalf("token %q must be rejected", tok)
			}
			if strings.Contains(err.Error(), tok) {
				t.Errorf("error leaks the token: %v", err)
			}
		})
	}
}

func TestResolveTokenHelperHasTimeout(t *testing.T) {
	files := fakeFS{"/fakehome/.vault": `token_helper = "helper"`}
	d := deps(files, nil, nil)
	d.Run = func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
		if _, ok := ctx.Deadline(); !ok {
			t.Error("token helper must run with a deadline")
		}
		return []byte("s.ok"), nil
	}
	if _, _, err := vault.ResolveToken(context.Background(), "", d); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultTokenDepsRefuseNonRegularAndHugeFiles(t *testing.T) {
	dir := t.TempDir()
	deps := vault.DefaultTokenDeps()

	small := filepath.Join(dir, "ok")
	if err := os.WriteFile(small, []byte("s.tok\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if data, err := deps.ReadFile(small); err != nil || string(data) != "s.tok\n" {
		t.Errorf("small file: %q, %v", data, err)
	}

	huge := filepath.Join(dir, "huge")
	if err := os.WriteFile(huge, make([]byte, 1<<20), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := deps.ReadFile(huge); err == nil {
		t.Error("oversized file must be refused")
	}

	if _, err := deps.ReadFile(dir); err == nil {
		t.Error("directory must be refused")
	}
	if _, err := deps.ReadFile("/dev/zero"); err == nil {
		t.Error("device file must be refused")
	}
	if _, err := deps.ReadFile(filepath.Join(dir, "missing")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("missing file should wrap ErrNotExist, got %v", err)
	}
}
