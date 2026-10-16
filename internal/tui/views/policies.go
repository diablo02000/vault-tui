package views

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/diablo02000/vault-tui/internal/tui/keys"
	"github.com/diablo02000/vault-tui/internal/tui/safe"
	"github.com/diablo02000/vault-tui/internal/vault"
)

type (
	policiesMsg struct {
		seq   int
		names []string
		err   error
	}
	policyMsg struct {
		seq   int
		name  string
		rules string
		err   error
	}
)

// PoliciesView lists ACL policies and shows their HCL.
type PoliciesView struct {
	base
	reader vault.Reader

	list     list.Model
	detail   Detail
	inDetail bool
	current  string
}

// NewPoliciesView creates the policies tab.
func NewPoliciesView(r vault.Reader, k keys.KeyMap) *PoliciesView {
	return &PoliciesView{base: base{keys: k}, reader: r, list: newList(), detail: newDetail()}
}

func (v *PoliciesView) Init() tea.Cmd { return v.loadList() }

func (v *PoliciesView) Resize(w, h int) {
	v.width, v.height = w, h
	v.list.SetSize(w, v.contentHeight())
	v.detail.resize(w, v.contentHeight())
}

func (v *PoliciesView) Capturing() bool { return !v.inDetail && filtering(v.list) }

func (v *PoliciesView) Path() string {
	if v.inDetail {
		return "policies/" + v.current
	}
	return "policies"
}

func (v *PoliciesView) loadList() tea.Cmd {
	seq := v.begin()
	return func() tea.Msg {
		ctx, cancel := request()
		defer cancel()
		n, err := v.reader.ListPolicies(ctx)
		return policiesMsg{seq, n, err}
	}
}

func (v *PoliciesView) loadPolicy(name string) tea.Cmd {
	seq := v.begin()
	return func() tea.Msg {
		ctx, cancel := request()
		defer cancel()
		r, err := v.reader.ReadPolicy(ctx, name)
		return policyMsg{seq, name, r, err}
	}
}

func (v *PoliciesView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case policiesMsg:
		if v.finish(msg.seq, msg.err) && msg.err == nil {
			items := make([]item, len(msg.names))
			for i, n := range msg.names {
				items[i] = item{title: n, value: n}
			}
			setItems(&v.list, items)
		}
	case policyMsg:
		if v.finish(msg.seq, msg.err) && msg.err == nil {
			v.current, v.inDetail = msg.name, true
			v.detail.set(msg.name, safe.Text(msg.rules))
		}
	case tea.KeyMsg:
		return v, v.handleKey(msg)
	}
	return v, v.forward(msg)
}

// forward passes other messages to the widget currently shown.
func (v *PoliciesView) forward(msg tea.Msg) tea.Cmd {
	if v.inDetail {
		return v.detail.update(msg)
	}
	return v.updateList(msg)
}

func (v *PoliciesView) handleKey(msg tea.KeyMsg) tea.Cmd {
	if v.Capturing() {
		return v.updateList(msg)
	}
	switch {
	case key.Matches(msg, v.keys.Select) && !v.inDetail:
		if it, ok := selected(v.list); ok {
			if name, ok := payload[string](it); ok {
				return v.loadPolicy(name)
			}
		}
		return nil
	case key.Matches(msg, v.keys.Back):
		if v.inDetail {
			v.inDetail, v.err = false, nil
			return nil
		}
		if v.list.FilterState() == list.FilterApplied {
			return v.updateList(msg)
		}
		return nil
	case key.Matches(msg, v.keys.Refresh):
		if v.inDetail {
			return v.loadPolicy(v.current)
		}
		return v.loadList()
	}
	if v.inDetail {
		return v.detail.update(msg)
	}
	return v.updateList(msg)
}

func (v *PoliciesView) updateList(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	v.list, cmd = v.list.Update(msg)
	return cmd
}

func (v *PoliciesView) View() string {
	if v.inDetail {
		return v.render(v.detail.view())
	}
	return v.render(v.list.View())
}

func (policiesMsg) loadMsg() {}
func (policyMsg) loadMsg()   {}
