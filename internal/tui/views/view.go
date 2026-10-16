// Package views contains the three explorer tabs of the TUI.
package views

import (
	"context"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/diablo02000/vault-tui/internal/tui/components"
	"github.com/diablo02000/vault-tui/internal/tui/keys"
	"github.com/diablo02000/vault-tui/internal/tui/safe"
)

// loadMsg marks the asynchronous Vault responses, which every view must receive
// even when inactive. Any other message belongs to the active view only
// (e.g. the list filter's own messages) and must not be broadcast.
type loadMsg interface{ loadMsg() }

// IsLoadMsg reports whether msg is a Vault response destined to its owning view.
func IsLoadMsg(msg tea.Msg) bool {
	_, ok := msg.(loadMsg)
	return ok
}

const requestTimeout = 15 * time.Second

// View is one tab of the application.
type View interface {
	// Init starts loading the view's root data.
	Init() tea.Cmd
	// Update handles a message. Key messages are only delivered to the active view.
	Update(tea.Msg) (View, tea.Cmd)
	View() string
	Resize(width, height int)
	// Capturing reports whether the view is consuming raw text input (list filter),
	// so global key bindings such as quit must be ignored.
	Capturing() bool
	// Path is the breadcrumb of the current location, shown in the status bar.
	Path() string
}

// item is a list entry carrying an arbitrary payload.
type item struct {
	title, desc string
	value       any
}

func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.desc }
func (i item) FilterValue() string { return i.title }

func newList() list.Model {
	l := list.New(nil, list.NewDefaultDelegate(), 0, 0)
	l.SetShowTitle(false)
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.DisableQuitKeybindings()
	return l
}

func setItems(l *list.Model, items []item) {
	li := make([]list.Item, len(items))
	for i, it := range items {
		// Display fields come from the server; the payload stays raw for API calls.
		it.title, it.desc = safe.Line(it.title), safe.Line(it.desc)
		li[i] = it
	}
	l.ResetFilter()
	l.SetItems(li)
	l.Select(0)
}

func selected(l list.Model) (item, bool) {
	it, ok := l.SelectedItem().(item)
	return it, ok
}

// payload extracts a typed payload from the selected item without panicking.
func payload[T any](it item) (T, bool) {
	v, ok := it.value.(T)
	return v, ok
}

func filtering(l list.Model) bool { return l.FilterState() == list.Filtering }

// request returns a context for a single Vault call.
func request() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), requestTimeout)
}

// base holds the state shared by every view.
type base struct {
	keys    keys.KeyMap
	width   int
	height  int
	loading bool
	err     error
	seq     int // identifies the latest request; older responses are dropped
}

// begin marks a new request as in flight and returns its sequence number.
func (b *base) begin() int {
	b.seq++
	b.loading = true
	b.err = nil
	return b.seq
}

// finish records a response. It returns false when the response is stale.
func (b *base) finish(seq int, err error) bool {
	if seq != b.seq {
		return false
	}
	b.loading = false
	b.err = err
	return true
}

// contentHeight is the height left for content under the message line.
func (b base) contentHeight() int { return max(b.height-1, 1) }

// render stacks the message line (loading/error, always one line) above the content.
func (b base) render(content string) string {
	msg := ""
	switch {
	case b.err != nil:
		msg = components.ErrorLine(b.err.Error(), b.width)
	case b.loading:
		msg = dimStyle.Render(" Loading…")
	}
	return msg + "\n" + content
}
