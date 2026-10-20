// Package clip copies secret values to the terminal's clipboard using the
// OSC 52 escape sequence, so it works over SSH and in containers without
// xclip, xsel or wl-clipboard. A copied secret is wiped after a timeout, and
// on exit.
//
// OSC 52 is write-only by design: reading the clipboard back would let any
// program writing to the terminal exfiltrate it, and terminals disable it. The
// wipe therefore cannot check that the clipboard still holds our value, so it
// overwrites whatever is there.
package clip

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/aymanbagabas/go-osc52/v2"
	"github.com/charmbracelet/x/term"
)

// ClearDelay is how long a copied value stays in the clipboard.
const ClearDelay = 30 * time.Second

// MaxSize is the largest value (in bytes, before base64) sent to the
// terminal. Terminals silently drop longer sequences (xterm: 100000 bytes
// once encoded), which would leave the previous clipboard content in place.
const MaxSize = 74994

var (
	// ErrNotTerminal is returned when the output is not a terminal, so an
	// OSC 52 sequence would be written to a file or pipe instead.
	ErrNotTerminal = errors.New("clipboard unavailable (output is not a terminal)")
	// ErrTooLarge is returned when a value exceeds MaxSize.
	ErrTooLarge = errors.New("value too large to copy through the terminal")
)

// Backend is a write-only clipboard.
type Backend interface {
	Write(string) error
	Clear() error
}

// OSC52 is a clipboard reached through terminal escape sequences.
type OSC52 struct {
	out  io.Writer
	mode osc52.Mode
}

// NewOSC52 returns a Backend writing to out, which should be the terminal.
// Inside tmux or GNU screen the sequence is wrapped so it reaches the outer
// terminal (tmux needs `set -g allow-passthrough on`).
func NewOSC52(out io.Writer) *OSC52 {
	mode := osc52.DefaultMode
	switch {
	case os.Getenv("TMUX") != "":
		mode = osc52.TmuxMode
	case strings.HasPrefix(os.Getenv("TERM"), "screen"):
		mode = osc52.ScreenMode
	}
	return &OSC52{out: out, mode: mode}
}

// Write replaces the clipboard text.
func (o *OSC52) Write(s string) error {
	if len(s) > MaxSize {
		return ErrTooLarge
	}
	return o.send(osc52.New(s).Mode(o.mode).Limit(MaxSize))
}

// Clear empties the clipboard.
func (o *OSC52) Clear() error {
	return o.send(osc52.New().Mode(o.mode).Clear())
}

// send emits the sequence in a single write so it cannot be split by the
// renderer's own output.
func (o *OSC52) send(seq osc52.Sequence) error {
	if f, ok := o.out.(*os.File); ok && !term.IsTerminal(f.Fd()) {
		return ErrNotTerminal
	}
	if _, err := o.out.Write([]byte(seq.String())); err != nil {
		return fmt.Errorf("write clipboard sequence: %w", err)
	}
	return nil
}

// Clipboard writes secrets to a Backend and wipes them later.
// It is safe for concurrent use.
type Clipboard struct {
	backend Backend
	delay   time.Duration

	mu  sync.Mutex
	gen uint64 // incremented on every copy; lets a stale timer know it was superseded
	set bool   // a copied value may still be in the clipboard
}

// New returns a Clipboard on top of b. A nil b selects OSC 52 on stdout.
func New(b Backend) *Clipboard {
	if b == nil {
		b = NewOSC52(os.Stdout)
	}
	return &Clipboard{backend: b, delay: ClearDelay}
}

// Copy puts value in the clipboard. The value is never retained.
func (c *Clipboard) Copy(value string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.backend.Write(value); err != nil {
		return err
	}
	c.gen++
	c.set = true
	return nil
}

// SetDelay overrides how long ClearAfterDelay waits (used by tests).
func (c *Clipboard) SetDelay(d time.Duration) { c.delay = d }

// Delay is how long a copied value is kept before ClearAfterDelay wipes it.
func (c *Clipboard) Delay() time.Duration { return c.delay }

// ClearAfterDelay blocks for the configured delay, then wipes the clipboard,
// unless a newer value was copied meanwhile: that copy has its own timer.
// Run it from a command or goroutine, after Copy.
func (c *Clipboard) ClearAfterDelay() error {
	c.mu.Lock()
	gen := c.gen
	c.mu.Unlock()

	time.Sleep(c.delay)

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.gen != gen {
		return nil
	}
	return c.clearLocked()
}

// Clear wipes the clipboard if we copied something into it.
func (c *Clipboard) Clear() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.clearLocked()
}

func (c *Clipboard) clearLocked() error {
	if !c.set {
		return nil
	}
	if err := c.backend.Clear(); err != nil {
		return err
	}
	c.set = false
	return nil
}
