package goodread

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/time/rate"
)

// ErrBlocked signals that Goodreads served a sign-in wall (or otherwise refused
// the page). Callers map it to the "blocked" exit code and print a hint.
var ErrBlocked = errors.New("blocked: page requires sign-in")

// ErrRateLimited signals a sustained HTTP 429 after retries.
var ErrRateLimited = errors.New("rate limited (HTTP 429)")

// Client performs polite, retrying HTTP GETs against goodreads.com.
type Client struct {
	http      *http.Client
	userAgent string
	limiter   *rate.Limiter
	retries   int

	robots    *RobotsFetcher
	noRobots  bool
	warnOnce  sync.Once
	warnTo    io.Writer
	crawlWait time.Duration

	// base is the site root. It exists so tests can exercise Do end to end
	// against a local server rather than against goodreads.com, which is the
	// only way to assert what the override does without actually doing it to
	// somebody's site.
	base string
}

// SetBaseURL points the client at a different origin, for tests.
func (c *Client) SetBaseURL(u string) { c.base = strings.TrimSuffix(u, "/") }

// ClampDelay enforces MinDelay and reports whether it had to.
//
// Clamping happens here, before the value can reach a limiter, and there is
// exactly one path that builds a limiter from a configured delay. A caller
// that wants a faster pace gets told it was clamped rather than having the
// value quietly honoured or quietly dropped.
func ClampDelay(d time.Duration) (time.Duration, bool) {
	if d <= 0 {
		return DefaultDelay, false
	}
	if d < MinDelay {
		return MinDelay, true
	}
	return d, false
}

// newLimiter builds the token-bucket limiter from a clamped delay.
//
// Burst is 1. A burst of N lets N requests leave at once, which is the same
// thing as concurrency wearing a different name, and the point of the floor is
// that no flag and no config makes this tool hit the site harder.
func newLimiter(cfg Config) *rate.Limiter {
	d, _ := ClampDelay(cfg.Delay)
	return rate.NewLimiter(rate.Every(d), 1)
}

