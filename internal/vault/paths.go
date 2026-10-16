package vault

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// ErrInvalidPath is returned for names that must never be put in an API path.
var ErrInvalidPath = errors.New("invalid path")

// checkPath rejects names that could escape the intended API endpoint
// (".." segments) or smuggle control characters into a request. Names come
// from the server's own listings, so this is defence in depth against a
// hostile or compromised server steering the client to other endpoints with
// the user's token.
func checkPath(p string) error {
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." || seg == "." {
			return fmt.Errorf("%w: %q contains a relative segment", ErrInvalidPath, p)
		}
	}
	for _, r := range p {
		if unicode.IsControl(r) {
			return fmt.Errorf("%w: %q contains a control character", ErrInvalidPath, p)
		}
	}
	return nil
}
