package config_test

import (
	"testing"

	"github.com/diablo02000/vault-tui/internal/config"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want config.Config
	}{
		{
			name: "defaults",
			env:  map[string]string{},
			want: config.Config{Address: config.DefaultAddress},
		},
		{
			name: "all variables",
			env: map[string]string{
				"VAULT_ADDR":        "http://vault.local:8200",
				"VAULT_TOKEN":       "s.abc",
				"VAULT_CACERT":      "/etc/ca.pem",
				"VAULT_NAMESPACE":   "team-a",
				"VAULT_SKIP_VERIFY": "true",
			},
			want: config.Config{
				Address:       "http://vault.local:8200",
				Token:         "s.abc",
				CACert:        "/etc/ca.pem",
				Namespace:     "team-a",
				SkipTLSVerify: true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := config.Load(tt.env)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Load() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestLoadInvalidBool(t *testing.T) {
	if _, err := config.Load(map[string]string{"VAULT_SKIP_VERIFY": "maybe"}); err == nil {
		t.Fatal("expected error for invalid VAULT_SKIP_VERIFY")
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		addr    string
		wantErr bool
	}{
		{"https://vault.example.com:8200", false},
		{"http://127.0.0.1:8200", false},
		{"vault.example.com:8200", true},
		{"ftp://vault.example.com", true},
		{"https://", true},
		{"https://user:pass@vault.example.com", true},
		{"https://user@vault.example.com", true},
		{"://bad", true},
	}
	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			err := config.Config{Address: tt.addr}.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestWarnings(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.Config
		want int
	}{
		{"https is fine", config.Config{Address: "https://vault.example.com"}, 0},
		{"http loopback ip", config.Config{Address: "http://127.0.0.1:8200"}, 0},
		{"http ipv6 loopback", config.Config{Address: "http://[::1]:8200"}, 0},
		{"http localhost", config.Config{Address: "http://localhost:8200"}, 0},
		{"http remote", config.Config{Address: "http://vault.example.com"}, 1},
		{
			"skip verify",
			config.Config{Address: "https://vault.example.com", SkipTLSVerify: true},
			1,
		},
		{"both", config.Config{Address: "http://10.0.0.5", SkipTLSVerify: true}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.Warnings(); len(got) != tt.want {
				t.Errorf("Warnings() = %v, want %d warning(s)", got, tt.want)
			}
		})
	}
}
