package goodread

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// The tests in this file are about what the tool will not do. They are
// deliberately blunt and deliberately hard to delete quietly: each one names
// the property it holds in its own name, so a diff that removes one is
// obvious in review.

// TestNoRobotsNotInConfig walks the Config struct and asserts nothing but the
// flag can reach NoRobots.
//
// A config key would make the override ambient. Somebody sets it once, and a
// month later a crawl is reading disallowed paths and nobody remembers why.
func TestNoRobotsNotInConfig(t *testing.T) {
	tp := reflect.TypeOf(Config{})
	for i := 0; i < tp.NumField(); i++ {
		f := tp.Field(i)
		// A serialisation tag on NoRobots would be the mechanism by which a
		// config file could set it, so its absence is the property under test.
		for _, key := range []string{"json", "toml", "yaml", "mapstructure", "env"} {
			if tag, ok := f.Tag.Lookup(key); ok && f.Name == "NoRobots" {
				t.Errorf("Config.NoRobots carries a %s tag (%q), so a config file could set it", key, tag)
			}
		}
	}
	if _, ok := tp.FieldByName("CookiePath"); ok {
		t.Error("Config.CookiePath is back: this tool does not borrow signed-in sessions")
	}
}

// TestNoRobotsNotInEnv sets every environment variable the tool reads and
// asserts the override stays off.
func TestNoRobotsNotInEnv(t *testing.T) {
	for _, k := range []string{
		"GOODREAD_NO_ROBOTS", "GOODREAD_NOROBOTS", "NO_ROBOTS", "ROBOTS",
		"GOODREAD_ROBOTS", "GOODREAD_DATA_DIR", "GOODREAD_DELAY", "GOODREAD_USER_AGENT",
	} {
		t.Setenv(k, "1")
	}
	if DefaultConfig().NoRobots {
		t.Error("an environment variable turned on --no-robots")
	}
}

// TestDefaultConfigIsPolite pins the defaults themselves, since every other
// guarantee here is stated relative to them.
func TestDefaultConfigIsPolite(t *testing.T) {
	c := DefaultConfig()
	if c.NoRobots {
		t.Error("NoRobots defaults to true")
	}
	if c.Delay < MinDelay {
		t.Errorf("default delay %s is below the floor %s", c.Delay, MinDelay)
	}
	if c.Workers > MaxWorkers {
		t.Errorf("default workers %d exceeds %d", c.Workers, MaxWorkers)
	}
	if !strings.HasPrefix(c.UserAgent, "goodread/") || !strings.Contains(c.UserAgent, "github.com/tamnd") {
		t.Errorf("default user agent %q does not name the tool and where to find it", c.UserAgent)
	}
	if strings.Contains(c.UserAgent, "Mozilla") {
		t.Errorf("default user agent impersonates a browser: %q", c.UserAgent)
	}
}

// TestPaceFloorHolds times real requests with the override on.
//
// This is the guarantee that stops --no-robots from becoming a hammer, so it
// gets a timing test against a real server rather than a unit test of the
// clamp function. --no-robots changes which paths are reachable and changes
// nothing about how fast.
func TestPaceFloorHolds(t *testing.T) {
	if testing.Short() {
		t.Skip("timing test")
	}
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	cfg := DefaultConfig()
	cfg.Delay = 10 * time.Millisecond // far below the floor
	cfg.NoRobots = true               // the override must not raise the ceiling
	cfg.Retries = 1
	c := NewClient(cfg)

	const n = 5
	start := time.Now()
	for i := 0; i < n; i++ {
		if _, _, err := c.Fetch(context.Background(), srv.URL); err != nil {
			t.Fatal(err)
		}
	}
	elapsed := time.Since(start)

	// n requests through a limiter of one per MinDelay with a burst of one:
	// the first leaves immediately, so the floor is (n-1) intervals.
	want := time.Duration(n-1) * MinDelay
	if elapsed < want-100*time.Millisecond {
		t.Errorf("%d requests took %s, want at least %s: the pace floor did not hold", n, elapsed, want)
	}
	if hits != n {
		t.Errorf("server saw %d requests, want %d", hits, n)
	}
}

// TestBurstIsOne is the other half of the floor. A burst of N lets N requests
// leave at once, which is concurrency wearing a different name.
func TestBurstIsOne(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Delay = time.Hour
	if b := newLimiter(cfg).Burst(); b != 1 {
		t.Errorf("limiter burst = %d, want 1", b)
	}
}

