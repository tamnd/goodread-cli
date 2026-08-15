package goodread

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// The crawler, per 05_commands.md section 6.
//
// One connection, the 2 second pace, and no parallelism flag. A crawler that
// can be told to open ten connections will be, and the site does not get a say,
// so the option is not offered rather than defaulted to one. The pace itself is
// the client's, which means it is the same limiter every other command shares
// and there is no second path that could be faster.
//
// Expansion is through the graph rather than through the HTML. A page is read,
// the record is stored, the store folds it into nodes and edges, and the
// neighbours of the node just written become the next depth. That means the
// crawler does not need to know what links a book page carries, and a surface
// that starts naming a new relationship widens the crawl for free.

// CrawlStats is what a crawl reports as it goes.
type CrawlStats struct {
	ByKind   map[string]int `json:"by_kind"`
	Requests int            `json:"requests"`
	Cached   int            `json:"cached"`
	Errors   int            `json:"errors"`
	Queued   int            `json:"queued"`
	Elapsed  time.Duration  `json:"elapsed"`
}

// Kinds lists what was read, most first, for a progress line.
func (s CrawlStats) Kinds() []string {
	out := make([]string, 0, len(s.ByKind))
	for k := range s.ByKind {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		if s.ByKind[out[i]] != s.ByKind[out[j]] {
			return s.ByKind[out[i]] > s.ByKind[out[j]]
		}
		return out[i] < out[j]
	})
	return out
}

func (s CrawlStats) String() string {
	var b strings.Builder
	for _, k := range s.Kinds() {
		fmt.Fprintf(&b, "%s %d  ", k, s.ByKind[k])
	}
	fmt.Fprintf(&b, "| requests %d  cached %d  errors %d  queued %d  elapsed %s",
		s.Requests, s.Cached, s.Errors, s.Queued, s.Elapsed.Round(time.Second))
	return b.String()
}

// Crawler walks the graph outward from a set of seeds.
type Crawler struct {
	Client *Client
	Store  *Store
	Cache  *Cache
	Config Config

	// Depth is how many hops from a seed the crawl goes. 0 reads the seeds and
	// stops, which is the cheapest useful thing this can do.
	Depth int

	// Max stops the crawl after this many pages, 0 for no limit. Not a
	// substitute for --depth: this is the flag for "read some of it and let me
	// look", and depth is the one that says what the crawl is.
	Max int

	// BookDepth is how much of each book page to read, per 02_extraction.md.
	BookDepth Depth

	// Progress is called after each page with the running totals.
	Progress func(CrawlStats)
}

// CrawlPlan is what --dry-run prints.
//
// The point of it is finding out that a crawl is twelve hours before starting
// it rather than at hour eleven. The request count is a floor and says so: a
// depth 2 crawl of an author reaches books whose own neighbours are not known
// until they have been read.
type CrawlPlan struct {
	Seeds    []FrontierItem `json:"seeds"`
	Depth    int            `json:"depth"`
	Pending  int            `json:"pending"`
	Pace     time.Duration  `json:"pace"`
	Requests int            `json:"requests_at_least"`
	Duration time.Duration  `json:"duration_at_least"`
	Skipped  []string       `json:"skipped,omitempty"`
}

// Seed adds the starting points to the frontier.
//
// Takes gr: URIs, a Goodreads URL or a bare id, because somebody with a URL in
// their clipboard should not have to convert it by hand first. Anything that
// resolves to no page is returned in the skipped list rather than dropped, so a
// typo in a seed is visible before the crawl runs for an hour.
func (c *Crawler) Seed(seeds []string) (added int, skipped []string, err error) {
	for _, raw := range seeds {
		raw = strings.TrimSpace(raw)
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		uri := raw
		if !strings.HasPrefix(uri, "gr:") {
			kind, id := Classify(raw)
			if kind == "" || id == "" {
				skipped = append(skipped, raw)
				continue
			}
			uri = NodeURI(kind, id)
		}
		kind, _, pageURL, ok := URIToURL(uri)
		if !ok {
			skipped = append(skipped, raw)
			continue
		}
		if err := c.Store.EnqueueURI(uri, pageURL, kind, 0); err != nil {
			return added, skipped, err
		}
		added++
	}
	return added, skipped, nil
}

// Plan describes the crawl without reading anything.
func (c *Crawler) Plan() (CrawlPlan, error) {
	pending, err := c.Store.NextFrontier(1000)
	if err != nil {
		return CrawlPlan{}, err
	}
	pace, _ := ClampDelay(c.Config.Delay)
	p := CrawlPlan{
		Seeds:    pending,
		Depth:    c.Depth,
		Pending:  len(pending),
		Pace:     pace,
		Requests: len(pending),
	}
	if c.Max > 0 && p.Requests > c.Max {
		p.Requests = c.Max
	}
	p.Duration = time.Duration(p.Requests) * pace
	return p, nil
}

