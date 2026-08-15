package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"

	"github.com/tamnd/goodread-cli/goodread"
)

// crawl is the command that makes a store.
//
// One connection, the 2 second pace, no parallelism flag. The flag is not
// offered rather than defaulted to one, because a crawler that can be told to
// open ten connections will be, and the site does not get a say.
func (a *App) crawlCmd() *cobra.Command {
	var (
		seeds       []string
		seedFile    string
		fromSitemap string
		depth       int
		max         int
		dryRun      bool
		yes         bool
		reset       bool
	)
	cmd := &cobra.Command{
		Use:   "crawl",
		Short: "Walk out from a set of seeds and build a store",
		Long: "crawl reads a seed, stores it, follows the edges the record named and\n" +
			"repeats until --depth is spent.\n\n" +
			"It is one connection at the usual pace and there is no flag to make it\n" +
			"more, because a crawler that can be told to open ten connections will be.\n\n" +
			"The frontier lives in the store, so an interrupted crawl continues where\n" +
			"it stopped when you run the same command again, and pages already read\n" +
			"come back out of the cache rather than off the site.\n\n" +
			"--dry-run prints the plan and reads nothing, which is how you find out a\n" +
			"crawl is twelve hours before starting it.",
		Args: cobra.NoArgs,
		Example: "  goodread crawl --seed gr:author/153394 --depth 2\n" +
			"  goodread crawl --seed https://www.goodreads.com/book/show/2767052 --dry-run\n" +
			"  goodread crawl --seed-file seeds.txt --depth 1 --max 200\n" +
			"  goodread crawl --from-sitemap author --max 50",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			st, err := a.openStore()
			if err != nil {
				return codeError(exitError, err)
			}

			// The refusal comes first, before anything is read and before the
			// frontier is touched. A crawl is the one place where --no-robots
			// multiplies: reading one disallowed page is a decision somebody
			// makes once, and a crawl makes it several thousand times without
			// asking again.
			if a.cfg.NoRobots && !yes {
				fmt.Fprintln(os.Stderr,
					"--no-robots on a crawl applies to every request it makes, not to one page.")
				return codeError(exitUsage, goodread.ErrCrawlNeedsYes)
			}

			if reset {
				// The frontier and not the records. Forgetting where a crawl
				// got to is cheap, and forgetting what it read is hours of
				// somebody else's bandwidth.
				if err := st.ResetFrontier(); err != nil {
					return codeError(exitError, err)
				}
				a.progressf("frontier cleared, the records are untouched")
			}

			c := &goodread.Crawler{
				Client: a.client, Store: st, Cache: a.cache, Config: a.cfg,
				Depth: depth, Max: max, BookDepth: goodread.Depth(a.depthArg),
			}

			all := append([]string(nil), seeds...)
			if seedFile != "" {
				lines, err := readSeedFile(seedFile)
				if err != nil {
					return codeError(exitUsage, err)
				}
				all = append(all, lines...)
			}
			if fromSitemap != "" {
				urls, err := a.sitemapSeeds(cmd, fromSitemap, max)
				if err != nil {
					return err
				}
				all = append(all, urls...)
			}

			added, skipped, err := c.Seed(all)
			if err != nil {
				return codeError(exitError, err)
			}
			for _, s := range skipped {
				fmt.Fprintf(os.Stderr, "skipped %q: it is not a gr: uri or a goodreads url with an id in it\n", s)
			}
			if added > 0 {
				a.progressf("seeded %d", added)
			}

			plan, err := c.Plan()
			if err != nil {
				return codeError(exitError, err)
			}
			plan.Skipped = skipped
			if plan.Pending == 0 {
				fmt.Fprintln(os.Stderr,
					"nothing to crawl. pass --seed, or --seed-file, or --from-sitemap, or run the same command again after seeding.")
				return codeError(exitUsage, nil)
			}

			if dryRun {
				a.printPlan(plan)
				return nil
			}

			// Ctrl-C stops the loop between pages rather than mid write. The
			// frontier is in the store either way, so the difference is only
			// whether the page in flight is wasted.
			runCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
			defer stop()

			a.progressf("pace %s per request, depth %d, %d to read",
				plan.Pace.Round(100*time.Millisecond), depth, plan.Pending)

			last := time.Now()
			c.Progress = func(s goodread.CrawlStats) {
				// Once a second at most. A progress line per page at two
				// seconds a page is fine, and a progress line per page out of
				// a warm cache is thousands of lines of scroll.
				if time.Since(last) < time.Second {
					return
				}
				last = time.Now()
				a.progressf("%s", s)
			}

			stats, err := c.Run(runCtx)
			if err != nil {
				return codeError(exitError, err)
			}
			a.printCrawlSummary(st, stats)

			if stats.Errors > 0 && stats.Requests == stats.Errors {
				// Everything failed. That is a run worth a non zero exit,
				// unlike a crawl of four hundred pages that lost three.
				return codeError(exitError, fmt.Errorf("every page failed, %d of them", stats.Errors))
			}
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&seeds, "seed", nil, "a gr: uri, a goodreads url or an id to start from, repeatable")
	cmd.Flags().StringVar(&seedFile, "seed-file", "", "a file of seeds, one per line, # for a comment")
	cmd.Flags().StringVar(&fromSitemap, "from-sitemap", "", "seed from a sitemap category: author, list, quote, genre, user")
	cmd.Flags().IntVar(&depth, "depth", 1, "how many hops out from each seed")
	cmd.Flags().IntVar(&max, "max", 0, "stop after this many pages (0 = until the frontier is empty)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the plan and the request count, and read nothing")
	cmd.Flags().BoolVar(&yes, "yes", false, "go ahead with a crawl that has --no-robots set")
	cmd.Flags().BoolVar(&reset, "reset", false, "clear the frontier and start over, keeping the records")
	return cmd
}

