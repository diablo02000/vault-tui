package views

import (
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/diablo02000/vault-tui/internal/tui/keys"
	"github.com/diablo02000/vault-tui/internal/vault"
)

type authMode int

const (
	authMethods authMode = iota
	authRoles
	authRole
)

type (
	authMethodsMsg struct {
		seq     int
		methods []vault.AuthMount
		err     error
	}
	authRolesMsg struct {
		seq   int
		roles []vault.AuthRole
		err   error
	}
	authRoleMsg struct {
		seq  int
		role vault.AuthRole
		data map[string]any
		err  error
	}
)

// AuthView lists auth methods (like `vault auth list`), then their roles/users/groups.
type AuthView struct {
	base
	reader vault.Reader

	mode    authMode
	methods list.Model
	roles   list.Model
	detail  Detail

	method vault.AuthMount
	role   vault.AuthRole
}

// NewAuthView creates the auth tab.
func NewAuthView(r vault.Reader, k keys.KeyMap) *AuthView {
	return &AuthView{
		base:    base{keys: k},
		reader:  r,
		methods: newList(),
		roles:   newList(),
		detail:  newDetail(),
	}
}

func (v *AuthView) Init() tea.Cmd { return v.loadMethods() }

func (v *AuthView) Resize(w, h int) {
	v.width, v.height = w, h
	v.methods.SetSize(w, v.contentHeight())
	v.roles.SetSize(w, v.contentHeight())
	v.detail.resize(w, v.contentHeight())
}

func (v *AuthView) Capturing() bool {
	switch v.mode {
	case authMethods:
		return filtering(v.methods)
	case authRoles:
		return filtering(v.roles)
	case authRole:
		return false
	}
	return false
}

func (v *AuthView) Path() string {
	switch v.mode {
	case authMethods:
		return "auth"
	case authRoles:
		return "auth/" + v.method.Path
	case authRole:
		return fmt.Sprintf("auth/%s%s/%s", v.method.Path, v.role.Kind, v.role.Name)
	}
	return "auth"
}

func (v *AuthView) loadMethods() tea.Cmd {
	seq := v.begin()
	return func() tea.Msg {
		ctx, cancel := request()
		defer cancel()
		m, err := v.reader.ListAuthMethods(ctx)
		return authMethodsMsg{seq, m, err}
	}
}

func (v *AuthView) loadRoles() tea.Cmd {
	seq := v.begin()
	method := v.method
	return func() tea.Msg {
		ctx, cancel := request()
		defer cancel()
		r, err := v.reader.ListAuthRoles(ctx, method)
		return authRolesMsg{seq, r, err}
	}
}

func (v *AuthView) loadRole(role vault.AuthRole) tea.Cmd {
	seq := v.begin()
	return func() tea.Msg {
		ctx, cancel := request()
		defer cancel()
		d, err := v.reader.ReadAuthRole(ctx, role)
		return authRoleMsg{seq, role, d, err}
	}
}

func (v *AuthView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case authMethodsMsg:
		if v.finish(msg.seq, msg.err) && msg.err == nil {
			items := make([]item, len(msg.methods))
			for i, m := range msg.methods {
				items[i] = item{title: m.Path, desc: authDesc(m), value: m}
			}
			setItems(&v.methods, items)
		}
	case authRolesMsg:
		if v.finish(msg.seq, msg.err) && msg.err == nil {
			items := make([]item, len(msg.roles))
			for i, r := range msg.roles {
				items[i] = item{title: r.Name, desc: r.Kind, value: r}
			}
			setItems(&v.roles, items)
		}
	case authRoleMsg:
		if v.finish(msg.seq, msg.err) && msg.err == nil {
			v.role, v.mode = msg.role, authRole
			v.detail.set(v.Path(), renderMap(msg.data, false))
		}
	case tea.KeyMsg:
		return v, v.handleKey(msg)
	}
	return v, v.forward(msg)
}

// forward passes other messages to the widget currently shown.
func (v *AuthView) forward(msg tea.Msg) tea.Cmd {
	if v.mode == authRole {
		return v.detail.update(msg)
	}
	return v.updateList(msg)
}

func (v *AuthView) handleKey(msg tea.KeyMsg) tea.Cmd {
	if v.Capturing() {
		return v.updateList(msg)
	}
	switch {
	case key.Matches(msg, v.keys.Select):
		return v.open()
	case key.Matches(msg, v.keys.Back):
		if v.mode != authRole && v.activeList().FilterState() == list.FilterApplied {
			return v.updateList(msg)
		}
		v.err = nil
		switch v.mode {
		case authRole:
			v.mode = authRoles
		case authRoles:
			v.mode = authMethods
		case authMethods:
		}
		return nil
	case key.Matches(msg, v.keys.Refresh):
		switch v.mode {
		case authMethods:
			return v.loadMethods()
		case authRoles:
			return v.loadRoles()
		case authRole:
			return v.loadRole(v.role)
		}
	}
	if v.mode == authRole {
		return v.detail.update(msg)
	}
	return v.updateList(msg)
}

func (v *AuthView) open() tea.Cmd {
	switch v.mode {
	case authMethods:
		if it, ok := selected(v.methods); ok {
			if m, ok := payload[vault.AuthMount](it); ok {
				v.method, v.mode = m, authRoles
				setItems(&v.roles, nil)
				return v.loadRoles()
			}
		}
	case authRoles:
		if it, ok := selected(v.roles); ok {
			if r, ok := payload[vault.AuthRole](it); ok {
				return v.loadRole(r)
			}
		}
	case authRole:
	}
	return nil
}

func (v *AuthView) activeList() *list.Model {
	if v.mode == authMethods {
		return &v.methods
	}
	return &v.roles
}

func (v *AuthView) updateList(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	l := v.activeList()
	*l, cmd = l.Update(msg)
	return cmd
}

func (v *AuthView) View() string {
	switch v.mode {
	case authMethods:
		return v.render(v.methods.View())
	case authRoles:
		return v.render(v.roles.View())
	case authRole:
		return v.render(v.detail.view())
	}
	return ""
}

// authDesc mirrors the columns of `vault auth list`: type, accessor, version, description.
func authDesc(m vault.AuthMount) string {
	d := fmt.Sprintf("type: %s · accessor: %s", m.Type, m.Accessor)
	if m.Version != "" {
		d += " · version: " + m.Version
	}
	if m.Description != "" {
		d += " · " + m.Description
	}
	return d
}

func (authMethodsMsg) loadMsg() {}
func (authRolesMsg) loadMsg()   {}
func (authRoleMsg) loadMsg()    {}