// Run works the frontier until it is empty, the budget is spent or ctx ends.
//
// Sequential on purpose. There is no worker pool and no flag that would make
// one, and the pace is the client's shared limiter, so this cannot be faster
// than any other command no matter what it is asked for.
func (c *Crawler) Run(ctx context.Context) (CrawlStats, error) {
	start := time.Now()
	stats := CrawlStats{ByKind: map[string]int{}}

	for c.Max <= 0 || stats.Requests+stats.Cached < c.Max {
		if err := ctx.Err(); err != nil {
			// An interrupted crawl is a normal outcome and not a failure. The
			// frontier is in the store, so what it got to is kept and the same
			// command picks it up.
			stats.Elapsed = time.Since(start)
			return stats, nil
		}
		batch, err := c.Store.NextFrontier(1)
		if err != nil {
			return stats, err
		}
		if len(batch) == 0 {
			break
		}
		it := batch[0]

		cached := c.Cache != nil && c.Cache.Fresh(it.URL) && !c.Config.Refresh && !c.Config.NoCache
		if err := c.read(ctx, it); err != nil {
			// A page that will not read is one page. The crawl carries on,
			// because stopping a twelve hour crawl on a 404 in hour two is
			// worse than finishing with three failures recorded.
			stats.Errors++
			_ = c.Store.MarkCrawled(it.URI, "failed", err)
		} else {
			stats.ByKind[it.Kind]++
			_ = c.Store.MarkCrawled(it.URI, "done", nil)
			if it.Depth < c.Depth {
				if err := c.expand(it); err != nil {
					return stats, err
				}
			}
		}
		if cached {
			stats.Cached++
		} else {
			stats.Requests++
		}

		q, _ := c.Store.FrontierStats()
		stats.Queued = q["pending"]
		stats.Elapsed = time.Since(start)
		if c.Progress != nil {
			c.Progress(stats)
		}
	}
	stats.Elapsed = time.Since(start)
	return stats, nil
}

// read fetches one node and stores it.
func (c *Crawler) read(ctx context.Context, it FrontierItem) error {
	_, id, _, ok := URIToURL(it.URI)
	if !ok {
		return fmt.Errorf("no page for %s", it.URI)
	}
	switch it.Kind {
	case "book":
		rec, err := c.Client.GetBookRecord(ctx, id, c.bookDepth())
		if err != nil {
			return err
		}
		return c.Store.Put("book", id, it.URL, rec)
	case "author":
		rec, err := c.Client.GetAuthorRecord(ctx, id)
		if err != nil {
			return err
		}
		return c.Store.Put("author", id, it.URL, rec)
	case "series":
		rec, err := c.Client.GetSeriesRecord(ctx, id)
		if err != nil {
			return err
		}
		return c.Store.Put("series", id, it.URL, rec)
	case "list":
		rec, err := c.Client.GetListRecord(ctx, id, 1)
		if err != nil {
			return err
		}
		return c.Store.Put("list", rec.ID, it.URL, rec)
	case "genre":
		rec, err := c.Client.GetGenreRecord(ctx, id)
		if err != nil {
			return err
		}
		return c.Store.Put("genre", id, it.URL, rec)
	case "work":
		rec, err := c.Client.GetEditionsRecord(ctx, id, 1)
		if err != nil {
			return err
		}
		return c.Store.Put("editions", id, it.URL, rec)
	}
	return fmt.Errorf("nothing reads a %s", it.Kind)
}

func (c *Crawler) bookDepth() Depth {
	if c.BookDepth == "" {
		return DepthMeta
	}
	return c.BookDepth
}

// expand puts the neighbours of a node that was just read onto the frontier.
//
// Out of the edge table rather than out of the HTML, because the edges are what
// the record actually said and they are already deduplicated by URI. A node
// with no page, a place or a character, is skipped by URIToURL.
func (c *Crawler) expand(it FrontierItem) error {
	seen := map[string]bool{it.URI: true}
	for _, incoming := range []bool{false, true} {
		edges, err := c.Store.Edges(it.URI, incoming)
		if err != nil {
			return err
		}
		for _, e := range edges {
			next := e.Dst
			if incoming {
				next = e.Src
			}
			if seen[next] {
				continue
			}
			seen[next] = true
			kind, _, pageURL, ok := URIToURL(next)
			if !ok {
				continue
			}
			if _, status, err := c.Store.CrawledDepth(next); err == nil && status == "done" {
				continue
			}
			if err := c.Store.EnqueueURI(next, pageURL, kind, it.Depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

// ErrCrawlNeedsYes is the refusal when --no-robots is set without --yes.
//
// A crawl is the one place where an override multiplies. Reading one disallowed
// page on purpose is a decision somebody makes once; a crawl with the same flag
// makes that decision several thousand times without being asked again.
var ErrCrawlNeedsYes = errors.New("a crawl with --no-robots needs --yes as well")
