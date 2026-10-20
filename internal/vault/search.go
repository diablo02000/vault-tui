package vault

import (
	"context"
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Search limits. A search walks the server, so it is bounded: every Vault call
// (list or read) counts against MaxCalls, and the walk stops cleanly when a
// budget is exhausted, reporting the result as truncated.
const (
	MaxCalls       = 3000
	MaxResults     = 1000
	MaxDepth       = 16
	searchWorkers  = 8
	maxPatternSize = 256
)

// ErrInvalidSearch is returned for unusable search criteria.
var ErrInvalidSearch = errors.New("invalid search")

// SearchQuery selects secrets across mounts, folders and sub-folders.
type SearchQuery struct {
	scope     []string       // glob segments over mount + path; nil matches everything
	pathRegex *regexp.Regexp // optional extra filter on the full secret path
	keyRegex  *regexp.Regexp // optional filter on the secret's data key names
}

// ParseSearch validates the three user-supplied criteria.
//
//   - scope is a path glob matched segment by segment, starting at the mount:
//     "secrets/kubernetes/*/app" where * is exactly one level, ** any number of
//     levels, and ?, [a-z] work inside a level. Everything below the matched
//     folder is searched.
//   - pathRegex is an RE2 regular expression applied to the full secret path.
//   - keyRegex is an RE2 regular expression applied to data key names. When
//     empty, secrets are matched on their path alone and never read.
//
// Both regular expressions are case-insensitive. RE2 has no backtracking, so a
// hostile or careless pattern cannot stall the UI.
func ParseSearch(scope, pathRegex, keyRegex string) (SearchQuery, error) {
	var q SearchQuery
	scope, pathRegex, keyRegex = strings.TrimSpace(scope), strings.TrimSpace(pathRegex),
		strings.TrimSpace(keyRegex)
	if scope == "" && pathRegex == "" && keyRegex == "" {
		return q, fmt.Errorf("%w: enter at least one criterion", ErrInvalidSearch)
	}
	for name, p := range map[string]string{"scope": scope, "path regex": pathRegex, "key regex": keyRegex} {
		if len(p) > maxPatternSize {
			return q, fmt.Errorf(
				"%w: %s is longer than %d characters",
				ErrInvalidSearch,
				name,
				maxPatternSize,
			)
		}
	}
	if scope != "" {
		q.scope = splitPath(scope)
		for _, seg := range q.scope {
			if seg == "**" {
				continue
			}
			if _, err := path.Match(seg, ""); err != nil {
				return q, fmt.Errorf("%w: scope: %w", ErrInvalidSearch, err)
			}
		}
	}
	var err error
	if pathRegex != "" {
		if q.pathRegex, err = regexp.Compile("(?i)" + pathRegex); err != nil {
			return q, fmt.Errorf("%w: path regex: %w", ErrInvalidSearch, err)
		}
	}
	if keyRegex != "" {
		if q.keyRegex, err = regexp.Compile("(?i)" + keyRegex); err != nil {
			return q, fmt.Errorf("%w: key regex: %w", ErrInvalidSearch, err)
		}
	}
	return q, nil
}

// SearchMatch is a secret that satisfies the query. Only key names are kept:
// secret values are never retained by a search.
type SearchMatch struct {
	Mount Mount
	Path  string   // full path, including the mount
	Keys  []string // data keys matching the key regex; empty for a path-only search
}

// SearchResult is the outcome of a search, possibly partial.
type SearchResult struct {
	Matches   []SearchMatch
	Calls     int    // Vault calls made
	Skipped   int    // folders or secrets that could not be read (permissions, errors)
	FirstErr  string // first skipped error, for display
	Truncated bool   // a limit was reached
	Cancelled bool   // the context ended before the walk completed
}

// Search walks every browsable mount reachable by the scope and returns the
// secrets matching the query. Folders that cannot match the scope are never
// listed. On cancellation or timeout it returns the partial result.
func Search(ctx context.Context, r Reader, q SearchQuery) (*SearchResult, error) {
	mounts, err := r.ListMounts(ctx)
	if err != nil {
		return nil, err
	}
	s := &searcher{
		ctx: ctx,
		r:   r,
		q:   q,
		sem: make(chan struct{}, searchWorkers),
		res: &SearchResult{},
	}
	for _, m := range mounts {
		if !m.Browsable() || !scopeMatch(q.scope, splitPath(m.Path), false) {
			continue
		}
		s.wg.Add(1)
		go s.walk(m, "", 0)
	}
	s.wg.Wait()

	s.res.Cancelled = ctx.Err() != nil
	sort.Slice(
		s.res.Matches,
		func(i, j int) bool { return s.res.Matches[i].Path < s.res.Matches[j].Path },
	)
	return s.res, nil
}

type searcher struct {
	ctx context.Context
	r   Reader
	q   SearchQuery
	sem chan struct{} // bounds concurrent Vault calls
	wg  sync.WaitGroup

	mu  sync.Mutex
	res *SearchResult
}

// spend reserves one Vault call, reporting false when the budget is exhausted.
func (s *searcher) spend() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.res.Calls >= MaxCalls {
		s.res.Truncated = true
		return false
	}
	s.res.Calls++
	return s.ctx.Err() == nil
}

