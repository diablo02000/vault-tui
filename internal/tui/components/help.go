package components

import (
	"github.com/charmbracelet/bubbles/help"

	"github.com/diablo02000/vault-tui/internal/tui/keys"
)

// Help renders the key bindings help bar, optionally expanded.
type Help struct {
	Expanded bool
	Keys     keys.KeyMap
	Width    int
}

// View renders the help line(s).
func (h Help) View() string {
	m := help.New()
	m.Width = h.Width
	m.ShowAll = h.Expanded
	return m.View(h.Keys)
}