func TestClampDelay(t *testing.T) {
	cases := []struct {
		in      time.Duration
		want    time.Duration
		clamped bool
	}{
		{0, DefaultDelay, false},
		{-time.Second, DefaultDelay, false},
		{time.Millisecond, MinDelay, true},
		{MinDelay, MinDelay, false},
		{5 * time.Second, 5 * time.Second, false},
	}
	for _, c := range cases {
		got, clamped := ClampDelay(c.in)
		if got != c.want || clamped != c.clamped {
			t.Errorf("ClampDelay(%s) = (%s, %v), want (%s, %v)", c.in, got, clamped, c.want, c.clamped)
		}
	}
}

// TestNoCookieHeader asserts no request carries one.
//
// v0.2.0 could load a Netscape cookie jar and borrow a signed-in session.
// Reading a member-only page with somebody's session is a different act from
// reading a public one, and this tool does only the second.
func TestNoCookieHeader(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.WriteHeader(200)
	}))
	defer srv.Close()

	c := NewClient(DefaultConfig())
	if _, _, err := c.Fetch(context.Background(), srv.URL); err != nil {
		t.Fatal(err)
	}
	if v := got.Get("Cookie"); v != "" {
		t.Errorf("request carried a Cookie header: %q", v)
	}
	if c.http.Jar != nil {
		t.Error("the client has a cookie jar")
	}
	if ua := got.Get("User-Agent"); !strings.HasPrefix(ua, "goodread/") {
		t.Errorf("User-Agent = %q, want an honest one", ua)
	}
}

// TestNoBrowserDependency reads go.mod.
//
// The thing worth preventing is not writing browser automation on purpose, it
// is acquiring it by accident through a dependency and then finding the tool
// can suddenly solve challenges. A module you cannot add without failing a
// test is a decision somebody has to make deliberately.
func TestNoBrowserDependency(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	forbidden := []string{
		"chromedp",
		"go-rod/rod",
		"playwright-go",
		"mafredri/cdp",
		"CycleTLS",
		"chromedp/cdproto",
		"tebeka/selenium",
	}
	mod := string(b)
	for _, f := range forbidden {
		if strings.Contains(mod, f) {
			t.Errorf("go.mod depends on %s: this tool reads pages, it does not drive a browser", f)
		}
	}
}

// TestNoChallengeHandling asserts no code path tries to solve one.
//
// Goodreads does not currently present a bot challenge. If it starts, that is
// a stop signal and not a problem to solve, because a challenge is an access
// control and a robots.txt is not. The client detects the AWS WAF header and
// reports it as blocked, which is the correct and only response.
func TestNoChallengeHandling(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("x-amzn-waf-action", "challenge")
		w.WriteHeader(202)
	}))
	defer srv.Close()

	cfg := DefaultConfig()
	cfg.Retries = 1
	_, _, err := NewClient(cfg).Fetch(context.Background(), srv.URL)
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("err = %v, want ErrBlocked: a challenge is a stop signal", err)
	}

	// And nothing in the package should be reaching for a solver.
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		src, err := os.ReadFile(e.Name())
		if err != nil {
			t.Fatal(err)
		}
		for _, bad := range []string{"turnstile", "hcaptcha", "recaptcha", "2captcha", "anticaptcha"} {
			if strings.Contains(strings.ToLower(string(src)), bad) {
				t.Errorf("%s mentions %q", e.Name(), bad)
			}
		}
	}
}

// --- the override, end to end ---

// testClient points a client at a local server with a captured rule set, so
// the override can be exercised without touching goodreads.com.
func testClient(t *testing.T, cfg Config, h http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	c := NewClient(cfg)
	c.SetBaseURL(srv.URL)
	b, err := os.ReadFile("testdata/robots.txt")
	if err != nil {
		t.Fatal(err)
	}
	r := ParseRobots(b, DefaultUserAgent())
	r.Source = RobotsTxtURL
	r.FetchedAt = time.Now()
	c.robots.Set(r)
	return c, srv
}

func TestDoRefusesDisallowedByDefault(t *testing.T) {
	c, _ := testClient(t, DefaultConfig(), func(w http.ResponseWriter, _ *http.Request) {
		t.Error("a disallowed op reached the network")
	})
	_, _, err := c.Do(context.Background(), "search", "hunger games")
	if !errors.Is(err, ErrDisallowed) {
		t.Fatalf("err = %v, want ErrDisallowed", err)
	}
	var de *DisallowedError
	if !errors.As(err, &de) {
		t.Fatal("error does not carry the rule")
	}
	if de.Rule.Pattern != "/search" {
		t.Errorf("rule = %q, want /search", de.Rule.Pattern)
	}
	if de.Alt != "suggest" {
		t.Errorf("alt = %q, want suggest: a refusal should offer something", de.Alt)
	}
}

