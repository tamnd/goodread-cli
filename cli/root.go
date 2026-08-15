package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	"github.com/tamnd/goodread-cli/goodread"
)

// Build metadata, injected via -ldflags by the Makefile/goreleaser.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// App holds shared state threaded through every command.
type App struct {
	cfg      goodread.Config
	client   *goodread.Client
	cache    *goodread.Cache
	store    *goodread.Store
	storePtr string

	// global flags
	format   string
	fields   []string
	noHeader bool
	template string
	color    string
	limit    int
	quiet    bool
}

// exit codes.
//
// 7 and 8 are separate on purpose. 7 is a decision the user can reverse by
// passing --no-robots. 8 is the tool refusing to guess because it could not
// read the rules at all, and no flag turns that into a proceed.
//
// The other five keep their v0.2.0 meanings for now. Renumbering them to match
// the spec's table is part of M4, where the rest of the command surface moves
// and one breaking change can be documented in one place.
const (
	exitError      = 1
	exitUsage      = 2
	exitNoData     = 3
	exitPartial    = 4
	exitBlocked    = 5
	exitDisallowed = 7
	exitNoRobots   = 8
)

// ExitError carries a process exit code up to main.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return fmt.Sprintf("exit %d", e.Code)
}

func (e *ExitError) Unwrap() error { return e.Err }

func codeError(code int, err error) error { return &ExitError{Code: code, Err: err} }

// NewRootCmd builds the full command tree.
func NewRootCmd() *cobra.Command {
	app := &App{cfg: goodread.DefaultConfig()}

	root := &cobra.Command{
		Use:   "goodread",
		Short: "Crawl and structure public Goodreads data",
		Long: "goodread reads public Goodreads pages (books, authors, series, lists,\n" +
			"quotes, users, shelves, genres, reviews, search) and returns rich,\n" +
			"structured records as table, JSON, JSONL, CSV, TSV, or URLs.\n\n" +
			"goodread is an independent tool and is not affiliated with, endorsed by,\n" +
			"or sponsored by Goodreads or Amazon.",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			return app.setup(cmd)
		},
		PersistentPostRun: func(_ *cobra.Command, _ []string) {
			if app.store != nil {
				_ = app.store.Close()
			}
		},
	}

	pf := root.PersistentFlags()
	pf.StringVarP(&app.format, "format", "f", "", "output: table|json|jsonl|csv|tsv|url|raw (default: table on a TTY, jsonl when piped)")
	pf.StringSliceVar(&app.fields, "fields", nil, "comma-separated columns to include")
	pf.BoolVar(&app.noHeader, "no-header", false, "omit the header row in table/csv/tsv")
	pf.StringVar(&app.template, "template", "", "Go text/template applied per record")
	pf.StringVar(&app.color, "color", "auto", "color: auto|always|never")
	pf.IntVarP(&app.limit, "limit", "n", 0, "limit number of results (0 = no limit)")
	pf.BoolVarP(&app.quiet, "quiet", "q", false, "suppress progress on stderr")

	pf.IntVar(&app.cfg.Workers, "workers", goodread.DefaultWorkers, "concurrent workers for bulk/crawl")
	pf.DurationVar(&app.cfg.Delay, "delay", goodread.DefaultDelay, "minimum spacing between requests")
	pf.DurationVar(&app.cfg.Timeout, "timeout", goodread.DefaultTimeout, "per-request timeout")
	pf.IntVar(&app.cfg.Retries, "retries", goodread.DefaultRetries, "retry attempts on 429/5xx")
	pf.DurationVar(&app.cfg.CacheTTL, "cache-ttl", goodread.DefaultCacheTTL, "on-disk cache freshness window")
	pf.BoolVar(&app.cfg.NoCache, "no-cache", false, "bypass the on-disk page cache")
	pf.BoolVar(&app.cfg.Refresh, "refresh", false, "force re-fetch and overwrite the cache")
	pf.StringVar(&app.cfg.DataDir, "data-dir", app.cfg.DataDir, "root directory for cache and store")
	pf.StringVar(&app.storePtr, "store", "", "SQLite store path (default: <data-dir>/goodread.db)")
	pf.StringVar(&app.cfg.UserAgent, "user-agent", goodread.DefaultUserAgent(), "User-Agent header")

	// --no-robots is bound straight to the config field and to nothing else.
	// It is deliberately absent from the config file and from the environment,
	// and TestNoRobotsNotInConfig and TestNoRobotsNotInEnv hold that.
	pf.BoolVar(&app.cfg.NoRobots, "no-robots", false,
		"read paths that robots.txt disallows. warns once, and the pace floor still applies")

	root.AddCommand(
		app.bookCmd(),
		app.authorCmd(),
		app.seriesCmd(),
		app.listCmd(),
		app.quoteCmd(),
		app.userCmd(),
		app.shelfCmd(),
		app.genreCmd(),
		app.searchCmd(),
		app.reviewsCmd(),
		app.similarCmd(),
		app.idCmd(),
		app.seedCmd(),
		app.crawlCmd(),
		app.dbCmd(),
		app.openCmd(),
		app.cacheCmd(),
		app.robotsCmd(),
		app.extractionCmd(),
		app.verifyCmd(),
		app.infoCmd(),
		app.versionCmd(),
	)
	return root
}

