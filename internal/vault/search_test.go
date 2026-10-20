package vault_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/diablo02000/vault-tui/internal/vault"
	"github.com/diablo02000/vault-tui/internal/vault/vaulttest"
)

var kv = vault.Mount{Path: "secrets/", Type: "kv", Version: 2}

func secret(path string, data map[string]any) *vault.Secret {
	return &vault.Secret{Path: path, Data: data}
}

// k8sVault mimics secrets/kubernetes/<env>/<app>/<name>.
func k8sVault() *vaulttest.Fake {
	const base = "secrets/kubernetes/"
	f := &vaulttest.Fake{
		Mounts: []vault.Mount{
			kv,
			{Path: "pki/", Type: "pki"},
			{Path: "other/", Type: "kv", Version: 1},
		},
		Keys: map[string][]string{
			"secrets/":                            {"kubernetes/", "top"},
			"secrets/kubernetes/":                 {"int/", "sta/", "prd/"},
			"other/":                              {"kubernetes/"},
			"other/kubernetes/":                   {"x"},
			"secrets/kubernetes/int/":             {"smart-entry/", "billing/"},
			"secrets/kubernetes/sta/":             {"smart-entry/"},
			"secrets/kubernetes/prd/":             {"smart-entry/", "billing/"},
			"secrets/kubernetes/int/smart-entry/": {"smart-entry-vault/", "other/"},
			"secrets/kubernetes/sta/smart-entry/": {"smart-entry-vault/"},
			"secrets/kubernetes/prd/smart-entry/": {"smart-entry-vault/"},
			"secrets/kubernetes/int/smart-entry/smart-entry-vault/": {"db", "api"},
			"secrets/kubernetes/sta/smart-entry/smart-entry-vault/": {"db"},
			"secrets/kubernetes/prd/smart-entry/smart-entry-vault/": {"db"},
			"secrets/kubernetes/int/smart-entry/other/":             {"misc"},
			"secrets/kubernetes/int/billing/":                       {"db"},
			"secrets/kubernetes/prd/billing/":                       {"db"},
		},
		Secrets: map[string]*vault.Secret{},
	}
	for _, p := range []string{
		"int/smart-entry/smart-entry-vault/db", "sta/smart-entry/smart-entry-vault/db",
		"prd/smart-entry/smart-entry-vault/db", "int/smart-entry/other/misc",
		"int/billing/db", "prd/billing/db",
	} {
		f.Secrets[base+p] = secret(base+p, map[string]any{"password": "pw-" + p, "user": "u"})
	}
	f.Secrets[base+"int/smart-entry/smart-entry-vault/api"] = secret(
		"x",
		map[string]any{"token": "t"},
	)
	f.Secrets["secrets/top"] = secret("secrets/top", map[string]any{"Password": "top"})
	f.Secrets["other/kubernetes/x"] = secret("x", map[string]any{"password": "o"})
	return f
}

func paths(res *vault.SearchResult) []string {
	var out []string
	for _, m := range res.Matches {
		out = append(out, m.Path)
	}
	return out
}

