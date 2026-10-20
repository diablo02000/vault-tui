package clip_test

import (
	"bytes"
	"encoding/base64"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/diablo02000/vault-tui/internal/tui/clip"
)

type mem struct {
	text string
	err  error
}

func (m *mem) Write(s string) error {
	if m.err != nil {
		return m.err
	}
	m.text = s
	return nil
}

func (m *mem) Clear() error { return m.Write("") }

func TestCopyThenClear(t *testing.T) {
	b := &mem{}
	c := clip.New(b)
	if err := c.Copy("hunter2"); err != nil {
		t.Fatal(err)
	}
	if b.text != "hunter2" {
		t.Fatalf("clipboard = %q", b.text)
	}
	if err := c.Clear(); err != nil {
		t.Fatal(err)
	}
	if b.text != "" {
		t.Fatalf("clipboard not wiped: %q", b.text)
	}
}

func TestClearWithoutCopyIsNoop(t *testing.T) {
	b := &mem{text: "user data"}
	if err := clip.New(b).Clear(); err != nil || b.text != "user data" {
		t.Fatalf("err=%v clipboard=%q", err, b.text)
	}
}

func TestCopyError(t *testing.T) {
	want := errors.New("boom")
	if err := clip.New(&mem{err: want}).Copy("x"); !errors.Is(err, want) {
		t.Fatalf("err = %v", err)
	}
}

func TestClearAfterDelay(t *testing.T) {
	b := &mem{}
	c := clip.New(b)
	c.SetDelay(time.Millisecond)
	_ = c.Copy("hunter2")
	if err := c.ClearAfterDelay(); err != nil {
		t.Fatal(err)
	}
	if b.text != "" {
		t.Fatalf("clipboard not wiped: %q", b.text)
	}
}

func TestClearAfterDelaySupersededByNewerCopy(t *testing.T) {
	b := &mem{}
	c := clip.New(b)
	c.SetDelay(20 * time.Millisecond)
	_ = c.Copy("first")
	done := make(chan error)
	go func() { done <- c.ClearAfterDelay() }()
	time.Sleep(5 * time.Millisecond)
	_ = c.Copy("second")
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if b.text != "second" {
		t.Fatalf("stale timer wiped newer copy: %q", b.text)
	}
}

func TestDefaultDelay(t *testing.T) {
	if d := clip.New(&mem{}).Delay(); d != clip.ClearDelay {
		t.Fatalf("delay = %v", d)
	}
}

func TestClearErrorKeepsTracking(t *testing.T) {
	b := &mem{}
	c := clip.New(b)
	_ = c.Copy("x")
	b.err = errors.New("write failed")
	if err := c.Clear(); err == nil {
		t.Fatal("expected error")
	}
	b.err = nil
	if err := c.Clear(); err != nil || b.text != "" {
		t.Fatalf("retry: err=%v clipboard=%q", err, b.text)
	}
}

func TestOSC52WriteEncodesPayload(t *testing.T) {
	t.Setenv("TMUX", "")
	t.Setenv("TERM", "xterm-256color")
	var buf bytes.Buffer
	payload := "p@ss\x1b]0;evil\x07word"
	if err := clip.NewOSC52(&buf).Write(payload); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	enc := base64.StdEncoding.EncodeToString([]byte(payload))
	if want := "\x1b]52;c;" + enc + "\x07"; got != want {
		t.Fatalf("sequence = %q, want %q", got, want)
	}
	if strings.Count(got, "\x1b") != 1 || strings.Count(got, "\x07") != 1 {
		t.Fatalf("raw value leaked into the escape stream: %q", got)
	}
}

func TestOSC52Clear(t *testing.T) {
	t.Setenv("TMUX", "")
	t.Setenv("TERM", "xterm")
	var buf bytes.Buffer
	if err := clip.NewOSC52(&buf).Clear(); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != "\x1b]52;c;!\x07" {
		t.Fatalf("sequence = %q", got)
	}
}

func TestOSC52WrapsInsideTmux(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1000/default,1,0")
	var buf bytes.Buffer
	if err := clip.NewOSC52(&buf).Write("x"); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); !strings.HasPrefix(got, "\x1bPtmux;") {
		t.Fatalf("not wrapped for tmux: %q", got)
	}
}

func TestOSC52RejectsOversizedValue(t *testing.T) {
	var buf bytes.Buffer
	err := clip.NewOSC52(&buf).Write(strings.Repeat("a", clip.MaxSize+1))
	if !errors.Is(err, clip.ErrTooLarge) || buf.Len() != 0 {
		t.Fatalf("err=%v written=%d", err, buf.Len())
	}
}

func TestOSC52RefusesNonTerminal(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := clip.NewOSC52(f).Write("secret"); !errors.Is(err, clip.ErrNotTerminal) {
		t.Fatalf("err = %v", err)
	}
	if st, _ := f.Stat(); st.Size() != 0 {
		t.Fatal("secret written to a non-terminal")
	}
}
