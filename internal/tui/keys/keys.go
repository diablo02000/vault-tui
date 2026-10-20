// Package keys defines the key bindings shared by the TUI components.
package keys

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines all key bindings for the application.
type KeyMap struct {
	Up      key.Binding
	Down    key.Binding
	Select  key.Binding
	Back    key.Binding
	Quit    key.Binding
	Help    key.Binding
	NextTab key.Binding
	PrevTab key.Binding
	Tab1    key.Binding
	Tab2    key.Binding
	Tab3    key.Binding
	Refresh key.Binding
	Reveal  key.Binding
	Copy    key.Binding
	Search  key.Binding
	Filter  key.Binding
}

// Default returns the default key bindings.
func Default() KeyMap {
	return KeyMap{
		Up:      key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:    key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Select:  key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		Back:    key.NewBinding(key.WithKeys("backspace", "esc"), key.WithHelp("esc", "back")),
		Quit:    key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		Help:    key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		NextTab: key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next tab")),
		PrevTab: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev tab")),
		Tab1:    key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "secrets")),
		Tab2:    key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "policies")),
		Tab3:    key.NewBinding(key.WithKeys("3"), key.WithHelp("3", "auth")),
		Refresh: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		Reveal:  key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "reveal values")),
		Copy:    key.NewBinding(key.WithKeys("y", "c"), key.WithHelp("y", "copy value")),
		Search:  key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "search secrets")),
		Filter:  key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
	}
}

// ShortHelp implements help.KeyMap.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Select, k.Back, k.Filter, k.NextTab, k.Help, k.Quit}
}

// FullHelp implements help.KeyMap.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Select, k.Back},
		{k.Filter, k.Search, k.Refresh, k.Reveal, k.Copy},
		{k.NextTab, k.PrevTab, k.Tab1, k.Tab2, k.Tab3},
		{k.Help, k.Quit},
	}
}
