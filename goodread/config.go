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
	DefaultWorkers  = 1
	DefaultTimeout  = 30 * time.Second
	DefaultRetries  = 3
	DefaultCacheTTL = 24 * time.Hour

	// MinDelay is the floor on request spacing and it is not overridable.
	//
	// --no-robots changes which paths are reachable and changes nothing about
	// how fast. The honest reason to ignore robots.txt is "I want this page",
	// never "I want it faster than they can serve it", so there is no flag
	// that raises this ceiling and no config key that lowers it. A value below
	// the floor is clamped and the user is told, rather than silently honoured
	// or silently ignored.
	MinDelay = 1 * time.Second

	// MaxWorkers is 1. A crawler that can be told to open ten connections will
	// be, and the site does not get a say.
	MaxWorkers = 1
)

// DefaultUserAgent names the tool, its version and where to find it.
//
// v0.2.0 rotated a pool of five real browser strings per request, so traffic
// would "look like ordinary browsing". That is the identity half of evading a
// crawler policy, and it sits badly next to a version whose whole point is
// reading and obeying that policy. It also breaks the one thing a User-Agent
// is for: a site operator who wants to rate-limit us, or ask us to stop, now
// has a name and a URL to do it with.
//
// The field is still free text and setting it to a browser string is possible,
// because pretending otherwise would be theatre. The default never does it and
// no flag does it for you.
func DefaultUserAgent() string {
	return "goodread/" + Version + " (+https://github.com/tamnd/goodread-cli)"
}

// Version is set by the CLI at startup from its own build info, so the library
// and the binary do not drift.
var Version = "dev"

// Config holds runtime configuration for the library.
//
// Note what is not here. There is no NoRobots key that can be set from a file
// or an environment variable: the field below is filled from the command-line
// flag and from nothing else, and TestNoRobotsNotInConfig holds that. An
// override whose whole justification is that a person decided it is their call
// has to actually be that person, this invocation, in the shell history. A
// config key would make it ambient and a month later nobody would remember it
// was on.
//
// There is also no CookiePath. v0.2.0 could load a Netscape cookie jar to
// borrow a signed-in session. Reading a member-only page with somebody's
// session is a different act from reading a public one, and this tool does
// only the second.
type Config struct {
	DataDir   string        // root for cache/store; defaults to XDG data dir
	Workers   int           // retained for API compatibility; clamped to MaxWorkers
	Delay     time.Duration // spacing between requests, clamped to MinDelay
	Timeout   time.Duration // per-request timeout
	Retries   int           // retry attempts on 429/5xx
	CacheTTL  time.Duration // on-disk page-cache freshness window
	NoCache   bool          // bypass the on-disk page cache entirely
	Refresh   bool          // force re-fetch and overwrite the cache
	UserAgent string        // overrides DefaultUserAgent when set

	// NoRobots is set from the --no-robots flag only. See the type comment.
	NoRobots bool
}

// DefaultConfig returns sensible, polite defaults rooted at the XDG data dir.
func DefaultConfig() Config {
	return Config{
		DataDir:   DefaultDataDir(),
		Workers:   DefaultWorkers,
		Delay:     DefaultDelay,
		Timeout:   DefaultTimeout,
		Retries:   DefaultRetries,
		CacheTTL:  DefaultCacheTTL,
		UserAgent: DefaultUserAgent(),
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
