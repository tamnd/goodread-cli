package goodread

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// liveRobots is a real copy of https://www.goodreads.com/robots.txt, captured
// 2026-08-15. Every rule asserted below is a rule Goodreads actually publishes,
// not one invented to make a parser look good.
func liveRobots(t *testing.T) *Robots {
	t.Helper()
	b, err := os.ReadFile("testdata/robots.txt")
	if err != nil {
		t.Fatalf("read capture: %v", err)
	}
	return ParseRobots(b, DefaultUserAgent())
}

func TestParsePicksTheStarGroup(t *testing.T) {
	r := liveRobots(t)
	if len(r.Rules) == 0 {
		t.Fatal("no rules parsed")
	}
	// The "*" group disallows /search. The Mediapartners-Google group, which
	// is the one immediately above it in the file, does not. Landing on the
	// wrong group is silent and would grant permissions we were never given.
	if r.Allowed("/search?q=x") {
		t.Error("/search allowed, so the wrong User-agent group was selected")
	}
	// AmazonAdBot is given "Allow: /search". If group selection leaked, this
	// is where it would show.
	if m := r.Check("/search?q=x"); m == nil || m.Allow {
		t.Errorf("expected a Disallow for /search, got %v", m)
	}
	// bingbot carries Crawl-delay: 5. The "*" group carries none.
	if r.CrawlDelay != 0 {
		t.Errorf("CrawlDelay = %v, want 0 for the * group", r.CrawlDelay)
	}
}

// TestWorkExceptions is the rule the rest of the tool depends on.
//
// Goodreads publishes "Disallow: /work" with "Allow: /work/editions" and
// "Allow: /work/quotes". Longest match has to win or the editions and quotes
// pages, which are two of the nine allowed surfaces, disappear.
func TestWorkExceptions(t *testing.T) {
	r := liveRobots(t)
	cases := []struct {
		path string
		want bool
	}{
		{"/work/2792775", false},
		{"/work/editions/2792775", true},
		{"/work/quotes/2792775", true},
		{"/work/editions/2792775?page=2", true},
		{"/work/quotes/2792775?page=3", true},
		{"/work/best_book/2792775", false},
	}
	for _, c := range cases {
		if got := r.Allowed(c.path); got != c.want {
			t.Errorf("Allowed(%q) = %v, want %v (rule %v)", c.path, got, c.want, r.Check(c.path))
		}
	}
}

// TestReviewFiltersNeedsTheQuery is why Check takes path plus query.
//
// "Disallow: /*reviewFilters" can only ever match a query string. A checker
// handed the path alone permits every URL that rule was written to refuse, and
// it does so silently, because the path half looks perfectly allowed.
func TestReviewFiltersNeedsTheQuery(t *testing.T) {
	r := liveRobots(t)

	const bare = "/book/show/2767052"
	if !r.Allowed(bare) {
		t.Fatalf("%s should be allowed", bare)
	}

	const filtered = "/book/show/2767052?reviewFilters=%7B%22languageCode%22%3A%22en%22%7D"
	if r.Allowed(filtered) {
		t.Error("a reviewFilters query was allowed, so matching ignored the query string")
	}
	if m := r.Check(filtered); m == nil || m.Pattern != "/*reviewFilters" {
		t.Errorf("matched %v, want /*reviewFilters", m)
	}
}

