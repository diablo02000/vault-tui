package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"

	"github.com/caarlos0/env/v11"
)

// DefaultAddress is the Vault address used when VAULT_ADDR is not set.
const DefaultAddress = "https://127.0.0.1:8200"

// Config holds the runtime configuration for vault-tui.
type Config struct {
	Address       string `env:"VAULT_ADDR"        envDefault:"https://127.0.0.1:8200"`
	Token         string `env:"VAULT_TOKEN"`
	CACert        string `env:"VAULT_CACERT"`
	Namespace     string `env:"VAULT_NAMESPACE"`
	SkipTLSVerify bool   `env:"VAULT_SKIP_VERIFY"`
}

// Load builds a Config from the given environment (name -> value).
// Pass the result of os.Environ() converted with EnvironMap for the real environment.
func Load(environment map[string]string) (Config, error) {
	var cfg Config
	if err := env.ParseWithOptions(&cfg, env.Options{Environment: environment}); err != nil {
		return Config{}, fmt.Errorf("parse environment: %w", err)
	}
	return cfg, nil
}

// Validate checks that the configuration is usable.
func (c Config) Validate() error {
	u, err := url.Parse(c.Address)
	if err != nil {
		return fmt.Errorf("invalid vault address %q: %w", c.Address, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("invalid vault address %q: scheme must be http or https", c.Address)
	}
	if u.Host == "" {
		return errors.New("invalid vault address: missing host")
	}
	// Credentials in the URL would be displayed in the UI and end up in logs.
	if u.User != nil {
		return errors.New("invalid vault address: credentials in the URL are not allowed")
	}
	return nil
}

// Warnings lists insecure settings the user should be made aware of.
// Call it on a validated Config.
func (c Config) Warnings() []string {
	var w []string
	if c.SkipTLSVerify {
		w = append(w, "TLS verification disabled")
	}
	if u, err := url.Parse(
		c.Address,
	); err == nil && u.Scheme == "http" &&
		!isLoopback(u.Hostname()) {
		w = append(w, "token sent over plain HTTP")
	}
	return w
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
