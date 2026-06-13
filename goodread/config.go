package goodread

import (
	"os"
	"path/filepath"
	"time"
)

// Site constants.
const (
	BaseURL      = "https://www.goodreads.com"
	RobotsTxtURL = "https://www.goodreads.com/robots.txt"

	DefaultDelay    = 2 * time.Second
	DefaultWorkers  = 2
	DefaultTimeout  = 30 * time.Second
	DefaultRetries  = 3
	DefaultCacheTTL = 24 * time.Hour
)

// userAgents is a small pool of real browser User-Agent strings, rotated per
// request so traffic looks like ordinary browsing rather than a single client.
var userAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:132.0) Gecko/20100101 Firefox/132.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 14_6_1) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15",
}

// Config holds runtime configuration for the library.
type Config struct {
	DataDir    string        // root for cache/store; defaults to XDG data dir
	Workers    int           // concurrency for multi-page/bulk work
	Delay      time.Duration // minimum spacing between requests
	Timeout    time.Duration // per-request timeout
	Retries    int           // retry attempts on 429/5xx
	CacheTTL   time.Duration // on-disk page-cache freshness window
	NoCache    bool          // bypass the on-disk page cache entirely
	Refresh    bool          // force re-fetch and overwrite the cache
	CookiePath string        // optional Netscape cookie jar for lent sessions
}

// DefaultConfig returns sensible, polite defaults rooted at the XDG data dir.
func DefaultConfig() Config {
	return Config{
		DataDir:  DefaultDataDir(),
		Workers:  DefaultWorkers,
		Delay:    DefaultDelay,
		Timeout:  DefaultTimeout,
		Retries:  DefaultRetries,
		CacheTTL: DefaultCacheTTL,
	}
}

// DefaultDataDir resolves $XDG_DATA_HOME/goodread (or ~/.local/share/goodread).
func DefaultDataDir() string {
	if d := os.Getenv("GOODREAD_DATA_DIR"); d != "" {
		return d
	}
	if x := os.Getenv("XDG_DATA_HOME"); x != "" {
		return filepath.Join(x, "goodread")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(os.TempDir(), "goodread")
	}
	return filepath.Join(home, ".local", "share", "goodread")
}

// CacheDir is where gzipped page captures live.
func (c Config) CacheDir() string { return filepath.Join(c.DataDir, "cache") }

// StorePath is the default path of the optional SQLite store.
func (c Config) StorePath() string { return filepath.Join(c.DataDir, "goodread.db") }