// readSeedFile reads seeds one per line, ignoring blanks and # comments.
func readSeedFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("--seed-file: %w", err)
	}
	defer func() { _ = f.Close() }()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out, sc.Err()
}

// sitemapSeeds pulls page URLs out of one sitemap category.
//
// Capped, because a category shard is fifty thousand URLs and seeding all of
// them is a crawl nobody asked for. --max is the cap, and the cap is stated
// rather than applied quietly.
func (a *App) sitemapSeeds(cmd *cobra.Command, category string, max int) ([]string, error) {
	ctx := cmd.Context()
	cats, err := a.client.RobotsSitemaps(ctx)
	if err != nil {
		return nil, mapFetchErr(err)
	}
	var indexURL string
	for _, c := range cats {
		if c.Category == category {
			indexURL = c.URL
			break
		}
	}
	if indexURL == "" {
		names := make([]string, 0, len(cats))
		for _, c := range cats {
			names = append(names, c.Category)
		}
		return nil, codeError(exitUsage, fmt.Errorf(
			"%q is not a sitemap category. it takes one of: %s", category, strings.Join(names, ", ")))
	}
	shards, err := a.client.SitemapIndex(ctx, indexURL)
	if err != nil {
		return nil, mapFetchErr(err)
	}
	if len(shards) == 0 {
		return nil, nil
	}
	limit := max
	if limit <= 0 {
		limit = 100
		a.progressf("--from-sitemap with no --max seeds the first %d urls", limit)
	}
	var out []string
	for _, shard := range shards {
		urls, err := a.client.SitemapURLs(ctx, shard)
		if err != nil {
			a.progressf("shard %s: %v", shard, err)
			continue
		}
		for _, u := range urls {
			out = append(out, u)
			if len(out) >= limit {
				a.progressf("seeded %d of the %s sitemap, which has more", len(out), category)
				return out, nil
			}
		}
	}
	return out, nil
}

// printPlan is what --dry-run prints.
func (a *App) printPlan(p goodread.CrawlPlan) {
	fmt.Printf("depth     %d\n", p.Depth)
	fmt.Printf("pace      %s per request\n", p.Pace.Round(100*time.Millisecond))
	fmt.Printf("pending   %d\n", p.Pending)
	for i, s := range p.Seeds {
		if i >= 10 {
			fmt.Printf("          and %d more\n", len(p.Seeds)-10)
			break
		}
		fmt.Printf("          %s  %s\n", s.URI, s.URL)
	}
	// At least, and it says so. A depth 2 crawl reaches books whose own
	// neighbours are not known until they have been read, so the honest number
	// before the crawl is a floor and not an estimate.
	fmt.Printf("requests  %d at least\n", p.Requests)
	fmt.Printf("time      %s at least\n", p.Duration.Round(time.Second))
	fmt.Println("\nnothing was read. drop --dry-run to run it.")
}

// printCrawlSummary is the block at the end of a run.
func (a *App) printCrawlSummary(st *goodread.Store, s goodread.CrawlStats) {
	if a.quiet {
		return
	}
	var b strings.Builder
	for _, k := range s.Kinds() {
		fmt.Fprintf(&b, "  %-10s %5d", k, s.ByKind[k])
	}
	if b.Len() > 0 {
		fmt.Fprintln(os.Stderr, strings.TrimRight(b.String(), " "))
	}
	fmt.Fprintf(os.Stderr, "  requests %5d   cached %5d   errors %5d   left %5d\n",
		s.Requests, s.Cached, s.Errors, s.Queued)
	fmt.Fprintf(os.Stderr, "  elapsed  %s\n", s.Elapsed.Round(time.Second))

	if info, err := os.Stat(a.storePath()); err == nil {
		fmt.Fprintf(os.Stderr, "\n%s  %s\n", a.storePath(), humanize.Bytes(uint64(info.Size())))
	}

	// What failed and why. "3 errors" is not actionable and "3 errors, all of
	// them a 404 on a series page" is.
	if s.Errors > 0 {
		items, causes, err := st.FrontierErrors(5)
		if err != nil {
			return
		}
		fmt.Fprintln(os.Stderr, "\nfailed:")
		for i, it := range items {
			fmt.Fprintf(os.Stderr, "  %s  %s\n", it.URI, causes[i])
		}
		if s.Errors > len(items) {
			fmt.Fprintf(os.Stderr, "  and %d more, in the frontier table\n", s.Errors-len(items))
		}
	}
}
