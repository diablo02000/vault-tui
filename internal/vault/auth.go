package vault

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
)

// Token sources, reported so the UI can tell the user where the token came from.
const (
	SourceFlagOrEnv    = "VAULT_TOKEN"
	SourceTokenHelper  = "token helper"
	SourceTokenFile    = "~/.vault-token" //nolint:gosec // a file name, not a credential
	vaultConfigFile    = ".vault"
	vaultTokenFile     = ".vault-token"
	envVaultConfigPath = "VAULT_CONFIG_PATH"
)

// ErrNoToken is returned when no Vault token could be found.
var ErrNoToken = errors.New(
	"no vault token found: set VAULT_TOKEN, configure a token_helper in ~/.vault, or run `vault login`",
)

const (
	maxFileSize        = 64 << 10 // config and token files are tiny; never slurp anything bigger
	maxTokenLength     = 4096
	tokenHelperTimeout = 10 * time.Second
)

var tokenHelperRe = regexp.MustCompile(`(?m)^\s*token_helper\s*=\s*"([^"]*)"`)

// TokenDeps holds the side effects needed to resolve a token, so they can be faked in tests.
type TokenDeps struct {
	Getenv   func(string) string
	HomeDir  func() (string, error)
	ReadFile func(string) ([]byte, error)
	Run      func(ctx context.Context, name string, args ...string) ([]byte, error)
}

// DefaultTokenDeps returns dependencies backed by the real OS.
func DefaultTokenDeps() TokenDeps {
	return TokenDeps{
		Getenv:   os.Getenv,
		HomeDir:  os.UserHomeDir,
		ReadFile: readFileLimited,
		Run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			// The helper path comes from the user's own ~/.vault config.
			cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // see above
			return cmd.Output()
		},
	}
}

// readFileLimited reads at most maxFileSize bytes and refuses anything that is
// not a regular file (a FIFO or /dev/zero symlinked as ~/.vault-token would
// otherwise hang or exhaust memory).
func readFileLimited(name string) ([]byte, error) {
	f, err := os.Open(name) //nolint:gosec // path derived from the user's home / VAULT_CONFIG_PATH
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", name)
	}
	data, err := io.ReadAll(io.LimitReader(f, maxFileSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxFileSize {
		return nil, fmt.Errorf("%s is larger than %d bytes", name, maxFileSize)
	}
	return data, nil
}

// validToken rejects values that cannot be a Vault token and could be used for
// header injection: control characters, inner whitespace, absurd length.
func validToken(t string) error {
	if len(t) > maxTokenLength {
		return errors.New("token is too long")
	}
	for _, r := range t {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return errors.New("token contains whitespace or control characters")
		}
	}
	return nil
}

// ResolveToken finds a Vault token, mirroring the Vault CLI: an explicit token
// (VAULT_TOKEN) wins, then the configured token helper, then ~/.vault-token.
// It returns the token and a description of where it came from.
func ResolveToken(ctx context.Context, explicit string, deps TokenDeps) (string, string, error) {
	token, source, err := resolveToken(ctx, explicit, deps)
	if err != nil {
		return "", "", err
	}
	if err := validToken(token); err != nil {
		return "", "", fmt.Errorf("token from %s: %w", source, err)
	}
	return token, source, nil
}

func resolveToken(ctx context.Context, explicit string, deps TokenDeps) (string, string, error) {
	if t := strings.TrimSpace(explicit); t != "" {
		return t, SourceFlagOrEnv, nil
	}

	home, err := deps.HomeDir()
	if err != nil {
		return "", "", fmt.Errorf("find home directory: %w", err)
	}

	helper, err := tokenHelperCommand(home, deps)
	if err != nil {
		return "", "", err
	}

	if helper != "" {
		token, err := runTokenHelper(ctx, helper, deps)
		if err != nil {
			return "", "", err
		}
		if token == "" {
			return "", "", ErrNoToken
		}
		return token, SourceTokenHelper, nil
	}

	data, err := deps.ReadFile(filepath.Join(home, vaultTokenFile))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", "", ErrNoToken
		}
		return "", "", fmt.Errorf("read token file: %w", err)
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", "", ErrNoToken
	}
	return token, SourceTokenFile, nil
}

// tokenHelperCommand returns the token_helper configured in the Vault CLI config, or "".
func tokenHelperCommand(home string, deps TokenDeps) (string, error) {
	path := deps.Getenv(envVaultConfigPath)
	if path == "" {
		path = filepath.Join(home, vaultConfigFile)
	}
	data, err := deps.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("read vault config %s: %w", path, err)
	}
	m := tokenHelperRe.FindSubmatch(data)
	if m == nil {
		return "", nil
	}
	return strings.TrimSpace(string(m[1])), nil
}

func runTokenHelper(ctx context.Context, helper string, deps TokenDeps) (string, error) {
	fields := strings.Fields(helper)
	if len(fields) == 0 {
		return "", nil
	}
	ctx, cancel := context.WithTimeout(ctx, tokenHelperTimeout)
	defer cancel()
	args := append(slices.Clone(fields[1:]), "get")
	out, err := deps.Run(ctx, fields[0], args...)
	if err != nil {
		return "", fmt.Errorf("token helper %q failed: %w", helper, err)
	}
	return strings.TrimSpace(string(out)), nil
}
