package views

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/diablo02000/vault-tui/internal/tui/clip"
	"github.com/diablo02000/vault-tui/internal/tui/keys"
	"github.com/diablo02000/vault-tui/internal/vault"
	"github.com/diablo02000/vault-tui/internal/vault/vaulttest"
)

type memClip struct {
	mu   sync.Mutex
	text string
	err  error
}

func (c *memClip) Read() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.text, c.err
}

func (c *memClip) Write(s string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return c.err
	}
	c.text = s
	return nil
}

func (c *memClip) get() string { t, _ := c.Read(); return t }

func detailView(t *testing.T, data map[string]any) (*SecretsView, *memClip, *clip.Clipboard) {
	t.Helper()
	mc := &memClip{}
	cb := clip.New(mc)
	cb.SetDelay(time.Millisecond)
	v := NewSecretsView(&vaulttest.Fake{}, keys.Default(), cb)
	v.Resize(80, 10)
	v.mount = vault.Mount{Path: "secret/", Version: 2}
	v.secret = &vault.Secret{Path: "secret/x", Data: data}
	v.mode = secretsDetail
	v.resetCursor()
	v.renderSecret()
	return v, mc, cb
}

func press(v *SecretsView, k string) tea.Cmd {
	var msg tea.KeyMsg
	switch k {
	case "down":
		msg = tea.KeyMsg{Type: tea.KeyDown}
	case "up":
		msg = tea.KeyMsg{Type: tea.KeyUp}
	default:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
	}
	_, cmd := v.handleKey(msg)
	return cmd
}

// run executes the copy command and feeds its message back to the view.
func run(t *testing.T, v *SecretsView, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("nil command")
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected batch, got %T", msg)
	}
	if m, ok := batch[0]().(copyMsg); ok {
		v.Update(m)
	} else {
		t.Fatal("first command must yield copyMsg")
	}
}

func TestDataKeysAreSorted(t *testing.T) {
	v, _, _ := detailView(t, map[string]any{"b": 1, "c": 2, "a": 3})
	if got := strings.Join(v.dataKeys, ","); got != "a,b,c" {
		t.Fatalf("keys = %s", got)
	}
}

func TestCursorIsClamped(t *testing.T) {
	v, _, _ := detailView(t, map[string]any{"a": 1, "b": 2})
	press(v, "up")
	if v.cursor != 0 {
		t.Fatalf("cursor = %d", v.cursor)
	}
	for range 5 {
		press(v, "down")
	}
	if v.cursor != 1 {
		t.Fatalf("cursor = %d", v.cursor)
	}
	press(v, "k")
	if v.cursor != 0 {
		t.Fatalf("k should move up, cursor = %d", v.cursor)
	}
}

func TestCopyTargetsSelectedKey(t *testing.T) {
	v, mc, _ := detailView(t, map[string]any{"a": "one", "b": "two"})
	press(v, "down")
	run(t, v, press(v, "y"))
	if mc.get() != "two" {
		t.Fatalf("clipboard = %q", mc.get())
	}
	if !strings.Contains(v.detail.title, `copied "b"`) {
		t.Fatalf("title = %q", v.detail.title)
	}
	if strings.Contains(v.detail.title, "two") || strings.Contains(v.detail.vp.View(), "two") {
		t.Fatal("copy must not reveal the value")
	}
}

func TestCopyAliasC(t *testing.T) {
	v, mc, _ := detailView(t, map[string]any{"a": "one"})
	run(t, v, press(v, "c"))
	if mc.get() != "one" {
		t.Fatalf("clipboard = %q", mc.get())
	}
}

func TestCopyRawValueIsNotSanitised(t *testing.T) {
	raw := "line1\nline2\t\x1b[31m"
	v, mc, _ := detailView(t, map[string]any{"cert": raw})
	run(t, v, press(v, "y"))
	if mc.get() != raw {
		t.Fatalf("clipboard = %q, want raw value", mc.get())
	}
	if strings.Contains(v.detail.vp.View(), "\x1b[31m") {
		t.Fatal("displayed text must stay sanitised")
	}
}

func TestCopyNonStringValues(t *testing.T) {
	tests := map[string]struct {
		val  any
		want string
	}{
		"number": {42, "42"},
		"bool":   {true, "true"},
		"nil":    {nil, "<nil>"},
		"map":    {map[string]any{"k": "v"}, `{"k":"v"}`},
		"list":   {[]any{"a", 1}, `["a",1]`},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			v, mc, _ := detailView(t, map[string]any{"x": tc.val})
			run(t, v, press(v, "y"))
			if mc.get() != tc.want {
				t.Fatalf("clipboard = %q, want %q", mc.get(), tc.want)
			}
		})
	}
}

