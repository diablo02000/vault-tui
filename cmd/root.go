package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"

	"github.com/diablo02000/vault-tui/internal/config"
	"github.com/diablo02000/vault-tui/internal/tui"
	"github.com/diablo02000/vault-tui/internal/vault"
)

// Version is the application version. It is "dev" for local builds and is set
// at release time with -ldflags "-X github.com/diablo02000/vault-tui/cmd.Version=...".
var Version = "dev"

// NewApp builds the CLI application.
func NewApp() *cli.Command {
	return &cli.Command{
		Name:    "vault-tui",
		Usage:   "Terminal user interface to explore Vault secrets, policies and auth methods",
		Version: Version,
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "debug", Aliases: []string{"d"}, Usage: "Set log level to debug"},
			&cli.StringFlag{
				Name:    "address",
				Aliases: []string{"a"},
				Usage:   "Vault address (default: $VAULT_ADDR)",
			},
			&cli.StringFlag{
				Name:    "namespace",
				Aliases: []string{"n"},
				Usage:   "Vault namespace (default: $VAULT_NAMESPACE)",
			},
			&cli.StringFlag{
				Name:  "ca-cert",
				Usage: "Path to a CA certificate file (default: $VAULT_CACERT)",
			},
			&cli.BoolFlag{Name: "tls-skip-verify", Usage: "Skip TLS certificate verification"},
		},
		Action: run,
	}
}

// Execute runs the application and exits non-zero on failure.
func Execute() {
	if err := NewApp().Run(context.Background(), os.Args); err != nil {
		logrus.Error(err)
		os.Exit(1)
	}
}

func run(ctx context.Context, c *cli.Command) error {
	if c.Bool("debug") {
		logrus.SetLevel(logrus.DebugLevel)
	}

	cfg, err := loadConfig(c)
	if err != nil {
		return err
	}

	token, source, err := vault.ResolveToken(ctx, cfg.Token, vault.DefaultTokenDeps())
	if err != nil {
		return err
	}
	logrus.Debugf("using token from %s", source)

	client, err := vault.Connect(cfg, token)
	if err != nil {
		return err
	}

	warnings := cfg.Warnings()
	for _, w := range warnings {
		logrus.Warn(w)
	}
	info := tui.Info{
		Address:     cfg.Address,
		Namespace:   cfg.Namespace,
		TokenSource: source,
		Warnings:    warnings,
	}
	_, err = tea.NewProgram(tui.New(client, info), tea.WithAltScreen()).Run()
	return err
}

// loadConfig reads the environment, then applies command-line overrides.
func loadConfig(c *cli.Command) (config.Config, error) {
	cfg, err := config.Load(environ())
	if err != nil {
		return cfg, err
	}
	if c.IsSet("address") {
		cfg.Address = c.String("address")
	}
	if c.IsSet("namespace") {
		cfg.Namespace = c.String("namespace")
	}
	if c.IsSet("ca-cert") {
		cfg.CACert = c.String("ca-cert")
	}
	if c.IsSet("tls-skip-verify") {
		cfg.SkipTLSVerify = c.Bool("tls-skip-verify")
	}
	if err := cfg.Validate(); err != nil {
		return cfg, fmt.Errorf("configuration: %w", err)
	}
	return cfg, nil
}

func environ() map[string]string {
	env := map[string]string{}
	for _, kv := range os.Environ() {
		if k, v, ok := strings.Cut(kv, "="); ok {
			env[k] = v
		}
	}
	return env
}
