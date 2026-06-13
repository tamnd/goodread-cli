package goodread

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// Cache is a content-addressed, gzipped on-disk page cache. A captured page is
// stored under cache/<ab>/<sha256>.html.gz, keyed by its URL.
type Cache struct {
	dir string
	ttl time.Duration
}

// NewCache returns a cache rooted at cfg.CacheDir() (created on first write).
func NewCache(cfg Config) *Cache {
	ttl := cfg.CacheTTL
	if ttl <= 0 {
		ttl = DefaultCacheTTL
	}
	return &Cache{dir: cfg.CacheDir(), ttl: ttl}
}

func (c *Cache) pathFor(url string) string {
	sum := sha256.Sum256([]byte(url))
	h := hex.EncodeToString(sum[:])
	return filepath.Join(c.dir, h[:2], h+".html.gz")
}

// Get returns cached bytes for a URL when present and within the TTL.
func (c *Cache) Get(url string) ([]byte, bool) {
	p := c.pathFor(url)
	info, err := os.Stat(p)
	if err != nil {
		return nil, false
	}
	if c.ttl > 0 && time.Since(info.ModTime()) > c.ttl {
		return nil, false
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, false
	}
	defer zr.Close()
	body, err := io.ReadAll(zr)
	if err != nil {
		return nil, false
	}
	return body, true
}

// Put writes bytes for a URL into the cache.
func (c *Cache) Put(url string, body []byte) error {
	p := c.pathFor(url)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	tmp := p + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	zw := gzip.NewWriter(f)
	if _, err := zw.Write(body); err != nil {
		zw.Close()
		f.Close()
		return err
	}
	if err := zw.Close(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// Path returns the on-disk cache path for a URL (whether or not it exists).
func (c *Cache) Path(url string) string { return c.pathFor(url) }

// Clear removes the entire cache directory.
func (c *Cache) Clear() error { return os.RemoveAll(c.dir) }

// Stats reports the file count and total bytes held in the cache.
func (c *Cache) Stats() (files int, bytes int64, err error) {
	walkErr := filepath.Walk(c.dir, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			files++
			bytes += info.Size()
		}
		return nil
	})
	if os.IsNotExist(walkErr) {
		return 0, 0, nil
	}
	return files, bytes, walkErr
}

// CachingFetch returns page bytes, reading the cache unless NoCache/Refresh is
// set, and populating it on a miss.
func (c *Client) CachingFetch(ctx context.Context, cache *Cache, cfg Config, url string) ([]byte, error) {
	if cache != nil && !cfg.NoCache && !cfg.Refresh {
		if body, ok := cache.Get(url); ok {
			return body, nil
		}
	}
	body, code, err := c.Fetch(ctx, url)
	if err != nil {
		return nil, err
	}
	if code == 404 {
		return nil, ErrNotFound
	}
	if cache != nil && !cfg.NoCache {
		_ = cache.Put(url, body)
	}
	return body, nil
}

// DocFromBytes parses raw HTML bytes into a goquery document.
func DocFromBytes(body []byte) (*goquery.Document, error) {
	return goquery.NewDocumentFromReader(bytes.NewReader(body))
}

// CachingFetchHTML is CachingFetch plus goquery parsing.
func (c *Client) CachingFetchHTML(ctx context.Context, cache *Cache, cfg Config, url string) (*goquery.Document, error) {
	body, err := c.CachingFetch(ctx, cache, cfg, url)
	if err != nil {
		return nil, err
	}
	return goquery.NewDocumentFromReader(bytes.NewReader(body))
}
