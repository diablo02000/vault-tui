package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/diablo02000/vault-tui/internal/tui/safe"
)

var (
	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Background(lipgloss.Color("236"))
	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("231")).
			Background(lipgloss.Color("160")).
			Bold(true)
)

// StatusBar renders the bottom status line: the current path and connection info.
type StatusBar struct {
	Path  string
	Info  string // e.g. address, namespace, token source
	Width int
}

// View renders the bar padded to the full width.
func (s StatusBar) View() string {
	left := " " + safe.Line(s.Path)
	right := safe.Line(s.Info) + " "
	gap := s.Width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return statusStyle.Render(left + strings.Repeat(" ", gap) + right)
}

// ErrorLine renders an error message in the status style.
func ErrorLine(msg string, width int) string {
	return errorStyle.Width(width).Render(" " + safe.Line(msg))
}
