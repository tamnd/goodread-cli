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
	json     bool
	fields   []string
	noHeader bool
	template string
	color    string
	limit    int
	quiet    bool
	verbose  int
	depth    goodread.Depth
	depthArg string
}

// Exit codes, as 05_commands.md section 8 lists them.
//
// These moved in v0.3.0. v0.2.0 had 3 for no data, 4 for partial and 5 for
// blocked, and the numbering below says what went wrong rather than how the
// run ended, which is what a script wants when it is deciding whether to
// retry. The change is breaking and it is documented in one place rather than
// dribbled out one command at a time.
//
// 7 and 8 are separate on purpose. 7 is a decision the user can reverse by
// passing --no-robots. 8 is the tool refusing to guess because it could not
// read the rules at all, and no flag turns that into a proceed.
//
// 1 is the unclassified failure, the one nothing below recognised. It stays a
// distinct code rather than being folded into any of the specific ones,
// because a run that exits 4 is telling a script something true about the site
// and a run that exits 1 is telling it we do not know.
const (
	exitError      = 1
	exitUsage      = 2 // usage, and a config file that will not load
	exitNetwork    = 3
	exitHTTP       = 4 // the site answered, and the answer was an error or a block
	exitParse      = 5 // extraction failed, or a record did not reconcile
	exitNotFound   = 6
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
	pf.BoolVar(&app.json, "json", false, "shorthand for --format json")
	pf.StringSliceVar(&app.fields, "fields", nil, "comma-separated columns to include")
	pf.BoolVar(&app.noHeader, "no-header", false, "omit the header row in table/csv/tsv")
	pf.StringVar(&app.template, "template", "", "Go text/template applied per record")
	pf.StringVar(&app.color, "color", "auto", "color: auto|always|never")
	pf.IntVarP(&app.limit, "limit", "n", 0, "limit number of results (0 = no limit)")
	pf.BoolVarP(&app.quiet, "quiet", "q", false, "suppress progress on stderr")
	pf.CountVarP(&app.verbose, "verbose", "v", "explain what is being read. -vv adds every request and the extraction ladder")
	pf.StringVar(&app.depthArg, "depth", string(goodread.DepthMeta), "how much to read: "+depthList())

	// There is no --workers, and there was one in v0.2.0. It was clamped to
	// MaxWorkers, which is 1, so it did nothing except suggest the tool could
	// be told to open ten connections. A flag that does nothing but imply that
	// is worse than no flag, and 05_commands.md section 6 asks for no flag.
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
		app.workCmd(),
		app.authorCmd(),
		app.seriesCmd(),
		app.listCmd(),
		app.quotesCmd(),
		app.editionsCmd(),
		app.userCmd(),
		app.shelfCmd(),
		app.genreCmd(),
		app.searchCmd(),
		app.lookupCmd(),
		app.findCmd(),
		app.queryCmd(),
		app.graphCmd(),
		app.exportCmd(),
		app.reviewsCmd(),
		app.similarCmd(),
		app.idCmd(),
		app.seedCmd(),
		app.crawlCmd(),
		app.mcpCmd(),
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
	// The file first, so everything below it works on resolved values. It only
	// fills in what the command line left alone, so nothing here can undo a
	// flag the user typed.
	fc, err := loadConfigFile(configPath())
	if err != nil {
		return codeError(exitUsage, err)
	}
	a.applyConfigFile(cmd, fc)

	if a.json {
		a.format = string(FormatJSON)
	}
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
	// Nothing sets this from the command line any more, and the clamp stays
	// because Config is importable and a caller of the library can still put a
	// number in it.
	if a.cfg.Workers > goodread.MaxWorkers {
		a.cfg.Workers = goodread.MaxWorkers
	}

	d, ok := goodread.ParseDepth(a.depthArg)
	if !ok {
		return codeError(exitUsage, fmt.Errorf("unknown depth %q, want one of %s", a.depthArg, depthList()))
	}
	a.depth = d

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
		return codeError(exitNotFound, nil)
	}
	return nil
}

// verbosef prints an explanation at or above a verbosity level.
//
// -v says what is being read and what was not. -vv adds every request and the
// extraction ladder. Both go to stderr, so piping the output stays clean.
func (a *App) verbosef(level int, format string, args ...any) {
	if a.verbose < level || a.quiet {
		return
	}
	fmt.Fprintf(os.Stderr, format+"\n", args...)
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
		return codeError(exitHTTP, err)
	case isNotFound(err):
		return codeError(exitNotFound, err)
	case isNetwork(err):
		return codeError(exitNetwork, err)
	default:
		return codeError(exitError, err)
	}
}
