package goodread

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ErrNoRobots means robots.txt could not be read, so nothing was attempted.
//
// This is deliberately not the same as "the rules said no". A crawler that
// carries on when it cannot check permission is worse than one that stops,
// so there is no compiled-in fallback copy and no flag that turns this into
// a proceed. A stale copy that says yes is worse than no answer at all.
var ErrNoRobots = errors.New("robots.txt could not be read")

// ErrDisallowed means the rules said no and --no-robots would have allowed it.
var ErrDisallowed = errors.New("disallowed by robots.txt")

// RobotsTTL is how long a fetched robots.txt is trusted before refetching.
const RobotsTTL = time.Hour

// Rule is one Allow or Disallow line from the group that applies to us.
type Rule struct {
	Allow   bool
	Pattern string
}

// String renders the rule the way it appeared in the file, so error messages
// can quote the site's own words rather than paraphrasing them.
func (r Rule) String() string {
	if r.Allow {
		return "Allow: " + r.Pattern
	}
	return "Disallow: " + r.Pattern
}

// Robots is the parsed User-agent group that applies to this tool.
type Robots struct {
	Rules      []Rule
	CrawlDelay time.Duration
	Sitemaps   []string
	Source     string
	FetchedAt  time.Time
}

// Expired reports whether the copy is older than RobotsTTL.
func (r *Robots) Expired(now time.Time) bool { return now.Sub(r.FetchedAt) > RobotsTTL }

// ParseRobots reads a robots.txt body and keeps the group matching agent.
//
// Group selection follows the usual convention: consecutive User-agent lines
// introduce one group, and the most specific matching token wins, with "*" as
// the fallback. Goodreads names several bots explicitly and gives them looser
// rules than "*", so picking the wrong group would quietly grant permissions
// this tool was never given.
//
// Sitemap lines are collected regardless of group, because they are a property
// of the site rather than of any one agent.
func ParseRobots(body []byte, agent string) *Robots {
	r := &Robots{Rules: []Rule{}}
	agent = strings.ToLower(agent)

	type group struct {
		agents []string
		rules  []Rule
		delay  time.Duration
	}
	var groups []*group
	var cur *group
	// namingAgents tracks whether the previous non-blank directive was also a
	// User-agent, so that stacked names join one group instead of starting new
	// ones.
	namingAgents := false

	sc := bufio.NewScanner(bytes.NewReader(body))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		val = strings.TrimSpace(val)

		switch key {
		case "user-agent":
			if !namingAgents || cur == nil {
				cur = &group{}
				groups = append(groups, cur)
			}
			cur.agents = append(cur.agents, strings.ToLower(val))
			namingAgents = true
			continue
		case "sitemap":
			if val != "" {
				r.Sitemaps = append(r.Sitemaps, val)
			}
			continue
		}
		namingAgents = false
		if cur == nil {
			continue
		}
		switch key {
		case "allow":
			// An empty Allow is meaningless and some files carry it. Skip it
			// rather than registering a zero-length pattern that matches
			// everything at the lowest specificity.
			if val != "" {
				cur.rules = append(cur.rules, Rule{Allow: true, Pattern: val})
			}
		case "disallow":
			// An empty Disallow means "nothing is disallowed" and must not
			// become a rule matching every path.
			if val != "" {
				cur.rules = append(cur.rules, Rule{Pattern: val})
			}
		case "crawl-delay":
			if f, err := strconv.ParseFloat(val, 64); err == nil && f > 0 {
				cur.delay = time.Duration(f * float64(time.Second))
			}
		}
	}

	// Most specific matching agent token wins; "*" is the fallback.
	var best *group
	bestLen := -1
	for _, g := range groups {
		for _, a := range g.agents {
			switch {
			case a == "*":
				if bestLen < 0 {
					best, bestLen = g, 0
				}
			case a != "" && strings.Contains(agent, a):
				if len(a) > bestLen {
					best, bestLen = g, len(a)
				}
			}
		}
	}
	if best != nil {
		r.Rules = best.rules
		r.CrawlDelay = best.delay
	}
	return r
}