func TestCopyEmptySecretDoesNothing(t *testing.T) {
	v, mc, _ := detailView(t, map[string]any{})
	if cmd := press(v, "y"); cmd != nil {
		t.Fatal("expected no command")
	}
	press(v, "down")
	if mc.get() != "" || v.cursor != 0 {
		t.Fatalf("clipboard=%q cursor=%d", mc.get(), v.cursor)
	}
}

func TestCopyOutsideDetailIsIgnored(t *testing.T) {
	v, mc, _ := detailView(t, map[string]any{"a": "one"})
	v.mode = secretsPaths
	press(v, "y")
	if mc.get() != "" {
		t.Fatalf("clipboard = %q", mc.get())
	}
}

func TestCopyErrorIsSurfaced(t *testing.T) {
	v, mc, _ := detailView(t, map[string]any{"a": "one"})
	mc.err = errors.New("no display")
	run(t, v, press(v, "y"))
	if v.err == nil || !strings.Contains(v.err.Error(), "no display") {
		t.Fatalf("err = %v", v.err)
	}
	if v.notice != "" {
		t.Fatalf("notice = %q", v.notice)
	}
}

func TestSecondCommandWipesClipboard(t *testing.T) {
	v, mc, _ := detailView(t, map[string]any{"a": "one"})
	cmd := press(v, "y")
	batch := cmd().(tea.BatchMsg)
	batch[0]()
	if mc.get() != "one" {
		t.Fatalf("clipboard = %q", mc.get())
	}
	batch[1]()
	if mc.get() != "" {
		t.Fatalf("clipboard not wiped after delay: %q", mc.get())
	}
}

func TestDelayedWipeSparesForeignContent(t *testing.T) {
	v, mc, _ := detailView(t, map[string]any{"a": "one"})
	batch := press(v, "y")().(tea.BatchMsg)
	batch[0]()
	_ = mc.Write("user text")
	batch[1]()
	if mc.get() != "user text" {
		t.Fatalf("clipboard = %q", mc.get())
	}
}

func TestMovingCursorClearsNotice(t *testing.T) {
	v, _, _ := detailView(t, map[string]any{"a": "one", "b": "two"})
	run(t, v, press(v, "y"))
	if v.notice == "" {
		t.Fatal("expected notice")
	}
	press(v, "down")
	if v.notice != "" {
		t.Fatalf("notice = %q", v.notice)
	}
}

func TestLeavingDetailForgetsCursorState(t *testing.T) {
	v, _, _ := detailView(t, map[string]any{"a": "one", "b": "two"})
	press(v, "down")
	v.back()
	if v.dataKeys != nil || v.cursor != 0 || v.notice != "" || v.secret != nil {
		t.Fatalf("state kept: keys=%v cursor=%d notice=%q", v.dataKeys, v.cursor, v.notice)
	}
}

func TestCursorMarkerRendered(t *testing.T) {
	v, _, _ := detailView(t, map[string]any{"a": "1", "b": "2"})
	lines := strings.Split(v.detail.vp.View(), "\n")
	var a, b string
	for _, l := range lines {
		switch {
		case strings.Contains(l, "a  "):
			a = l
		case strings.Contains(l, "b  "):
			b = l
		}
	}
	if !strings.Contains(a, "▸") || strings.Contains(b, "▸") {
		t.Fatalf("marker misplaced:\na=%q\nb=%q", a, b)
	}
}

func TestRenderDataMasking(t *testing.T) {
	m := map[string]any{"k": "secret"}
	if out := renderData(m, []string{"k"}, 0, true); strings.Contains(out, "secret") ||
		!strings.Contains(out, maskedValue) {
		t.Fatalf("masked = %q", out)
	}
	if out := renderData(m, []string{"k"}, 0, false); !strings.Contains(out, "secret") {
		t.Fatalf("revealed = %q", out)
	}
}

func TestDetailRevealScrolls(t *testing.T) {
	d := newDetail()
	d.resize(40, 5) // viewport height 4
	d.set("t", strings.Repeat("x\n", 50))
	d.reveal(2)
	if d.vp.YOffset != 0 {
		t.Fatalf("offset = %d", d.vp.YOffset)
	}
	d.reveal(10)
	if d.vp.YOffset != 7 {
		t.Fatalf("offset = %d, want 7", d.vp.YOffset)
	}
	d.reveal(3)
	if d.vp.YOffset != 3 {
		t.Fatalf("offset = %d, want 3", d.vp.YOffset)
	}
}

func TestDetailRerenderKeepsScrollSetResets(t *testing.T) {
	d := newDetail()
	d.resize(40, 5)
	content := strings.Repeat("x\n", 50)
	d.set("t", content)
	d.vp.SetYOffset(9)
	d.rerender("t", content)
	if d.vp.YOffset != 9 {
		t.Fatalf("rerender offset = %d", d.vp.YOffset)
	}
	d.set("t", content)
	if d.vp.YOffset != 0 {
		t.Fatalf("set offset = %d", d.vp.YOffset)
	}
}
