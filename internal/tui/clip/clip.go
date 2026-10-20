// Package clip copies secret values to the system clipboard and makes sure
// they do not linger there: the clipboard is shared with every application
// (and clipboard managers), so a copied secret is wiped after a timeout, and
// on exit, unless something else has been copied since.
package clip

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"sync"
	"time"

	"github.com/atotto/clipboard"
)

// ClearDelay is how long a copied value stays in the clipboard.
const ClearDelay = 30 * time.Second

// ErrUnavailable is returned when no clipboard mechanism is usable
// (e.g. a headless Linux session without xclip, xsel or wl-clipboard).
var ErrUnavailable = errors.New("clipboard unavailable (install xclip, xsel or wl-clipboard)")

// Backend is a raw clipboard.
type Backend interface {
	Read() (string, error)
	Write(string) error
}

// System is the operating system clipboard.
type System struct{}

// Read returns the clipboard text.
func (System) Read() (string, error) { return clipboard.ReadAll() }

// Write replaces the clipboard text.
func (System) Write(s string) error { return clipboard.WriteAll(s) }

// Clipboard writes secrets to a Backend and wipes them later.
// It is safe for concurrent use.
type Clipboard struct {
	backend Backend
	delay   time.Duration

	mu   sync.Mutex
	hash [sha256.Size]byte // digest of the last copied value; the value itself is never retained
	set  bool
}

// New returns a Clipboard on top of b. A nil b selects the system clipboard.
func New(b Backend) *Clipboard {
	if b == nil {
		if clipboard.Unsupported {
			return &Clipboard{backend: unavailable{}, delay: ClearDelay}
		}
		b = System{}
	}
	return &Clipboard{backend: b, delay: ClearDelay}
}

// Copy puts value in the clipboard.
func (c *Clipboard) Copy(value string) error {
	if err := c.backend.Write(value); err != nil {
		return err
	}
	c.mu.Lock()
	c.hash, c.set = sha256.Sum256([]byte(value)), true
	c.mu.Unlock()
	return nil
}

// SetDelay overrides how long ClearAfterDelay waits (used by tests).
func (c *Clipboard) SetDelay(d time.Duration) { c.delay = d }

// Delay is how long a copied value is kept before ClearAfterDelay wipes it.
func (c *Clipboard) Delay() time.Duration { return c.delay }

// ClearAfterDelay blocks for the configured delay, then calls Clear.
// Run it from a command or goroutine.
func (c *Clipboard) ClearAfterDelay() error {
	time.Sleep(c.delay)
	return c.Clear()
}

// Clear empties the clipboard, but only if it still holds the last value we
// copied: anything the user copied since is left alone.
func (c *Clipboard) Clear() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.set {
		return nil
	}
	cur, err := c.backend.Read()
	if err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(cur))
	if subtle.ConstantTimeCompare(sum[:], c.hash[:]) != 1 {
		c.set = false // overwritten by someone else, nothing to protect
		return nil
	}
	if err := c.backend.Write(""); err != nil {
		return err
	}
	c.hash, c.set = [sha256.Size]byte{}, false
	return nil
}

type unavailable struct{}

func (unavailable) Read() (string, error) { return "", ErrUnavailable }
func (unavailable) Write(string) error    { return ErrUnavailable }
