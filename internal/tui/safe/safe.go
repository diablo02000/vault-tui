// Package safe neutralises untrusted text (anything that comes from the Vault
// server) before it reaches the terminal. Raw control characters would let a
// malicious secret, policy name or error message inject escape sequences:
// rewrite the screen, set the window title, or spoof other rows.
package safe

import (
	"strings"
	"unicode"
)

const replacement = '�'

// Line returns s as a single printable line: control characters are replaced,
// and newlines and tabs are made visible as the two characters \n and \t.
func Line(s string) string {
	return clean(s, false)
}

// Text is like Line but keeps real newlines and expands tabs; use it for
// multi-line documents such as policies.
func Text(s string) string {
	return clean(s, true)
}

func clean(s string, multiline bool) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\n':
			if multiline {
				b.WriteRune(r)
			} else {
				b.WriteString(`\n`)
			}
		case r == '\t':
			if multiline {
				b.WriteString("    ")
			} else {
				b.WriteString(`\t`)
			}
		case r == '\r':
			if !multiline {
				b.WriteString(`\r`)
			}
		case unicode.IsControl(r), isBidi(r):
			b.WriteRune(replacement)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// isBidi reports Unicode bidirectional overrides/isolates, which can visually
// reorder text (e.g. make "evil" look like "live").
func isBidi(r rune) bool {
	return (r >= 0x202A && r <= 0x202E) || (r >= 0x2066 && r <= 0x2069) || r == 0x200E ||
		r == 0x200F ||
		r == 0x061C
}
