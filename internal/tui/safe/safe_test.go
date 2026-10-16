package safe_test

import (
	"strings"
	"testing"

	"github.com/diablo02000/vault-tui/internal/tui/safe"
)

func TestLine(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"plain", "secret/app", "secret/app"},
		{"unicode kept", "café 日本 🔑", "café 日本 🔑"},
		{"ansi escape", "\x1b[2J\x1b[Hpwned", "�[2J�[Hpwned"},
		{"osc title", "\x1b]0;evil\x07", "�]0;evil�"},
		{"c1 csi", "a\u009bb", "a�b"},
		{"newline spoof", "pw\npassword  admin", `pw\npassword  admin`},
		{"tab and cr", "a\tb\rc", `a\tb\rc`},
		{"bidi override", "good\u202edab", "good�dab"},
		{"nul", "a\x00b", "a�b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := safe.Line(tt.in); got != tt.want {
				t.Errorf("Line(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestText(t *testing.T) {
	got := safe.Text("path \"x\" {\n\tcapabilities = [\"read\"]\x1b[31m\r\n}")
	if !strings.Contains(got, "\n    capabilities") {
		t.Errorf("newlines/tabs should be preserved or expanded: %q", got)
	}
	if strings.ContainsAny(got, "\x1b\r\t") {
		t.Errorf("control characters survived: %q", got)
	}
}

func FuzzLineNeverEmitsControls(f *testing.F) {
	for _, s := range []string{"", "a", "\x1b[0m", "\u202e", "a\nb"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		for _, r := range safe.Line(s) {
			if r < 0x20 || (r >= 0x7f && r <= 0x9f) {
				t.Fatalf("control rune %U in output for input %q", r, s)
			}
		}
	})
}
