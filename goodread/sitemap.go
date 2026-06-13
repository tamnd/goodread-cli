package goodread

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// Goodreads does not serve /sitemap.xml. The real entry points are listed in
// robots.txt as siteindex.<category>.xml files, each a <sitemapindex> pointing
// at gzipped <urlset> shards (sitemap.NNN.xml.gz).

type sitemapIndex struct {
	Sitemaps []struct {
		Loc string `xml:"loc"`
	} `xml:"sitemap"`
}

type urlSet struct {
	URLs []struct {
		Loc string `xml:"loc"`
	} `xml:"url"`
}

// SitemapCategory pairs a siteindex category (author, list, quote, genre, ...)
// with its siteindex URL.
type SitemapCategory struct {
	Category string `json:"category"`
	URL      string `json:"url"`
}

// RobotsSitemaps fetches robots.txt and returns the advertised siteindex URLs.
func (c *Client) RobotsSitemaps(ctx context.Context) ([]SitemapCategory, error) {
	body, code, err := c.Fetch(ctx, RobotsTxtURL)
	if err != nil {
		return nil, err
	}
	if code != 200 {
		return nil, fmt.Errorf("robots.txt HTTP %d", code)
	}
	var out []SitemapCategory
	sc := bufio.NewScanner(bytes.NewReader(body))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(strings.ToLower(line), "sitemap:") {
			continue
		}
		u := strings.TrimSpace(line[len("sitemap:"):])
		out = append(out, SitemapCategory{Category: sitemapCategory(u), URL: u})
	}
	return out, sc.Err()
}

// sitemapCategory extracts "author" from ".../siteindex.author.xml".
func sitemapCategory(u string) string {
	base := u
	if i := strings.LastIndex(u, "/"); i >= 0 {
		base = u[i+1:]
	}
	base = strings.TrimPrefix(base, "siteindex.")
	base = strings.TrimSuffix(base, ".xml")
	return base
}

// SitemapIndex fetches a <sitemapindex> document and returns its child URLs.
func (c *Client) SitemapIndex(ctx context.Context, indexURL string) ([]string, error) {
	body, err := c.fetchMaybeGz(ctx, indexURL)
	if err != nil {
		return nil, err
	}
	var idx sitemapIndex
	if err := xml.Unmarshal(body, &idx); err != nil {
		return nil, fmt.Errorf("parse sitemap index: %w", err)
	}
	out := make([]string, 0, len(idx.Sitemaps))
	for _, s := range idx.Sitemaps {
		if s.Loc != "" {
			out = append(out, s.Loc)
		}
	}
	return out, nil
}

// SitemapURLs fetches one <urlset> shard (gz-aware) and returns its URLs.
func (c *Client) SitemapURLs(ctx context.Context, sitemapURL string) ([]string, error) {
	body, err := c.fetchMaybeGz(ctx, sitemapURL)
	if err != nil {
		return nil, err
	}
	var set urlSet
	if err := xml.Unmarshal(body, &set); err != nil {
		return nil, fmt.Errorf("parse sitemap: %w", err)
	}
	out := make([]string, 0, len(set.URLs))
	for _, u := range set.URLs {
		if u.Loc != "" {
			out = append(out, u.Loc)
		}
	}
	return out, nil
}

func (c *Client) fetchMaybeGz(ctx context.Context, u string) ([]byte, error) {
	body, code, err := c.Fetch(ctx, u)
	if err != nil {
		return nil, err
	}
	if code != 200 {
		return nil, fmt.Errorf("sitemap HTTP %d for %s", code, u)
	}
	// Decompress when gzipped by extension or by magic bytes (CloudFront may
	// already have decoded the transfer encoding).
	if strings.HasSuffix(u, ".gz") && len(body) >= 2 && body[0] == 0x1f && body[1] == 0x8b {
		zr, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("gunzip sitemap: %w", err)
		}
		defer func() { _ = zr.Close() }()
		if body, err = io.ReadAll(zr); err != nil {
			return nil, err
		}
	}
	return body, nil
}
