package clip_test

import (
	"errors"
	"testing"
	"time"

	"github.com/diablo02000/vault-tui/internal/tui/clip"
)

type mem struct {
	text string
	err  error
}

func (m *mem) Read() (string, error) { return m.text, m.err }
func (m *mem) Write(s string) error {
	if m.err != nil {
		return m.err
	}
	m.text = s
	return nil
}

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

func TestClearKeepsForeignContent(t *testing.T) {
	b := &mem{}
	c := clip.New(b)
	_ = c.Copy("hunter2")
	b.text = "something the user copied later"
	if err := c.Clear(); err != nil {
		t.Fatal(err)
	}
	if b.text != "something the user copied later" {
		t.Fatalf("foreign clipboard content was wiped: %q", b.text)
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

func TestDefaultDelay(t *testing.T) {
	if d := clip.New(&mem{}).Delay(); d != clip.ClearDelay {
		t.Fatalf("delay = %v", d)
	}
}

func TestClearReadError(t *testing.T) {
	b := &mem{}
	c := clip.New(b)
	_ = c.Copy("x")
	want := errors.New("read failed")
	b.err = want
	if err := c.Clear(); !errors.Is(err, want) {
		t.Fatalf("err = %v", err)
	}
}

func TestClearWriteErrorKeepsTracking(t *testing.T) {
	b := &failWrite{mem: mem{}}
	c := clip.New(b)
	_ = c.Copy("x")
	b.fail = true
	if err := c.Clear(); err == nil {
		t.Fatal("expected write error")
	}
	b.fail = false
	if err := c.Clear(); err != nil || b.text != "" {
		t.Fatalf("retry: err=%v clipboard=%q", err, b.text)
	}
}

func TestClearTwiceIsNoop(t *testing.T) {
	b := &mem{}
	c := clip.New(b)
	_ = c.Copy("x")
	_ = c.Clear()
	b.text = "later"
	if err := c.Clear(); err != nil || b.text != "later" {
		t.Fatalf("err=%v clipboard=%q", err, b.text)
	}
}

type failWrite struct {
	mem
	fail bool
}

func (f *failWrite) Write(s string) error {
	if f.fail {
		return errors.New("write failed")
	}
	return f.mem.Write(s)
}