// Check returns the rule governing a path, or nil when no rule matches.
//
// The argument is path plus query, not path alone. Goodreads publishes
// "Disallow: /*reviewFilters", which can only ever match a query string, so a
// checker that is handed the path alone silently permits every URL that rule
// was written to refuse.
//
// Resolution is longest-match-wins measured on the pattern as written, with
// Allow beating Disallow at equal length. Both halves matter here: the two
// /work exceptions in Goodreads' file are "Allow: /work/editions" against
// "Disallow: /work", and getting the comparison backwards loses the editions
// and quotes pages that the rest of this tool depends on.
func (r *Robots) Check(pathAndQuery string) *Rule {
	if r == nil {
		return nil
	}
	var best *Rule
	bestLen := -1
	for i := range r.Rules {
		rule := &r.Rules[i]
		if !matchPattern(rule.Pattern, pathAndQuery) {
			continue
		}
		n := len(rule.Pattern)
		if n > bestLen || (n == bestLen && rule.Allow && best != nil && !best.Allow) {
			best, bestLen = rule, n
		}
	}
	return best
}

// Allowed reports whether a path plus query may be fetched. No matching rule
// means allowed, which is what the standard says and what everything else
// assumes.
func (r *Robots) Allowed(pathAndQuery string) bool {
	m := r.Check(pathAndQuery)
	return m == nil || m.Allow
}

// matchPattern implements robots.txt path matching: a plain prefix match, with
// "*" standing for any run of characters and a trailing "$" anchoring the end.
//
// Goodreads' current file uses "*" and not "$", but "$" is part of the de facto
// standard every other crawler implements, and a parser that ignores it will
// over-fetch the day the file changes. Cheap to support, expensive to add
// under pressure later.
func matchPattern(pattern, path string) bool {
	anchored := strings.HasSuffix(pattern, "$")
	if anchored {
		pattern = pattern[:len(pattern)-1]
	}
	parts := strings.Split(pattern, "*")

	// No wildcard: prefix match, or full match when anchored.
	if len(parts) == 1 {
		if anchored {
			return path == pattern
		}
		return strings.HasPrefix(path, pattern)
	}

	// The first segment must sit at the start.
	if !strings.HasPrefix(path, parts[0]) {
		return false
	}
	pos := len(parts[0])

	// Middle segments match greedily left to right, which is correct here
	// because robots.txt patterns have no alternation to backtrack over.
	for _, seg := range parts[1 : len(parts)-1] {
		if seg == "" {
			continue
		}
		i := strings.Index(path[pos:], seg)
		if i < 0 {
			return false
		}
		pos += i + len(seg)
	}

	last := parts[len(parts)-1]
	if last == "" {
		// Pattern ended in "*", so anything left over is fine. An anchored
		// "*$" is the same thing.
		return true
	}
	if anchored {
		return strings.HasSuffix(path[pos:], last)
	}
	return strings.Contains(path[pos:], last)
}

// RobotsFetcher fetches and caches robots.txt for one host.
//
// It is a separate type from Client so that the client can depend on it
// without the fetch of robots.txt itself needing to be checked against
// robots.txt, which would not terminate.
type RobotsFetcher struct {
	mu     sync.Mutex
	cached *Robots
	url    string
	agent  string
	get    func(ctx context.Context, url string) ([]byte, int, error)
	now    func() time.Time
}

// NewRobotsFetcher builds a fetcher for the site's robots.txt.
func NewRobotsFetcher(agent string, get func(ctx context.Context, url string) ([]byte, int, error)) *RobotsFetcher {
	return &RobotsFetcher{url: RobotsTxtURL, agent: agent, get: get, now: time.Now}
}

// Get returns the parsed rules, fetching or refetching as needed.
//
// A 404 is treated as "no rules published", which is what the standard says
// and is a real answer. Any other failure is ErrNoRobots, and the caller stops.
func (f *RobotsFetcher) Get(ctx context.Context) (*Robots, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.cached != nil && !f.cached.Expired(f.now()) {
		return f.cached, nil
	}

	body, code, err := f.get(ctx, f.url)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNoRobots, err)
	}
	switch {
	case code == 404:
		f.cached = &Robots{Source: f.url, FetchedAt: f.now()}
		return f.cached, nil
	case code != 200:
		return nil, fmt.Errorf("%w: HTTP %d from %s", ErrNoRobots, code, f.url)
	}

	r := ParseRobots(body, f.agent)
	r.Source = f.url
	r.FetchedAt = f.now()
	f.cached = r
	return r, nil
}

// Set installs a parsed copy, for tests and for offline use.
func (f *RobotsFetcher) Set(r *Robots) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cached = r
}
