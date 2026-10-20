package views

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/diablo02000/vault-tui/internal/tui/safe"
	"github.com/diablo02000/vault-tui/internal/vault"
)

// searchTimeout bounds a whole search, on top of the per-call limits in vault.Search.
const searchTimeout = 2 * time.Minute

// Search form fields, in focus order. The results list is focused after them.
const (
	fieldScope = iota
	fieldPath
	fieldKey
	fieldCount
	focusResults = fieldCount
)

// searchMsg carries the outcome of a search. It holds key names only, never values.
type searchMsg struct {
	seq int
	res *vault.SearchResult
	err error
}

func (searchMsg) loadMsg() {}

// searchPane is the search form and its results, shown over the secrets browser.
type searchPane struct {
	inputs  [fieldCount]textinput.Model
	results list.Model
	focus   int
	status  string
	width   int
	height  int
	cancel  context.CancelFunc // non-nil while a search is running
}

var searchLabels = [fieldCount]string{
	"Path scope  (glob: * one level, ** any depth)",
	"Path regex  (on the full secret path)",
	"Key regex   (on secret key names)",
}

func newSearchPane() searchPane {
	p := searchPane{results: newList()}
	placeholders := [fieldCount]string{
		"secrets/kubernetes/*/smart-entry/smart-entry-vault/",
		"optional, e.g. /(int|prd)/",
		"e.g. ^(password|token)$",
	}
	for i := range p.inputs {
		in := textinput.New()
		in.Prompt = "> "
		in.CharLimit = 256
		in.Placeholder = placeholders[i]
		p.inputs[i] = in
	}
	p.setFocus(fieldScope)
	return p
}

func (p *searchPane) setFocus(f int) {
	p.focus = (f + fieldCount + 1) % (fieldCount + 1)
	for i := range p.inputs {
		if i == p.focus {
			p.inputs[i].Focus()
		} else {
			p.inputs[i].Blur()
		}
	}
}

const searchHeader = 1 + 2*fieldCount + 1 // title, label+input per field, status

func (p *searchPane) resize(w, h int) {
	p.width, p.height = w, h
	for i := range p.inputs {
		p.inputs[i].Width = max(w-4, 10)
	}
	p.results.SetSize(w, max(h-searchHeader, 1))
}

// reset forgets the previous results, but keeps what the user typed.
func (p *searchPane) reset() {
	setItems(&p.results, nil)
	p.status = ""
	p.stop()
	p.setFocus(fieldScope)
}

// stop cancels a running search.
func (p *searchPane) stop() {
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
}

func (p *searchPane) running() bool { return p.cancel != nil }

func (p *searchPane) setResults(res *vault.SearchResult) {
	items := make([]item, len(res.Matches))
	for i, m := range res.Matches {
		desc := "path match"
		if len(m.Keys) > 0 {
			desc = "keys: " + strings.Join(m.Keys, ", ")
		}
		items[i] = item{title: m.Path, desc: desc, value: m}
	}
	setItems(&p.results, items)

	parts := []string{
		fmt.Sprintf("%d match(es)", len(res.Matches)),
		fmt.Sprintf("%d call(s)", res.Calls),
	}
	if res.Skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped (%s)", res.Skipped, res.FirstErr))
	}
	if res.Truncated {
		parts = append(parts, "truncated: narrow the scope")
	}
	if res.Cancelled {
		parts = append(parts, "cancelled or timed out: partial results")
	}
	p.status = strings.Join(parts, " · ")
}

func (p searchPane) view() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Search secrets") +
		dimStyle.Render("  enter: search · tab: next field · esc: close") + "\n")
	for i := range p.inputs {
		b.WriteString(dimStyle.Render(searchLabels[i]) + "\n" + p.inputs[i].View() + "\n")
	}
	status := p.status
	if p.running() {
		status = "searching… (esc to cancel)"
	}
	b.WriteString(dimStyle.Render(safe.Line(status)) + "\n")
	b.WriteString(p.results.View())
	return b.String()
}

// selectedMatch is the result under the cursor.
func (p searchPane) selectedMatch() (vault.SearchMatch, bool) {
	it, ok := selected(p.results)
	if !ok {
		return vault.SearchMatch{}, false
	}
	return payload[vault.SearchMatch](it)
}

// searchKey handles a key while the search pane is open.
func (v *SecretsView) searchKey(msg tea.KeyMsg) (View, tea.Cmd) {
	p := &v.search
	switch msg.String() {
	case "esc":
		if p.running() {
			p.stop() // the partial results arrive with the search message
			return v, nil
		}
		v.closeSearch()
		return v, nil
	case "tab":
		p.setFocus(p.focus + 1)
		return v, nil
	case "shift+tab":
		p.setFocus(p.focus - 1)
		return v, nil
	case "enter":
		if p.focus == focusResults {
			return v, v.openMatch()
		}
		return v, v.runSearch()
	}
	if p.focus == focusResults {
		var cmd tea.Cmd
		p.results, cmd = p.results.Update(msg)
		return v, cmd
	}
	var cmd tea.Cmd
	p.inputs[p.focus], cmd = p.inputs[p.focus].Update(msg)
	return v, cmd
}

func (v *SecretsView) openSearch() {
	v.searchFrom = searchOrigin{mode: v.mode, mount: v.mount, dir: v.dir}
	v.mode = secretsSearch
	v.search.setFocus(fieldScope)
}

func (v *SecretsView) closeSearch() {
	v.search.reset()
	v.mode, v.mount, v.dir = v.searchFrom.mode, v.searchFrom.mount, v.searchFrom.dir
	v.fromSearch = false
	v.err = nil
}

func (v *SecretsView) runSearch() tea.Cmd {
	p := &v.search
	q, err := vault.ParseSearch(p.inputs[fieldScope].Value(), p.inputs[fieldPath].Value(),
		p.inputs[fieldKey].Value())
	if err != nil {
		v.err = err
		return nil
	}
	p.stop()
	ctx, cancel := context.WithTimeout(context.Background(), searchTimeout)
	p.cancel = cancel
	setItems(&p.results, nil)
	p.status = ""
	seq := v.begin()
	reader := v.reader
	return func() tea.Msg {
		res, err := vault.Search(ctx, reader, q)
		return searchMsg{seq, res, err}
	}
}

func (v *SecretsView) onSearchDone(msg searchMsg) {
	if !v.finish(msg.seq, msg.err) {
		return
	}
	v.search.stop()
	if msg.err == nil && msg.res != nil {
		v.search.setResults(msg.res)
		if len(msg.res.Matches) > 0 {
			v.search.setFocus(focusResults)
		}
	}
}

// openMatch loads the selected result in the detail view; leaving it returns to the results.
func (v *SecretsView) openMatch() tea.Cmd {
	m, ok := v.search.selectedMatch()
	if !ok {
		return nil
	}
	v.mount = m.Mount
	v.fromSearch = true
	v.reveal = false
	return v.loadSecret(strings.TrimPrefix(m.Path, m.Mount.Path))
}