func TestLongestMatchWins(t *testing.T) {
	r := ParseRobots([]byte(`
User-agent: *
Disallow: /a
Allow: /a/b
Disallow: /a/b/c
`), "x")
	cases := []struct {
		path string
		want bool
	}{
		{"/a", false},
		{"/a/x", false},
		{"/a/b", true},
		{"/a/b/x", true},
		{"/a/b/c", false},
		{"/a/b/c/d", false},
		{"/z", true},
	}
	for _, c := range cases {
		if got := r.Allowed(c.path); got != c.want {
			t.Errorf("Allowed(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestAllowBeatsDisallowAtEqualLength(t *testing.T) {
	r := ParseRobots([]byte("User-agent: *\nDisallow: /x\nAllow: /x\n"), "x")
	if !r.Allowed("/x/y") {
		t.Error("equal-length Allow and Disallow: Allow should win")
	}
}

func TestEmptyDisallowMeansEverythingAllowed(t *testing.T) {
	r := ParseRobots([]byte("User-agent: *\nDisallow:\n"), "x")
	if !r.Allowed("/anything") {
		t.Error("an empty Disallow became a rule matching every path")
	}
}

func TestWildcardAndAnchor(t *testing.T) {
	cases := []struct {
		pattern, path string
		want          bool
	}{
		{"/a", "/a/b", true},
		{"/a", "/b", false},
		{"/*.pdf", "/docs/x.pdf", true},
		{"/*.pdf", "/docs/x.pdf?v=1", true},
		{"/*.pdf$", "/docs/x.pdf", true},
		{"/*.pdf$", "/docs/x.pdf?v=1", false},
		{"/a$", "/a", true},
		{"/a$", "/a/b", false},
		{"/a/*/c", "/a/b/c", true},
		{"/a/*/c", "/a/b/d", false},
		{"/*reviewFilters", "/book/show/1?reviewFilters=x", true},
		{"/*reviewFilters", "/book/show/1", false},
		{"/a*", "/ab", true},
		{"/", "/anything", true},
	}
	for _, c := range cases {
		if got := matchPattern(c.pattern, c.path); got != c.want {
			t.Errorf("matchPattern(%q, %q) = %v, want %v", c.pattern, c.path, got, c.want)
		}
	}
}

func TestSitemapsCollected(t *testing.T) {
	r := liveRobots(t)
	if len(r.Sitemaps) < 10 {
		t.Fatalf("got %d sitemaps, want the full advertised set", len(r.Sitemaps))
	}
	var found bool
	for _, s := range r.Sitemaps {
		if strings.HasSuffix(s, "siteindex.author.xml") {
			found = true
		}
	}
	if !found {
		t.Error("siteindex.author.xml missing from the parsed sitemaps")
	}
}

// TestNoDisallowedByDefault walks every registered op against the real rules.
//
// This is the test that would have caught v0.2.0. It does not read the code,
// it reads the registry, so an op added later is covered the moment it is
// registered. The declaration on the op and the live rules have to agree in
// both directions: an op marked allowed that the rules refuse is a bug we are
// shipping, and an op marked disallowed that the rules permit means the site
// relaxed and we are refusing for no reason.
func TestNoDisallowedByDefault(t *testing.T) {
	r := liveRobots(t)
	for _, o := range Ops {
		if o.Name == "robots" {
			continue // fetching the rules cannot be gated on the rules
		}
		path := SamplePath(o)
		allowed := r.Allowed(path)
		switch {
		case o.Disallowed && allowed:
			t.Errorf("op %q (%s) is marked Disallowed but %s is permitted: the rules relaxed",
				o.Name, o.Surface, path)
		case !o.Disallowed && !allowed:
			t.Errorf("op %q (%s) is read by default but %s is refused by %v",
				o.Name, o.Surface, path, r.Check(path))
		}
	}
}

// TestEverySurfaceHasSampleArgs keeps the policy test honest. An op with no
// sample arguments renders an empty path, which every rule set permits, so it
// would pass TestNoDisallowedByDefault while testing nothing at all.
func TestEverySurfaceHasSampleArgs(t *testing.T) {
	for _, o := range Ops {
		if _, ok := SampleArgs[o.Name]; !ok {
			t.Errorf("op %q has no SampleArgs, so the policy test cannot render a path for it", o.Name)
			continue
		}
		if o.Name == "robots" {
			continue
		}
		if p := SamplePath(o); p == "" || p == "/" {
			t.Errorf("op %q renders %q from its sample args, which tests nothing", o.Name, p)
		}
	}
}

func TestSurfacesAreOrdered(t *testing.T) {
	got := Surfaces()
	for i := 1; i < len(got); i++ {
		if surfaceNum(got[i-1]) >= surfaceNum(got[i]) {
			t.Fatalf("surfaces out of order: %v", got)
		}
	}
}

// --- the fetcher ---

func TestFetcherCachesAndExpires(t *testing.T) {
	calls := 0
	body := []byte("User-agent: *\nDisallow: /x\n")
	now := time.Unix(1_700_000_000, 0)

	f := NewRobotsFetcher(DefaultUserAgent(), func(context.Context, string) ([]byte, int, error) {
		calls++
		return body, 200, nil
	})
	f.now = func() time.Time { return now }

	for i := 0; i < 3; i++ {
		if _, err := f.Get(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Errorf("fetched %d times within the TTL, want 1", calls)
	}

	now = now.Add(RobotsTTL + time.Second)
	if _, err := f.Get(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Errorf("fetched %d times after the TTL, want 2", calls)
	}
}

// TestFetchFailureStopsEverything is the exit-8 behaviour.
//
// There is no compiled-in fallback copy and no flag that turns this into a
// proceed. A tool that carries on when it cannot check permission is the thing
// this whole version exists to avoid.
func TestFetchFailureStopsEverything(t *testing.T) {
	f := NewRobotsFetcher(DefaultUserAgent(), func(context.Context, string) ([]byte, int, error) {
		return nil, 0, errors.New("dial tcp: no route to host")
	})
	_, err := f.Get(context.Background())
	if !errors.Is(err, ErrNoRobots) {
		t.Fatalf("err = %v, want ErrNoRobots", err)
	}

	f2 := NewRobotsFetcher(DefaultUserAgent(), func(context.Context, string) ([]byte, int, error) {
		return []byte("<html>oops</html>"), 503, nil
	})
	if _, err := f2.Get(context.Background()); !errors.Is(err, ErrNoRobots) {
		t.Fatalf("err = %v, want ErrNoRobots on HTTP 503", err)
	}
}

// A 404 is a real answer: the site publishes no rules, so nothing is refused.
func TestFetch404MeansNoRules(t *testing.T) {
	f := NewRobotsFetcher(DefaultUserAgent(), func(context.Context, string) ([]byte, int, error) {
		return nil, 404, nil
	})
	r, err := f.Get(context.Background())
	if err != nil {
		t.Fatalf("404 should not be an error: %v", err)
	}
	if !r.Allowed("/anything") {
		t.Error("no published rules should mean nothing is refused")
	}
}

func TestDisallowedErrorNamesRuleFlagAndAlternative(t *testing.T) {
	e := &DisallowedError{
		Op:     "search",
		Path:   "/search?q=hunger+games",
		Rule:   Rule{Pattern: "/search"},
		Source: RobotsTxtURL,
		Alt:    "suggest",
	}
	msg := e.Error()
	for _, want := range []string{"/search", "Disallow: /search", "--no-robots", "goodread suggest", "goodread robots"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message does not mention %q:\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "q=hunger") {
		t.Errorf("the query leaked into the refusal message:\n%s", msg)
	}
	if !errors.Is(e, ErrDisallowed) {
		t.Error("DisallowedError should unwrap to ErrDisallowed")
	}
}
