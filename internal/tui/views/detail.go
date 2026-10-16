package views

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/diablo02000/vault-tui/internal/tui/safe"
)

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	keyStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	dimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
)

const maskedValue = "********"

// Detail is a scrollable text pane used for secrets, policies and role details.
type Detail struct {
	vp    viewport.Model
	title string
}

func newDetail() Detail { return Detail{vp: viewport.New(0, 0)} }

func (d *Detail) resize(w, h int) {
	d.vp.Width = w
	d.vp.Height = max(h-1, 1) // one line for the title
}

func (d *Detail) set(title, content string) {
	d.title = safe.Line(title)
	d.vp.SetContent(content)
	d.vp.GotoTop()
}

func (d *Detail) update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	d.vp, cmd = d.vp.Update(msg)
	return cmd
}

func (d Detail) view() string {
	return titleStyle.Render(d.title) + "\n" + d.vp.View()
}

// renderMap renders sorted "key  value" lines. When mask is true, values are hidden.
func renderMap(m map[string]any, mask bool) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for _, k := range keys {
		v := maskedValue
		if !mask {
			v = formatValue(m[k])
		}
		fmt.Fprintf(&b, "%s  %s\n", keyStyle.Render(safe.Line(k)), safe.Line(v))
	}
	return b.String()
}

func formatValue(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case nil:
		return "<nil>"
	case map[string]any, []any:
		if out, err := json.Marshal(t); err == nil {
			return string(out)
		}
	}
	return fmt.Sprint(v)
}