// setup resolves output defaults and constructs the shared client/cache.
func (a *App) setup(cmd *cobra.Command) error {
	if a.format == "" {
		if isatty.IsTerminal(os.Stdout.Fd()) {
			a.format = string(FormatTable)
		} else {
			a.format = string(FormatJSONL)
		}
	}
	// Clamp before the client sees it, and say so. A pace below the floor is
	// neither honoured nor silently dropped.
	if d, clamped := goodread.ClampDelay(a.cfg.Delay); clamped {
		fmt.Fprintf(os.Stderr, "note: --delay %s is below the %s floor, using %s\n",
			a.cfg.Delay, goodread.MinDelay, d)
		a.cfg.Delay = d
	}
	if a.cfg.Workers > goodread.MaxWorkers {
		fmt.Fprintf(os.Stderr, "note: --workers %d exceeds the maximum of %d, using %d\n",
			a.cfg.Workers, goodread.MaxWorkers, goodread.MaxWorkers)
		a.cfg.Workers = goodread.MaxWorkers
	}

	goodread.Version = Version
	a.client = goodread.NewClient(a.cfg)
	a.cache = goodread.NewCache(a.cfg)
	return nil
}

// openStore lazily opens the SQLite store.
func (a *App) openStore() (*goodread.Store, error) {
	if a.store != nil {
		return a.store, nil
	}
	path := a.storePtr
	if path == "" {
		path = a.cfg.StorePath()
	}
	st, err := goodread.OpenStore(path)
	if err != nil {
		return nil, err
	}
	a.store = st
	return st, nil
}

// render writes records using the resolved global flags.
func (a *App) render(records any) error {
	r := NewRenderer(os.Stdout, Format(a.format), a.fields, a.noHeader, a.template)
	return r.Render(records)
}

// renderOrEmpty renders records, mapping an empty result to exit code 3.
func (a *App) renderOrEmpty(records any, n int) error {
	if err := a.render(records); err != nil {
		return err
	}
	if n == 0 {
		return codeError(exitNoData, nil)
	}
	return nil
}

// progressf prints a progress line to stderr unless --quiet.
func (a *App) progressf(format string, args ...any) {
	if a.quiet {
		return
	}
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}

// mapFetchErr converts a library error into the right exit code.
func mapFetchErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, goodread.ErrDisallowed):
		return codeError(exitDisallowed, err)
	case errors.Is(err, goodread.ErrNoRobots):
		return codeError(exitNoRobots, fmt.Errorf(
			"%w\nnothing was attempted. there is no fallback copy of the rules, "+
				"because a stale copy that says yes is worse than no answer", err))
	case isBlocked(err):
		return codeError(exitBlocked, err)
	case isNotFound(err):
		return codeError(exitNoData, err)
	default:
		return codeError(exitError, err)
	}
}