func (s *searcher) skip(err error) {
	if s.ctx.Err() != nil {
		return // cancellation is reported as such, not as a failure
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.res.Skipped++
	if s.res.FirstErr == "" {
		s.res.FirstErr = err.Error()
	}
}

func (s *searcher) add(m SearchMatch) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.res.Matches) >= MaxResults {
		s.res.Truncated = true
		return
	}
	s.res.Matches = append(s.res.Matches, m)
}

func (s *searcher) walk(m Mount, dir string, depth int) {
	defer s.wg.Done()
	if depth > MaxDepth || !s.spend() {
		return
	}
	s.sem <- struct{}{}
	names, err := s.r.ListSecrets(s.ctx, m, dir)
	<-s.sem
	if err != nil {
		if !errors.Is(err, ErrNotFound) {
			s.skip(err)
		}
		return
	}
	for _, name := range names {
		rel := dir + name
		full := m.Path + rel
		if strings.HasSuffix(name, "/") {
			if scopeMatch(s.q.scope, splitPath(full), false) {
				s.wg.Add(1)
				go s.walk(m, rel, depth+1)
			}
			continue
		}
		if !scopeMatch(s.q.scope, splitPath(full), true) {
			continue
		}
		if s.q.pathRegex != nil && !s.q.pathRegex.MatchString(full) {
			continue
		}
		if s.q.keyRegex == nil {
			s.add(SearchMatch{Mount: m, Path: full})
			continue
		}
		s.wg.Add(1)
		go s.inspect(m, rel, full)
	}
}

// inspect reads one secret and records which of its key names match. Values
// are dropped as soon as the key names are extracted.
func (s *searcher) inspect(m Mount, rel, full string) {
	defer s.wg.Done()
	if !s.spend() {
		return
	}
	s.sem <- struct{}{}
	secret, err := s.r.ReadSecret(s.ctx, m, rel)
	<-s.sem
	if err != nil {
		if !errors.Is(err, ErrNotFound) { // deleted KV v2 versions are not worth reporting
			s.skip(err)
		}
		return
	}
	var keys []string
	for k := range secret.Data {
		if s.q.keyRegex.MatchString(k) {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return
	}
	sort.Strings(keys)
	s.add(SearchMatch{Mount: m, Path: full, Keys: keys})
}

func splitPath(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

// scopeMatch reports whether segs lies under (or, for a folder, could lead to
// something under) the scope glob. With full set, segs is a secret and must be
// at least as deep as the scope; otherwise it is a folder and a shorter path is
// still worth descending into.
func scopeMatch(scope, segs []string, full bool) bool {
	switch {
	case len(scope) == 0:
		return true
	case scope[0] == "**": // zero or more levels, then the rest of the scope
		for i := 0; i <= len(segs); i++ {
			if scopeMatch(scope[1:], segs[i:], full) {
				return true
			}
		}
		return false
	case len(segs) == 0:
		return !full
	}
	if ok, _ := path.Match(scope[0], segs[0]); !ok {
		return false
	}
	return scopeMatch(scope[1:], segs[1:], full)
}
