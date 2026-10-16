package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/diablo02000/vault-tui/internal/tui/keys"
	"github.com/diablo02000/vault-tui/internal/vault"
)

type secretsMode int

const (
	secretsMounts secretsMode = iota
	secretsPaths
	secretsDetail
)

type (
	mountsMsg struct {
		seq    int
		mounts []vault.Mount
		err    error
	}
	secretKeysMsg struct {
		seq  int
		keys []string
		err  error
	}
	secretMsg struct {
		seq    int
		secret *vault.Secret
		err    error
	}
)

// SecretsView browses secret engines, KV paths and secret contents.
type SecretsView struct {
	base
	reader vault.Reader

	mode   secretsMode
	mounts list.Model
	paths  list.Model
	detail Detail

	mount  vault.Mount
	dir    string // current folder inside the mount, "" or "a/b/"
	secret *vault.Secret
	reveal bool
}

// NewSecretsView creates the secrets tab.
func NewSecretsView(r vault.Reader, k keys.KeyMap) *SecretsView {
	return &SecretsView{
		base:   base{keys: k},
		reader: r,
		mounts: newList(),
		paths:  newList(),
		detail: newDetail(),
	}
}

func (v *SecretsView) Init() tea.Cmd { return v.loadMounts() }

func (v *SecretsView) Resize(w, h int) {
	v.width, v.height = w, h
	v.mounts.SetSize(w, v.contentHeight())
	v.paths.SetSize(w, v.contentHeight())
	v.detail.resize(w, v.contentHeight())
}

func (v *SecretsView) Capturing() bool {
	switch v.mode {
	case secretsMounts:
		return filtering(v.mounts)
	case secretsPaths:
		return filtering(v.paths)
	case secretsDetail:
		return false
	}
	return false
}

func (v *SecretsView) Path() string {
	switch v.mode {
	case secretsMounts:
		return "secrets"
	case secretsPaths:
		return v.mount.Path + v.dir
	case secretsDetail:
		if v.secret != nil {
			return v.secret.Path
		}
	}
	return "secrets"
}

func (v *SecretsView) loadMounts() tea.Cmd {
	seq := v.begin()
	return func() tea.Msg {
		ctx, cancel := request()
		defer cancel()
		m, err := v.reader.ListMounts(ctx)
		return mountsMsg{seq, m, err}
	}
}

func (v *SecretsView) loadKeys() tea.Cmd {
	seq := v.begin()
	mount, dir := v.mount, v.dir
	return func() tea.Msg {
		ctx, cancel := request()
		defer cancel()
		k, err := v.reader.ListSecrets(ctx, mount, dir)
		return secretKeysMsg{seq, k, err}
	}
}

func (v *SecretsView) loadSecret(name string) tea.Cmd {
	seq := v.begin()
	mount := v.mount
	return func() tea.Msg {
		ctx, cancel := request()
		defer cancel()
		s, err := v.reader.ReadSecret(ctx, mount, name)
		return secretMsg{seq, s, err}
	}
}

func (v *SecretsView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case mountsMsg:
		if v.finish(msg.seq, msg.err) && msg.err == nil {
			items := make([]item, len(msg.mounts))
			for i, m := range msg.mounts {
				items[i] = item{title: m.Path, desc: mountDesc(m), value: m}
			}
			setItems(&v.mounts, items)
		}
		return v, nil
	case secretKeysMsg:
		if v.finish(msg.seq, msg.err) && msg.err == nil {
			items := make([]item, len(msg.keys))
			for i, k := range msg.keys {
				desc := "secret"
				if strings.HasSuffix(k, "/") {
					desc = "folder"
				}
				items[i] = item{title: k, desc: desc, value: k}
			}
			setItems(&v.paths, items)
		}
		return v, nil
	case secretMsg:
		if v.finish(msg.seq, msg.err) && msg.err == nil {
			v.secret = msg.secret
			v.mode = secretsDetail
			v.renderSecret()
		}
		return v, nil
	case tea.KeyMsg:
		return v.handleKey(msg)
	}
	return v, v.forward(msg)
}

// forward passes other messages to the widget currently shown.
func (v *SecretsView) forward(msg tea.Msg) tea.Cmd {
	if v.mode == secretsDetail {
		return v.detail.update(msg)
	}
	return v.updateList(msg)
}

