package goodread

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The crawl tests run against the golden captures over a local server.
//
// A rewriting transport rather than SetBaseURL, because the crawler builds its
// URLs with BookURL and AuthorURL, which are the real ones by design: the
// frontier is keyed by the canonical URL and a test that changed how those are
// built would be testing something the tool does not do.

// captureServer serves the golden captures at the paths they were captured from.
func captureServer(t *testing.T) *httptest.Server {
	t.Helper()
	pages := map[string]string{
		"/book/show/2767052":     "book_show_2767052.html.gz",
		"/book/show/1885":        "book_show_1885.html.gz",
		"/author/show/1077326":   "author_show_1077326.html.gz",
		"/series/45175":          "series_45175.html.gz",
		"/list/show/1":           "list_show_1.html.gz",
		"/genres/fantasy":        "genres_fantasy.html.gz",
		"/work/editions/2792775": "work_editions_2792775.html.gz",
	}
	var mu = make(chan struct{}, 1)
	mu <- struct{}{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-mu
		defer func() { mu <- struct{}{} }()
		if r.URL.Path == "/robots.txt" {
			b, err := os.ReadFile("testdata/robots.txt")
			if err != nil {
				t.Error(err)
			}
			_, _ = w.Write(b)
			return
		}
		name, ok := pages[strings.SplitN(r.URL.Path, "?", 2)[0]]
		if !ok {
			// A 404 is a real outcome for a crawl and the tests want one, so
			// an unknown path answers like the site would rather than failing
			// the test.
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write(readCapture(t, name))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// rewriteHost sends every request to the test server, whatever it was addressed to.
type rewriteHost struct {
	to   string
	next http.RoundTripper
	hits *int
}

func (rw rewriteHost) RoundTrip(req *http.Request) (*http.Response, error) {
	*rw.hits++
	u := *req.URL
	target, _ := http.NewRequest(req.Method, rw.to+u.RequestURI(), nil)
	target.Header = req.Header
	return rw.next.RoundTrip(target.WithContext(req.Context()))
}

// crawlHarness wires a crawler onto the captures with nothing pointed at the site.
func crawlHarness(t *testing.T) (*Crawler, *Store, *int) {
	t.Helper()
	srv := captureServer(t)
	hits := new(int)

	cfg := DefaultConfig()
	cfg.Delay = MinDelay
	c := NewClient(cfg)
	c.http = &http.Client{Timeout: 30 * time.Second, Transport: rewriteHost{to: srv.URL, next: http.DefaultTransport, hits: hits}}

	b, err := os.ReadFile("testdata/robots.txt")
	if err != nil {
		t.Fatal(err)
	}
	r := ParseRobots(b, DefaultUserAgent())
	r.Source = RobotsTxtURL
	r.FetchedAt = time.Now()
	c.robots.Set(r)

	st, err := OpenStore(filepath.Join(t.TempDir(), "goodread.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	return &Crawler{Client: c, Store: st, Config: cfg, Depth: 1, BookDepth: DepthMeta}, st, hits
}

func TestSeedTakesAURIAURLOrABareID(t *testing.T) {
	c, st, _ := crawlHarness(t)

	added, skipped, err := c.Seed([]string{
		"gr:author/1077326",
		"https://www.goodreads.com/book/show/2767052-the-hunger-games",
		"1885",
		"",
		"# a comment line, which a seed file will have",
		"not a goodreads thing at all",
	})
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	if added != 3 {
		t.Errorf("seeded %d, want the uri, the url and the bare id", added)
	}
	if len(skipped) != 1 || skipped[0] != "not a goodreads thing at all" {
		t.Errorf("skipped = %v, want the one thing that resolves to no page", skipped)
	}

	// Skipped and not dropped, and what did land is keyed by URI, so the URL
	// with a slug on it and the bare URI are the same row.
	pending, err := st.NextFrontier(10)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, it := range pending {
		got[it.URI] = true
	}
	for _, want := range []string{"gr:author/1077326", "gr:book/2767052", "gr:book/1885"} {
		if !got[want] {
			t.Errorf("%s is not on the frontier: %v", want, got)
		}
	}
}

func TestSeedingTheSameBookTwiceIsOneRow(t *testing.T) {
	c, st, _ := crawlHarness(t)
	if _, _, err := c.Seed([]string{
		"https://www.goodreads.com/book/show/2767052",
		"https://www.goodreads.com/book/show/2767052-the-hunger-games",
		"gr:book/2767052",
	}); err != nil {
		t.Fatalf("Seed: %v", err)
	}
	pending, err := st.NextFrontier(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 {
		t.Fatalf("three spellings of one book made %d rows, want 1: %v", len(pending), pending)
	}
}

func TestDryRunPlansWithoutReadingAnything(t *testing.T) {
	c, _, hits := crawlHarness(t)
	if _, _, err := c.Seed([]string{"gr:book/2767052", "gr:book/1885"}); err != nil {
		t.Fatalf("Seed: %v", err)
	}
	plan, err := c.Plan()
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if plan.Pending != 2 {
		t.Errorf("plan says %d pending, want 2", plan.Pending)
	}
	if plan.Pace < MinDelay {
		t.Errorf("plan pace is %s, under the %s floor", plan.Pace, MinDelay)
	}
	if plan.Duration < 2*MinDelay {
		t.Errorf("two pages at %s came to %s", plan.Pace, plan.Duration)
	}
	if *hits != 0 {
		t.Errorf("planning made %d requests, and the point of --dry-run is that it makes none", *hits)
	}
}

func TestCrawlReadsTheSeedAndStoresIt(t *testing.T) {
	c, st, _ := crawlHarness(t)
	c.Depth = 0
	if _, _, err := c.Seed([]string{"gr:book/2767052"}); err != nil {
		t.Fatalf("Seed: %v", err)
	}
	stats, err := c.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.ByKind["book"] != 1 {
		t.Fatalf("read %v, want one book", stats.ByKind)
	}
	if stats.Errors != 0 {
		t.Errorf("%d errors on a page that is in testdata", stats.Errors)
	}
	if _, err := st.Get("book", "2767052"); err != nil {
		t.Errorf("the book was read but not stored: %v", err)
	}

	// Depth 0 reads the seeds and stops, so the author the book named is on
	// the graph but not on the frontier.
	if _, _, err := st.CrawledDepth("gr:author/153394"); err == nil {
		t.Error("depth 0 queued a neighbour")
	}
}

func TestCrawlFollowsTheEdgesOutOneHop(t *testing.T) {
	c, st, _ := crawlHarness(t)
	c.Depth = 1
	c.Max = 3
	if _, _, err := c.Seed([]string{"gr:book/2767052"}); err != nil {
		t.Fatalf("Seed: %v", err)
	}
	if _, err := c.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// The neighbours came out of the edge table rather than out of the HTML,
	// so what got queued is what the record actually said.
	edges, err := st.Edges("gr:book/2767052", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) == 0 {
		t.Fatal("the book was stored with no edges, so there was nothing to expand into")
	}
	queued := 0
	for _, e := range edges {
		if _, _, _, ok := URIToURL(e.Dst); !ok {
			continue
		}
		if _, _, err := st.CrawledDepth(e.Dst); err == nil {
			queued++
		}
	}
	if queued == 0 {
		t.Errorf("none of the book's %d neighbours reached the frontier", len(edges))
	}
}

func TestMaxStopsTheCrawlWhereItSays(t *testing.T) {
	c, _, _ := crawlHarness(t)
	c.Depth = 2
	c.Max = 2
	if _, _, err := c.Seed([]string{"gr:book/2767052", "gr:book/1885", "gr:author/1077326"}); err != nil {
		t.Fatalf("Seed: %v", err)
	}
	stats, err := c.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := stats.Requests + stats.Cached; got != 2 {
		t.Errorf("--max 2 read %d pages", got)
	}
}

func TestAnInterruptedCrawlPicksUpWhereItStopped(t *testing.T) {
	c, st, _ := crawlHarness(t)
	c.Depth = 0
	if _, _, err := c.Seed([]string{"gr:book/2767052", "gr:book/1885"}); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	// First run stops after one page, the way a Ctrl-C would.
	c.Max = 1
	first, err := c.Run(context.Background())
	if err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if first.Requests+first.Cached != 1 {
		t.Fatalf("first run read %d pages, want 1", first.Requests+first.Cached)
	}
	if first.Queued != 1 {
		t.Errorf("first run left %d on the frontier, want the other book", first.Queued)
	}

	// The state is in the store, so a second crawler over the same store
	// finishes the job rather than starting it.
	again := &Crawler{Client: c.Client, Store: st, Config: c.Config, Depth: 0, BookDepth: DepthMeta}
	second, err := again.Run(context.Background())
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if second.Requests+second.Cached != 1 {
		t.Errorf("the resumed run read %d pages, want the one that was left", second.Requests+second.Cached)
	}
	if second.Queued != 0 {
		t.Errorf("%d still pending after both books were read", second.Queued)
	}
	for _, id := range []string{"2767052", "1885"} {
		if _, err := st.Get("book", id); err != nil {
			t.Errorf("book %s was not stored across the two runs: %v", id, err)
		}
	}
}

func TestAPageThatFailsDoesNotStopTheCrawl(t *testing.T) {
	c, st, _ := crawlHarness(t)
	c.Depth = 0
	// The middle one 404s on the test server, the way a dead id does on the site.
	if _, _, err := c.Seed([]string{"gr:book/2767052", "gr:book/999999999", "gr:book/1885"}); err != nil {
		t.Fatalf("Seed: %v", err)
	}
	stats, err := c.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Errors != 1 {
		t.Errorf("errors = %d, want the one dead id", stats.Errors)
	}
	if stats.ByKind["book"] != 2 {
		t.Errorf("read %v, want the two books either side of the failure", stats.ByKind)
	}

	// And what failed is recorded with its reason, because "1 error" is not
	// something anybody can act on.
	items, causes, err := st.FrontierErrors(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].URI != "gr:book/999999999" {
		t.Fatalf("the failure was not recorded: %v", items)
	}
	if causes[0] == "" {
		t.Error("the failure was recorded with no reason")
	}
}

func TestCancellingACrawlIsNotAFailure(t *testing.T) {
	c, _, _ := crawlHarness(t)
	if _, _, err := c.Seed([]string{"gr:book/2767052", "gr:book/1885"}); err != nil {
		t.Fatalf("Seed: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stats, err := c.Run(ctx)
	if err != nil {
		t.Fatalf("a cancelled crawl came back as an error: %v", err)
	}
	if stats.Requests != 0 {
		t.Errorf("a crawl cancelled before it started read %d pages", stats.Requests)
	}
}

func TestNothingWithoutAPageIsEverQueued(t *testing.T) {
	// A place, a character and an award are strings on a work rather than
	// things with URLs, and a quote is read through the work it hangs off.
	for _, uri := range []string{
		"gr:place/panem", "gr:character/katniss-everdeen", "gr:award/hugo-award", "gr:quote/12345",
	} {
		if _, _, _, ok := URIToURL(uri); ok {
			t.Errorf("%s came back with a page to read", uri)
		}
	}
	for _, uri := range []string{
		"gr:book/2767052", "gr:author/153394", "gr:series/45175",
		"gr:list/1", "gr:genre/fantasy", "gr:work/2792775", "gr:user/1",
	} {
		if _, _, u, ok := URIToURL(uri); !ok || u == "" {
			t.Errorf("%s has no page and it should", uri)
		}
	}
}
