// Package tui is the root Bubble Tea application.
package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/diablo02000/vault-tui/internal/tui/clip"
	"github.com/diablo02000/vault-tui/internal/tui/components"
	"github.com/diablo02000/vault-tui/internal/tui/keys"
	"github.com/diablo02000/vault-tui/internal/tui/views"
	"github.com/diablo02000/vault-tui/internal/vault"
)

// Info describes the connection, shown in the status bar.
type Info struct {
	Address     string
	Namespace   string
	TokenSource string
	Warnings    []string // insecure settings, shown prominently
}

func (i Info) String() string {
	var parts []string
	if len(i.Warnings) > 0 {
		parts = append(parts, "⚠ "+strings.Join(i.Warnings, ", "))
	}
	parts = append(parts, i.Address)
	if i.Namespace != "" {
		parts = append(parts, "ns:"+i.Namespace)
	}
	if i.TokenSource != "" {
		parts = append(parts, "token:"+i.TokenSource)
	}
	return strings.Join(parts, " · ")
}

var (
	tabStyle       = lipgloss.NewStyle().Padding(0, 2).Foreground(lipgloss.Color("244"))
	activeTabStyle = tabStyle.Foreground(lipgloss.Color("231")).
			Background(lipgloss.Color("63")).
			Bold(true)
)

var tabTitles = []string{"1 Secrets", "2 Policies", "3 Auth"}

// Model is the root Bubble Tea model.
type Model struct {
	keys   keys.KeyMap
	info   Info
	views  []views.View
	active int
	help   components.Help
	width  int
	height int
	clip   *clip.Clipboard
}

// Option customises the model.
type Option func(*options)

type options struct{ clipboard clip.Backend }

// WithClipboard replaces the system clipboard (used by tests).
func WithClipboard(b clip.Backend) Option { return func(o *options) { o.clipboard = b } }

// New builds the application model on top of a Vault reader.
func New(r vault.Reader, info Info, opts ...Option) Model {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	k := keys.Default()
	c := clip.New(o.clipboard)
	return Model{
		keys: k,
		info: info,
		clip: c,
		views: []views.View{
			views.NewSecretsView(r, k, c),
			views.NewPoliciesView(r, k),
			views.NewAuthView(r, k),
		},
		help: components.Help{Keys: k},
	}
}

func (m Model) Init() tea.Cmd {
	cmds := make([]tea.Cmd, len(m.views))
	for i, v := range m.views {
		cmds[i] = v.Init()
	}
	return tea.Batch(cmds...)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// Vault responses go to every view; anything else to the active one only.
	if views.IsLoadMsg(msg) {
		cmds := make([]tea.Cmd, len(m.views))
		for i, v := range m.views {
			m.views[i], cmds[i] = v.Update(msg)
		}
		return m, tea.Batch(cmds...)
	}
	return m, m.updateActive(msg)
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, m.quit()
	}
	if m.views[m.active].Capturing() {
		return m, m.updateActive(msg)
	}
	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, m.quit()
	case key.Matches(msg, m.keys.Help):
		m.help.Expanded = !m.help.Expanded
		m.layout()
	case key.Matches(msg, m.keys.NextTab):
		m.active = (m.active + 1) % len(m.views)
	case key.Matches(msg, m.keys.PrevTab):
		m.active = (m.active + len(m.views) - 1) % len(m.views)
	case key.Matches(msg, m.keys.Tab1):
		m.active = 0
	case key.Matches(msg, m.keys.Tab2):
		m.active = 1
	case key.Matches(msg, m.keys.Tab3):
		m.active = 2
	default:
		return m, m.updateActive(msg)
	}
	return m, nil
}

// quit wipes any secret still in the clipboard before leaving.
func (m Model) quit() tea.Cmd {
	_ = m.clip.Clear()
	return tea.Quit
}

func (m Model) updateActive(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	m.views[m.active], cmd = m.views[m.active].Update(msg)
	return cmd
}

// layout distributes the terminal height between tabs, body, status bar and help.
func (m *Model) layout() {
	m.help.Width = m.width
	helpLines := strings.Count(m.help.View(), "\n") + 1
	bodyHeight := max(m.height-1-1-helpLines, 1) // tabs + status bar + help
	for _, v := range m.views {
		v.Resize(m.width, bodyHeight)
	}
}

func (m Model) View() string {
	if m.width == 0 {
		return "vault-tui loading...\n"
	}
	tabs := make([]string, len(tabTitles))
	for i, t := range tabTitles {
		if i == m.active {
			tabs[i] = activeTabStyle.Render(t)
		} else {
			tabs[i] = tabStyle.Render(t)
		}
	}
	status := components.StatusBar{
		Path:  m.views[m.active].Path(),
		Info:  m.info.String(),
		Width: m.width,
	}
	return strings.Join([]string{
		lipgloss.JoinHorizontal(lipgloss.Top, tabs...),
		m.views[m.active].View(),
		status.View(),
		m.help.View(),
	}, "\n")
}