func (v *SecretsView) handleKey(msg tea.KeyMsg) (View, tea.Cmd) {
	if v.Capturing() {
		return v, v.updateList(msg)
	}
	switch {
	case key.Matches(msg, v.keys.Select):
		return v, v.open()
	case key.Matches(msg, v.keys.Back):
		// Esc first clears an applied list filter.
		if v.mode != secretsDetail && v.activeList().FilterState() == list.FilterApplied {
			return v, v.updateList(msg)
		}
		return v, v.back()
	case key.Matches(msg, v.keys.Refresh):
		return v, v.refresh()
	case key.Matches(msg, v.keys.Reveal) && v.mode == secretsDetail:
		v.reveal = !v.reveal
		v.renderSecret()
		return v, nil
	}
	if v.mode == secretsDetail {
		return v, v.detail.update(msg)
	}
	return v, v.updateList(msg)
}

func (v *SecretsView) activeList() *list.Model {
	if v.mode == secretsMounts {
		return &v.mounts
	}
	return &v.paths
}

func (v *SecretsView) updateList(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	l := v.activeList()
	*l, cmd = l.Update(msg)
	return cmd
}

func (v *SecretsView) open() tea.Cmd {
	switch v.mode {
	case secretsMounts:
		it, ok := selected(v.mounts)
		if !ok {
			return nil
		}
		m, ok := payload[vault.Mount](it)
		if !ok {
			return nil
		}
		if !m.Browsable() {
			v.err = fmt.Errorf(
				"mount %s has type %q: only KV engines can be browsed",
				m.Path,
				m.Type,
			)
			return nil
		}
		v.mount, v.dir, v.mode = m, "", secretsPaths
		setItems(&v.paths, nil)
		return v.loadKeys()
	case secretsPaths:
		it, ok := selected(v.paths)
		if !ok {
			return nil
		}
		name, ok := payload[string](it)
		if !ok {
			return nil
		}
		if strings.HasSuffix(name, "/") {
			v.dir += name
			setItems(&v.paths, nil)
			return v.loadKeys()
		}
		v.reveal = false
		return v.loadSecret(v.dir + name)
	case secretsDetail:
		return nil
	}
	return nil
}

func (v *SecretsView) back() tea.Cmd {
	v.err = nil
	switch v.mode {
	case secretsDetail:
		// Do not keep secret values in memory (or on screen) once left.
		v.secret, v.reveal = nil, false
		v.detail.set("", "")
		v.mode = secretsPaths
	case secretsPaths:
		if v.dir == "" {
			v.mode = secretsMounts
			return nil
		}
		v.dir = parentDir(v.dir)
		setItems(&v.paths, nil)
		return v.loadKeys()
	case secretsMounts:
	}
	return nil
}

func (v *SecretsView) refresh() tea.Cmd {
	switch v.mode {
	case secretsMounts:
		return v.loadMounts()
	case secretsPaths:
		return v.loadKeys()
	case secretsDetail:
		if v.secret != nil {
			return v.loadSecret(strings.TrimPrefix(v.secret.Path, v.mount.Path))
		}
	}
	return nil
}

func (v *SecretsView) renderSecret() {
	var b strings.Builder
	if len(v.secret.Metadata) > 0 {
		b.WriteString(dimStyle.Render("metadata") + "\n")
		b.WriteString(renderMap(v.secret.Metadata, false) + "\n")
	}
	b.WriteString(dimStyle.Render("data") + "\n")
	b.WriteString(renderMap(v.secret.Data, !v.reveal))
	title := v.secret.Path
	if !v.reveal {
		title += "  (values hidden, press v to reveal)"
	}
	v.detail.set(title, b.String())
}

func (v *SecretsView) View() string {
	switch v.mode {
	case secretsMounts:
		return v.render(v.mounts.View())
	case secretsPaths:
		return v.render(v.paths.View())
	case secretsDetail:
		return v.render(v.detail.view())
	}
	return ""
}

func mountDesc(m vault.Mount) string {
	d := "type: " + m.Type
	if m.Version > 0 {
		d += fmt.Sprintf(" (kv v%d)", m.Version)
	}
	if m.Description != "" {
		d += " · " + m.Description
	}
	return d
}

// parentDir turns "a/b/" into "a/" and "a/" into "".
func parentDir(dir string) string {
	trimmed := strings.TrimSuffix(dir, "/")
	i := strings.LastIndex(trimmed, "/")
	if i < 0 {
		return ""
	}
	return trimmed[:i+1]
}

func (mountsMsg) loadMsg()     {}
func (secretKeysMsg) loadMsg() {}
func (secretMsg) loadMsg()     {}