func search(t *testing.T, r vault.Reader, scope, pathRe, keyRe string) *vault.SearchResult {
	t.Helper()
	q, err := vault.ParseSearch(scope, pathRe, keyRe)
	if err != nil {
		t.Fatal(err)
	}
	res, err := vault.Search(context.Background(), r, q)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// recorder tracks which folders were listed and how many calls ran at once.
type recorder struct {
	*vaulttest.Fake
	mu      sync.Mutex
	listed  []string
	running int
	peak    int
}

func (r *recorder) enter() {
	r.mu.Lock()
	r.running++
	r.peak = max(r.peak, r.running)
	r.mu.Unlock()
	time.Sleep(time.Millisecond)
}

func (r *recorder) leave() { r.mu.Lock(); r.running--; r.mu.Unlock() }

func (r *recorder) ListSecrets(ctx context.Context, m vault.Mount, dir string) ([]string, error) {
	r.enter()
	defer r.leave()
	r.mu.Lock()
	r.listed = append(r.listed, m.Path+dir)
	r.mu.Unlock()
	return r.Fake.ListSecrets(ctx, m, dir)
}

func (r *recorder) ReadSecret(ctx context.Context, m vault.Mount, p string) (*vault.Secret, error) {
	r.enter()
	defer r.leave()
	return r.Fake.ReadSecret(ctx, m, p)
}

func TestSearchScopeWildcardAcrossEnvironments(t *testing.T) {
	res := search(t, k8sVault(), "secrets/kubernetes/*/smart-entry/smart-entry-vault/", "", "")
	want := []string{
		"secrets/kubernetes/int/smart-entry/smart-entry-vault/api",
		"secrets/kubernetes/int/smart-entry/smart-entry-vault/db",
		"secrets/kubernetes/prd/smart-entry/smart-entry-vault/db",
		"secrets/kubernetes/sta/smart-entry/smart-entry-vault/db",
	}
	if got := paths(res); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestSearchPrunesFoldersOutsideScope(t *testing.T) {
	r := &recorder{Fake: k8sVault()}
	search(t, r, "secrets/kubernetes/*/smart-entry/smart-entry-vault/", "", "")
	for _, dir := range r.listed {
		if strings.Contains(dir, "billing") || strings.HasSuffix(dir, "/other/") ||
			strings.HasPrefix(dir, "other/") {
			t.Errorf("folder outside the scope was listed: %s", dir)
		}
	}
	if len(r.listed) == 0 {
		t.Fatal("nothing listed")
	}
}

func TestSearchOnlyVisitsMatchingMounts(t *testing.T) {
	r := &recorder{Fake: k8sVault()}
	search(t, r, "secrets/**", "", "")
	for _, dir := range r.listed {
		if !strings.HasPrefix(dir, "secrets/") {
			t.Errorf("unrelated mount listed: %s", dir)
		}
	}
}

func TestSearchSkipsNonBrowsableMounts(t *testing.T) {
	r := &recorder{Fake: k8sVault()}
	search(t, r, "", "", "password")
	for _, dir := range r.listed {
		if strings.HasPrefix(dir, "pki/") {
			t.Errorf("non-KV mount listed: %s", dir)
		}
	}
}

func TestSearchDoubleStar(t *testing.T) {
	res := search(t, k8sVault(), "secrets/kubernetes/**/billing", "", "")
	want := "secrets/kubernetes/int/billing/db,secrets/kubernetes/prd/billing/db"
	if got := strings.Join(paths(res), ","); got != want {
		t.Fatalf("got %s", got)
	}
}

func TestSearchScopeWithoutTrailingSlashMatchesSecretItself(t *testing.T) {
	res := search(t, k8sVault(), "secrets/top", "", "")
	if got := paths(res); len(got) != 1 || got[0] != "secrets/top" {
		t.Fatalf("got %v", got)
	}
}

func TestSearchScopeGlobInsideSegment(t *testing.T) {
	res := search(t, k8sVault(), "secrets/kubernetes/p*/billing", "", "")
	if got := strings.Join(paths(res), ","); got != "secrets/kubernetes/prd/billing/db" {
		t.Fatalf("got %s", got)
	}
}

func TestSearchPathRegex(t *testing.T) {
	res := search(t, k8sVault(), "secrets/kubernetes", `/(int|sta)/.*/db$`, "")
	want := "secrets/kubernetes/int/billing/db,secrets/kubernetes/int/smart-entry/smart-entry-vault/db," +
		"secrets/kubernetes/sta/smart-entry/smart-entry-vault/db"
	if got := strings.Join(paths(res), ","); got != want {
		t.Fatalf("got %s", got)
	}
}

func TestSearchKeyRegexReportsMatchingKeys(t *testing.T) {
	res := search(t, k8sVault(), "secrets/kubernetes/int/smart-entry", "", "^(token|user)$")
	if len(res.Matches) != 3 {
		t.Fatalf("matches = %v", paths(res))
	}
	for _, m := range res.Matches {
		if strings.HasSuffix(m.Path, "/api") {
			if strings.Join(m.Keys, ",") != "token" {
				t.Errorf("api keys = %v", m.Keys)
			}
		} else if strings.Join(m.Keys, ",") != "user" {
			t.Errorf("%s keys = %v", m.Path, m.Keys)
		}
	}
}

func TestSearchKeyRegexIsCaseInsensitive(t *testing.T) {
	res := search(t, k8sVault(), "secrets/top", "", "^password$")
	if len(res.Matches) != 1 || res.Matches[0].Keys[0] != "Password" {
		t.Fatalf("matches = %+v", res.Matches)
	}
}

func TestSearchPathOnlyNeverReadsSecrets(t *testing.T) {
	f := k8sVault()
	r := &readCounter{recorder: recorder{Fake: f}}
	search(t, r, "secrets/kubernetes", "", "")
	if r.reads != 0 {
		t.Fatalf("%d secrets were read for a path-only search", r.reads)
	}
}

type readCounter struct {
	recorder
	reads int
}

func (r *readCounter) ReadSecret(
	ctx context.Context,
	m vault.Mount,
	p string,
) (*vault.Secret, error) {
	r.mu.Lock()
	r.reads++
	r.mu.Unlock()
	return r.recorder.ReadSecret(ctx, m, p)
}

func TestSearchResultsNeverContainValues(t *testing.T) {
	res := search(t, k8sVault(), "secrets/kubernetes", "", "password")
	if len(res.Matches) == 0 {
		t.Fatal("no matches")
	}
	for _, m := range res.Matches {
		for _, k := range m.Keys {
			if strings.Contains(k, "pw-") {
				t.Fatalf("value leaked into result: %+v", m)
			}
		}
	}
}

func TestSearchNoMatch(t *testing.T) {
	res := search(t, k8sVault(), "secrets/nothing", "", "")
	if len(res.Matches) != 0 || res.Truncated || res.Cancelled {
		t.Fatalf("res = %+v", res)
	}
}

func TestSearchSkipsUnreadableAndReportsIt(t *testing.T) {
	f := k8sVault()
	r := &failingReader{Fake: f, failDir: "secrets/kubernetes/sta/"}
	res := search(t, r, "secrets/kubernetes", "", "")
	if res.Skipped != 1 || !strings.Contains(res.FirstErr, "permission denied") {
		t.Fatalf("skipped=%d err=%q", res.Skipped, res.FirstErr)
	}
	for _, p := range paths(res) {
		if strings.Contains(p, "/sta/") {
			t.Errorf("unreadable folder produced %s", p)
		}
	}
	if len(res.Matches) == 0 {
		t.Fatal("other folders must still be searched")
	}
}

type failingReader struct {
	*vaulttest.Fake
	failDir string
}

func (f *failingReader) ListSecrets(
	ctx context.Context,
	m vault.Mount,
	dir string,
) ([]string, error) {
	if m.Path+dir == f.failDir {
		return nil, vault.ErrPermissionDenied
	}
	return f.Fake.ListSecrets(ctx, m, dir)
}

func TestSearchNotFoundIsNotAFailure(t *testing.T) {
	f := k8sVault()
	delete(f.Secrets, "secrets/kubernetes/prd/billing/db") // gitleaks:allow (test fixture)
	res := search(t, f, "secrets/kubernetes/prd/billing", "", "password")
	if res.Skipped != 0 || len(res.Matches) != 0 {
		t.Fatalf("res = %+v", res)
	}
}

func TestSearchMountListingError(t *testing.T) {
	q, _ := vault.ParseSearch("x", "", "")
	f := &vaulttest.Fake{Err: errors.New("boom")}
	if _, err := vault.Search(context.Background(), f, q); err == nil {
		t.Fatal("expected error")
	}
}

func TestSearchCancelledReturnsPartialResult(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	q, _ := vault.ParseSearch("secrets", "", "")
	res, err := vault.Search(ctx, k8sVault(), q)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Cancelled || res.Skipped != 0 {
		t.Fatalf("res = %+v", res)
	}
}

func TestSearchCallBudget(t *testing.T) {
	f := &vaulttest.Fake{
		Mounts:  []vault.Mount{kv},
		Keys:    map[string][]string{"secrets/": {}},
		Secrets: map[string]*vault.Secret{},
	}
	for i := range vault.MaxCalls + 50 {
		name := "s" + strings.Repeat(
			"x",
			1,
		) + string(
			rune('a'+i%26),
		) + string(
			rune('a'+i/26%26),
		) + string(
			rune('a'+i/676),
		)
		f.Keys["secrets/"] = append(f.Keys["secrets/"], name)
		f.Secrets["secrets/"+name] = secret(name, map[string]any{"k": "v"})
	}
	res := search(t, f, "", "", "^k$")
	if !res.Truncated || res.Calls > vault.MaxCalls {
		t.Fatalf("truncated=%v calls=%d", res.Truncated, res.Calls)
	}
}

func TestSearchResultLimit(t *testing.T) {
	f := &vaulttest.Fake{Mounts: []vault.Mount{kv}, Keys: map[string][]string{"secrets/": {}}}
	for i := range vault.MaxResults + 10 {
		f.Keys["secrets/"] = append(f.Keys["secrets/"],
			string(rune('a'+i%26))+string(rune('a'+i/26%26))+string(rune('a'+i/676)))
	}
	res := search(t, f, "secrets", "", "")
	if len(res.Matches) != vault.MaxResults || !res.Truncated {
		t.Fatalf("matches=%d truncated=%v", len(res.Matches), res.Truncated)
	}
}

func TestSearchConcurrencyIsBounded(t *testing.T) {
	r := &recorder{Fake: k8sVault()}
	search(t, r, "secrets/kubernetes", "", "password")
	if r.peak > 8 {
		t.Fatalf("peak concurrent calls = %d", r.peak)
	}
}

func TestSearchResultsAreSorted(t *testing.T) {
	res := search(t, k8sVault(), "secrets/kubernetes", "", "")
	p := paths(res)
	for i := 1; i < len(p); i++ {
		if p[i-1] > p[i] {
			t.Fatalf("unsorted: %v", p)
		}
	}
}

func TestParseSearchErrors(t *testing.T) {
	tests := map[string][3]string{
		"empty":         {"", "", ""},
		"blank":         {"  ", " ", "  "},
		"bad path re":   {"", "(", ""},
		"bad key re":    {"", "", "[a-"},
		"bad glob":      {"secrets/[a-", "", ""},
		"huge scope":    {strings.Repeat("a", 300), "", ""},
		"huge regex":    {"", strings.Repeat("a", 300), ""},
		"huge key":      {"", "", strings.Repeat("a", 300)},
		"lookahead N/A": {"", "(?=x)", ""},
	}
	for name, in := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := vault.ParseSearch(in[0], in[1], in[2])
			if !errors.Is(err, vault.ErrInvalidSearch) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestParseSearchAcceptsAnySingleCriterion(t *testing.T) {
	for _, in := range [][3]string{{"a", "", ""}, {"", "a", ""}, {"", "", "a"}} {
		if _, err := vault.ParseSearch(in[0], in[1], in[2]); err != nil {
			t.Errorf("%v: %v", in, err)
		}
	}
}

func TestSearchRegexCannotBacktrack(t *testing.T) {
	// A classic catastrophic pattern must finish instantly under RE2.
	start := time.Now()
	search(t, k8sVault(), "", `(a+)+$`, "")
	if time.Since(start) > 2*time.Second {
		t.Fatal("regex evaluation too slow")
	}
}