func TestDoAllowsDisallowedWithOverride(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NoRobots = true
	cfg.Delay = MinDelay

	var reached string
	c, _ := testClient(t, cfg, func(w http.ResponseWriter, r *http.Request) {
		reached = r.URL.Path
		_, _ = w.Write([]byte("ok"))
	})

	var warn bytes.Buffer
	c.SetWarnWriter(&warn)

	body, code, err := c.Do(context.Background(), "search", "x")
	if err != nil {
		t.Fatalf("--no-robots did not lift the refusal: %v", err)
	}
	if code != 200 || string(body) != "ok" {
		t.Errorf("got (%d, %q), want (200, %q)", code, body, "ok")
	}
	if reached != "/search" {
		t.Errorf("server saw %q, want /search", reached)
	}

	if !strings.Contains(warn.String(), "--no-robots") {
		t.Errorf("no warning was printed: %q", warn.String())
	}
	if !strings.Contains(warn.String(), "Disallow: /search") {
		t.Errorf("the warning does not name the rule: %q", warn.String())
	}
}

// TestWarnOnce holds the once-per-process rule. A crawl would otherwise print
// the warning thousands of times, and a warning nobody reads is not a warning.
func TestWarnOnce(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NoRobots = true
	c, _ := testClient(t, cfg, func(w http.ResponseWriter, _ *http.Request) {})

	var warn bytes.Buffer
	c.SetWarnWriter(&warn)
	for i := 0; i < 3; i++ {
		c.warn("/search?q=x", Rule{Pattern: "/search"})
	}
	if n := strings.Count(warn.String(), "warning:"); n != 1 {
		t.Errorf("printed the warning %d times, want 1", n)
	}
}

// TestFetchEnforcesForHandBuiltURLs is the regression test for the actual
// v0.2.0 defect.
//
// The bug was not that somebody wrote a disallowed URL on purpose. It was that
// three call sites built their own URLs and nothing sat between them and the
// network. So the check lives in Fetch, and this test goes around the registry
// entirely to prove a caller cannot get past it by not using Do.
func TestFetchEnforcesForHandBuiltURLs(t *testing.T) {
	c, srv := testClient(t, DefaultConfig(), func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("a disallowed hand-built URL reached the network: %s", r.URL)
	})

	for _, path := range []string{
		"/search?q=hunger+games&search_type=books",
		"/review/list/1?shelf=read",
		"/review/list_rss/1?shelf=read",
		"/review/show/2892457",
		"/book/reviews/2767052",
		"/work/2792775",
		"/book/show/2767052?reviewFilters=%7B%7D",
	} {
		_, _, err := c.Fetch(context.Background(), srv.URL+path)
		if !errors.Is(err, ErrDisallowed) {
			t.Errorf("Fetch(%s) err = %v, want ErrDisallowed", path, err)
		}
	}
}

// And the allowed ones must still go through, or the fix is worse than the bug.
func TestFetchAllowsTheAllowedOnes(t *testing.T) {
	var seen []string
	c, srv := testClient(t, DefaultConfig(), func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Path)
		_, _ = w.Write([]byte("ok"))
	})
	for _, path := range []string{
		"/book/show/2767052",
		"/author/show/153394",
		"/work/editions/2792775",
		"/work/quotes/2792775",
		"/book/auto_complete?format=json&q=x",
	} {
		if _, _, err := c.Fetch(context.Background(), srv.URL+path); err != nil {
			t.Errorf("Fetch(%s) = %v, want success", path, err)
		}
	}
	if len(seen) != 5 {
		t.Errorf("server saw %d requests, want 5: %v", len(seen), seen)
	}
}

// Off-site URLs are none of Goodreads' business, so the checker leaves them
// alone rather than applying one site's rules to another's.
func TestOffSiteURLsPassThrough(t *testing.T) {
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer other.Close()

	c, _ := testClient(t, DefaultConfig(), func(w http.ResponseWriter, _ *http.Request) {})
	// A path Goodreads disallows, on a host that has never heard of it.
	if _, _, err := c.Fetch(context.Background(), other.URL+"/search?q=x"); err != nil {
		t.Errorf("off-site fetch was refused by goodreads' rules: %v", err)
	}
}

// TestSuggestIsAllowed pins the finding that makes search work by default.
//
// /book/auto_complete is public JSON, carries no key, and no rule refuses it.
// The spec assumed there was no allowed search route and there is one, so this
// test exists to notice if that ever stops being true.
func TestSuggestIsAllowed(t *testing.T) {
	r := liveRobots(t)
	op, ok := LookupOp("suggest")
	if !ok {
		t.Fatal("no suggest op")
	}
	p := SamplePath(op)
	if !r.Allowed(p) {
		t.Fatalf("%s is refused by %v: the allowed search route is gone", p, r.Check(p))
	}
	if op.Disallowed {
		t.Error("suggest is marked Disallowed")
	}
}