// transport pins the connection pool to MaxWorkers. It takes no Config on
// purpose: there is no setting that widens it.
func transport() *http.Transport {
	return &http.Transport{
		MaxIdleConns:        MaxWorkers,
		MaxConnsPerHost:     MaxWorkers,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
}

// NewClient builds an anonymous client that obeys robots.txt.
func NewClient(cfg Config) *Client {
	retries := cfg.Retries
	if retries <= 0 {
		retries = DefaultRetries
	}
	ua := cfg.UserAgent
	if ua == "" {
		ua = DefaultUserAgent()
	}
	c := &Client{
		http:      &http.Client{Timeout: cfg.Timeout, Transport: transport()},
		userAgent: ua,
		limiter:   newLimiter(cfg),
		retries:   retries,
		noRobots:  cfg.NoRobots,
		warnTo:    os.Stderr,
		base:      BaseURL,
	}
	// The rules are fetched through doGet rather than Fetch, because gating
	// the fetch of robots.txt on robots.txt does not terminate.
	c.robots = NewRobotsFetcher(ua, func(ctx context.Context, _ string) ([]byte, int, error) {
		if err := c.limiter.Wait(ctx); err != nil {
			return nil, 0, err
		}
		return c.doGet(ctx, c.base+"/robots.txt")
	})
	return c
}

// Robots exposes the fetcher so callers can render `goodread robots` and so
// tests can install a captured copy without going near the network.
func (c *Client) Robots() *RobotsFetcher { return c.robots }

// SetWarnWriter redirects the --no-robots warning, for tests.
func (c *Client) SetWarnWriter(w io.Writer) { c.warnTo = w }

// Do performs a registered op.
//
// The op lookup is what lets the refusal name an allowed alternative, which a
// bare URL cannot do. The permission check itself lives in Fetch, so a caller
// that has not been ported to the registry yet is still covered.
func (c *Client) Do(ctx context.Context, opName string, args ...string) ([]byte, int, error) {
	op, ok := LookupOp(opName)
	if !ok {
		return nil, 0, fmt.Errorf("no such op %q", opName)
	}
	return c.Fetch(ctx, c.base+op.Path(args...))
}

// check enforces robots.txt for one URL, and is the only place that does.
//
// It sits in Fetch rather than in Do on purpose. Putting it in Do would only
// cover callers that had been ported to the registry, and the whole defect
// being fixed here is that v0.2.0 had call sites building their own URLs. A
// check that can be bypassed by not calling the right function is not a check.
// Off-site URLs pass through untouched, since Goodreads' rules have nothing to
// say about another host.
func (c *Client) check(ctx context.Context, rawurl string) error {
	target, ok := c.samePath(rawurl)
	if !ok {
		return nil
	}
	// Fetching the rules cannot be gated on the rules.
	if strings.HasPrefix(target, "/robots.txt") {
		return nil
	}

	r, err := c.robots.Get(ctx)
	if err != nil {
		return err
	}

	if rule := r.Check(target); rule != nil && !rule.Allow {
		if !c.noRobots {
			e := &DisallowedError{Path: target, Rule: *rule, Source: r.Source}
			if op, ok := opForPath(target); ok {
				e.Op, e.Alt = op.Name, op.Alt
			}
			return e
		}
		c.warn(target, *rule)
	}

	// A published Crawl-delay is the site stating its capacity, so it is
	// honoured whether or not the override is on. Ignoring a stated rate is
	// the part that actually causes harm, which is a different thing from
	// reading a page the file asked us not to.
	if r.CrawlDelay > c.crawlWait {
		c.crawlWait = r.CrawlDelay
		if d, _ := ClampDelay(c.crawlWait); d > 0 {
			c.limiter.SetLimit(rate.Every(d))
		}
	}
	return nil
}

// samePath returns the path and query when a URL points at the configured
// site, and reports false otherwise.
func (c *Client) samePath(rawurl string) (string, bool) {
	base, err := url.Parse(c.base)
	if err != nil {
		return "", false
	}
	u, err := url.Parse(rawurl)
	if err != nil {
		return "", false
	}
	if u.Host != "" && !strings.EqualFold(u.Host, base.Host) {
		return "", false
	}
	if u.RawQuery != "" {
		return u.Path + "?" + u.RawQuery, true
	}
	return u.Path, true
}

// opForPath finds the registered op a path belongs to, so a refusal raised
// deep in an unported call site can still name an alternative.
func opForPath(path string) (Op, bool) {
	path = pathOnly(path)
	var best Op
	bestLen := -1
	for _, o := range Ops {
		p := pathOnly(SamplePath(o))
		// Compare on the fixed prefix, since the sample carries a stand-in id.
		prefix := p
		if i := strings.LastIndexByte(strings.TrimSuffix(p, "/"), '/'); i > 0 {
			prefix = p[:i+1]
		}
		if strings.HasPrefix(path, prefix) && len(prefix) > bestLen {
			best, bestLen = o, len(prefix)
		}
	}
	return best, bestLen >= 0
}

// warn prints the override notice once per process, on stderr, so a pipe is
// unaffected. Once, because a crawl would otherwise print it thousands of
// times and a warning nobody reads is not a warning.
func (c *Client) warn(path string, rule Rule) {
	c.warnOnce.Do(func() {
		if c.warnTo == nil {
			return
		}
		fmt.Fprintf(c.warnTo, "warning: --no-robots. %s is disallowed by goodreads robots.txt (%s).\n",
			pathOnly(path), rule.String())
	})
}

// Fetch returns the raw body and status for a URL, retrying transient failures.
// A 404 returns (nil, 404, nil). A sign-in redirect returns ErrBlocked.
func (c *Client) Fetch(ctx context.Context, rawurl string) ([]byte, int, error) {
	if err := c.check(ctx, rawurl); err != nil {
		return nil, 0, err
	}
	max := c.retries
	if max < 1 {
		max = 1
	}
	for attempt := 1; attempt <= max; attempt++ {
		if err := c.limiter.Wait(ctx); err != nil {
			return nil, 0, err
		}
		body, code, err := c.doGet(ctx, rawurl)
		if err != nil {
			if errors.Is(err, ErrBlocked) || attempt == max {
				return nil, code, err
			}
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}
		switch {
		case code == 429:
			if attempt == max {
				return nil, code, ErrRateLimited
			}
			time.Sleep(time.Duration(attempt*attempt) * 10 * time.Second)
			continue
		case code == 404:
			return nil, code, nil
		case code >= 500:
			if attempt == max {
				return nil, code, fmt.Errorf("server error HTTP %d", code)
			}
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
			continue
		}
		return body, code, nil
	}
	return nil, 0, fmt.Errorf("all %d attempts failed", max)
}

// FetchHTML fetches a URL and parses it into a goquery document.
func (c *Client) FetchHTML(ctx context.Context, rawurl string) (*goquery.Document, int, error) {
	body, code, err := c.Fetch(ctx, rawurl)
	if err != nil {
		return nil, code, err
	}
	if code == 404 {
		return nil, code, nil
	}
	if code != 200 {
		return nil, code, fmt.Errorf("unexpected HTTP %d for %s", code, rawurl)
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil, code, fmt.Errorf("parse HTML: %w", err)
	}
	return doc, code, nil
}

func (c *Client) doGet(ctx context.Context, rawurl string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawurl, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	// Leave Accept-Encoding to the transport for transparent gzip.

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	// AWS WAF serves a JS challenge as 202/405 with x-amzn-waf-action: challenge
	// and an empty body. No headless solver here, so treat it as blocked.
	if action := resp.Header.Get("x-amzn-waf-action"); action == "challenge" || action == "captcha" {
		return nil, resp.StatusCode, fmt.Errorf("%w (AWS WAF %s for %s)", ErrBlocked, action, rawurl)
	}

	// The client follows redirects; landing on a sign-in path means blocked.
	final := resp.Request.URL.String()
	if isSignIn(final) {
		return nil, 401, fmt.Errorf("%w (%s redirected to %s)", ErrBlocked, rawurl, final)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}

	// Follow the "You are being redirected" HTML soft-redirect once.
	if resp.StatusCode == 200 && strings.Contains(string(body), "You are being") {
		if to := extractHTMLRedirect(string(body)); to != "" && to != rawurl {
			if isSignIn(to) {
				return nil, 401, fmt.Errorf("%w (%s)", ErrBlocked, rawurl)
			}
			time.Sleep(300 * time.Millisecond)
			return c.doGet(ctx, to)
		}
	}
	return body, resp.StatusCode, nil
}

func isSignIn(u string) bool {
	return strings.Contains(u, "/sign_in") || strings.Contains(u, "/user/login") || strings.Contains(u, "/user/sign_in")
}

func extractHTMLRedirect(body string) string {
	start := strings.Index(body, `href="`)
	if start < 0 {
		return ""
	}
	start += len(`href="`)
	end := strings.Index(body[start:], `"`)
	if end < 0 {
		return ""
	}
	return body[start : start+end]
}
