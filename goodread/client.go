package goodread

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
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
	http       *http.Client
	userAgents []string
	limiter    *rate.Limiter
	retries    int
}

// newLimiter builds a token-bucket limiter: rate = workers/delay, burst = workers,
// so every worker can fire at startup but the average pace stays polite.
func newLimiter(cfg Config) *rate.Limiter {
	if cfg.Delay <= 0 || cfg.Workers <= 0 {
		return rate.NewLimiter(rate.Inf, 1)
	}
	r := rate.Limit(float64(cfg.Workers) / cfg.Delay.Seconds())
	return rate.NewLimiter(r, cfg.Workers)
}

func transport(cfg Config) *http.Transport {
	return &http.Transport{
		MaxIdleConns:        cfg.Workers + 4,
		MaxConnsPerHost:     cfg.Workers + 4,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
}

// NewClient builds an anonymous client.
func NewClient(cfg Config) *Client {
	retries := cfg.Retries
	if retries <= 0 {
		retries = DefaultRetries
	}
	return &Client{
		http:       &http.Client{Timeout: cfg.Timeout, Transport: transport(cfg)},
		userAgents: userAgents,
		limiter:    newLimiter(cfg),
		retries:    retries,
	}
}

// NewClientWithCookies builds a client pre-loaded with a lent session.
func NewClientWithCookies(cfg Config, cookies []*http.Cookie) (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	u, _ := url.Parse(BaseURL)
	jar.SetCookies(u, cookies)
	c := NewClient(cfg)
	c.http.Jar = jar
	return c, nil
}

// Fetch returns the raw body and status for a URL, retrying transient failures.
// A 404 returns (nil, 404, nil). A sign-in redirect returns ErrBlocked.
func (c *Client) Fetch(ctx context.Context, rawurl string) ([]byte, int, error) {
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
	req.Header.Set("User-Agent", c.userAgents[rand.Intn(len(c.userAgents))])
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
