package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"

	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
	"github.com/tamnd/goodread-cli/goodread"
	"golang.org/x/sync/errgroup"
)

// seed ──────────────────────────────────────────────────────────────────────

func (a *App) seedCmd() *cobra.Command {
	var (
		enqueue  bool
		category string
		urlsMode bool
		max      int
	)
	cmd := &cobra.Command{
		Use:   "seed",
		Short: "Discover sitemap categories, shards, and page URLs",
		Long: "seed walks Goodreads' sitemap tree advertised in robots.txt.\n\n" +
			"With no flags it lists the sitemap categories (author, list, quote, ...).\n" +
			"With --type <category> it lists that category's gzipped shard sitemaps.\n" +
			"Add --urls to drill into the shards and emit actual page URLs (use --max).",
		Args: cobra.NoArgs,
		Example: "  goodread seed\n" +
			"  goodread seed --type list\n" +
			"  goodread seed --type quote --urls --max 50 --enqueue",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			cats, err := a.client.RobotsSitemaps(ctx)
			if err != nil {
				return mapFetchErr(err)
			}
			// No category: list the categories themselves.
			if category == "" {
				return a.renderOrEmpty(cats, len(cats))
			}
			var indexURL string
			for _, c := range cats {
				if c.Category == category {
					indexURL = c.URL
					break
				}
			}
			if indexURL == "" {
				return codeError(exitUsage, fmt.Errorf("unknown sitemap category %q (try: goodread seed)", category))
			}
			shards, err := a.client.SitemapIndex(ctx, indexURL)
			if err != nil {
				return mapFetchErr(err)
			}
			a.progressf("category %s: %d shards", category, len(shards))

			// Shard list mode.
			if !urlsMode {
				type shardRow struct {
					URL string `json:"url"`
				}
				rows := make([]shardRow, 0, len(shards))
				for _, s := range shards {
					rows = append(rows, shardRow{URL: s})
					if max > 0 && len(rows) >= max {
						break
					}
				}
				return a.renderOrEmpty(rows, len(rows))
			}

			// URL mode: drill into shards and emit page URLs.
			type row struct {
				URL    string `json:"url"`
				Entity string `json:"entity_type"`
			}
			var rows []row
			var st *goodread.Store
			if enqueue {
				if st, err = a.openStore(); err != nil {
					return codeError(exitError, err)
				}
			}
			for _, shard := range shards {
				urls, err := a.client.SitemapURLs(ctx, shard)
				if err != nil {
					a.progressf("shard %s: %v", shard, err)
					continue
				}
				for _, u := range urls {
					ent := goodread.InferEntityType(u)
					rows = append(rows, row{URL: u, Entity: ent})
					if enqueue {
						_ = st.Enqueue(ctx, u, ent, 0)
					}
				}
				if max > 0 && len(rows) >= max {
					rows = rows[:max]
					break
				}
			}
			if enqueue {
				a.progressf("enqueued %d URLs into %s", len(rows), a.storePath())
			}
			return a.renderOrEmpty(rows, len(rows))
		},
	}
	cmd.Flags().BoolVar(&enqueue, "enqueue", false, "enqueue discovered URLs into the crawl queue (with --urls)")
	cmd.Flags().StringVar(&category, "type", "", "sitemap category to expand (author|list|quote|genre|user|...)")
	cmd.Flags().BoolVar(&urlsMode, "urls", false, "drill into shards and emit page URLs")
	cmd.Flags().IntVar(&max, "max", 0, "cap the number of rows emitted (0 = no limit)")
	return cmd
}

// crawl ─────────────────────────────────────────────────────────────────────

func (a *App) crawlCmd() *cobra.Command {
	var (
		max   int
		parse bool
	)
	cmd := &cobra.Command{
		Use:   "crawl",
		Short: "Process the crawl queue (fetch, cache, optionally parse)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			st, err := a.openStore()
			if err != nil {
				return codeError(exitError, err)
			}
			if err := st.ResetActive(); err != nil {
				return codeError(exitError, err)
			}
			var (
				processed int
				failed    int
				mu        sync.Mutex
			)
			for max <= 0 || processed < max {
				batch := a.cfg.Workers * 2
				if max > 0 && processed+batch > max {
					batch = max - processed
				}
				items, err := st.NextPending(ctx, batch)
				if err != nil {
					return codeError(exitError, err)
				}
				if len(items) == 0 {
					break
				}
				g, gctx := errgroup.WithContext(ctx)
				g.SetLimit(a.cfg.Workers)
				for _, it := range items {
					it := it
					g.Go(func() error {
						body, ferr := a.client.CachingFetch(gctx, a.cache, a.cfg, it.URL)
						mu.Lock()
						defer mu.Unlock()
						if ferr != nil {
							failed++
							_ = st.MarkFailed(gctx, it.ID)
							a.progressf("crawl %s: %v", it.URL, ferr)
							return nil
						}
						if parse {
							a.parseAndStore(st, it, body)
						}
						processed++
						_ = st.MarkFetched(gctx, it.ID, a.cache.Path(it.URL))
						return nil
					})
				}
				if err := g.Wait(); err != nil {
					return codeError(exitError, err)
				}
				a.progressf("crawled %d (failed %d)", processed, failed)
			}
			stats, _ := st.QueueStats()
			fmt.Printf("done: processed=%d failed=%d queue=%v\n", processed, failed, stats)
			if processed == 0 {
				return codeError(exitNoData, nil)
			}
			if failed > 0 {
				return codeError(exitPartial, nil)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&max, "max", 0, "process at most this many items (0 = drain queue)")
	cmd.Flags().BoolVar(&parse, "parse", false, "parse each fetched page into the records table")
	return cmd
}

// parseAndStore parses a fetched page body by entity type and stores the record.
func (a *App) parseAndStore(st *goodread.Store, it goodread.QueueItem, body []byte) {
	doc, err := goodread.DocFromBytes(body)
	if err != nil {
		return
	}
	switch it.EntityType {
	case "book":
		_, id := goodread.Classify(it.URL)
		if b, err := goodread.ParseBook(doc, id, it.URL); err == nil {
			_ = st.Put("book", b.BookID, b.URL, b)
		}
	case "author":
		_, id := goodread.Classify(it.URL)
		if au, err := goodread.ParseAuthor(doc, id, it.URL); err == nil {
			_ = st.Put("author", au.AuthorID, au.URL, au)
		}
	case "series":
		_, id := goodread.Classify(it.URL)
		if s, _, err := goodread.ParseSeries(doc, id, it.URL); err == nil {
			_ = st.Put("series", s.SeriesID, s.URL, s)
		}
	case "list":
		_, id := goodread.Classify(it.URL)
		if l, _, err := goodread.ParseList(doc, id, it.URL); err == nil {
			_ = st.Put("list", l.ListID, l.URL, l)
		}
	case "genre":
		_, slug := goodread.Classify(it.URL)
		if g, err := goodread.ParseGenre(doc, slug, it.URL); err == nil {
			_ = st.Put("genre", g.Slug, g.URL, g)
		}
	}
}

// db ────────────────────────────────────────────────────────────────────────

func (a *App) dbCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "db",
		Short: "Inspect and export the local store",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(a.dbInfoCmd(), a.dbCountCmd(), a.dbGetCmd(), a.dbExportCmd(), a.dbVacuumCmd())
	return cmd
}

func (a *App) dbInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info",
		Short: "Summarize stored records and the crawl queue",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			st, err := a.openStore()
			if err != nil {
				return codeError(exitError, err)
			}
			counts, err := st.CountsByType()
			if err != nil {
				return codeError(exitError, err)
			}
			q, _ := st.QueueStats()
			type row struct {
				EntityType string `json:"entity_type"`
				Count      int    `json:"count"`
			}
			var rows []row
			for k, v := range counts {
				rows = append(rows, row{EntityType: k, Count: v})
			}
			if len(rows) == 0 {
				a.progressf("store has no records yet (%s)", a.storePath())
			}
			a.progressf("queue: %v", q)
			return a.render(rows)
		},
	}
}

func (a *App) dbCountCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "count [entity-type]",
		Short: "Count stored records (all types, or one)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			st, err := a.openStore()
			if err != nil {
				return codeError(exitError, err)
			}
			t := ""
			if len(args) == 1 {
				t = args[0]
			}
			n, err := st.Count(t)
			if err != nil {
				return codeError(exitError, err)
			}
			fmt.Println(n)
			return nil
		},
	}
}

func (a *App) dbGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <entity-type> <id>",
		Short: "Print a stored record as JSON",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			st, err := a.openStore()
			if err != nil {
				return codeError(exitError, err)
			}
			data, err := st.Get(args[0], args[1])
			if err != nil {
				return mapFetchErr(err)
			}
			var buf any
			_ = json.Unmarshal(data, &buf)
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(buf)
		},
	}
}

func (a *App) dbExportCmd() *cobra.Command {
	var (
		entityType string
		out        string
		format     string
	)
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export stored records to JSONL or NDJSON",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			st, err := a.openStore()
			if err != nil {
				return codeError(exitError, err)
			}
			w := os.Stdout
			if out != "" {
				f, err := os.Create(out)
				if err != nil {
					return codeError(exitError, err)
				}
				defer func() { _ = f.Close() }()
				w = f
			}
			n := 0
			err = st.Each(entityType, func(_ string, data []byte) error {
				n++
				_, werr := fmt.Fprintln(w, string(data))
				return werr
			})
			if err != nil {
				return codeError(exitError, err)
			}
			a.progressf("exported %d records (%s)", n, format)
			if n == 0 {
				return codeError(exitNoData, nil)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&entityType, "type", "", "only export this entity type")
	cmd.Flags().StringVarP(&out, "out", "o", "", "output file (default: stdout)")
	cmd.Flags().StringVar(&format, "format", "jsonl", "export format: jsonl")
	return cmd
}

func (a *App) dbVacuumCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "vacuum",
		Short: "Compact the store database file",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			st, err := a.openStore()
			if err != nil {
				return codeError(exitError, err)
			}
			if err := st.Vacuum(); err != nil {
				return codeError(exitError, err)
			}
			fmt.Println("ok")
			return nil
		},
	}
}

func (a *App) storePath() string {
	if a.storePtr != "" {
		return a.storePtr
	}
	return a.cfg.StorePath()
}

// open ──────────────────────────────────────────────────────────────────────

func (a *App) openCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "open <id|url>",
		Short: "Open a Goodreads page in the default browser",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			url := resolveURL(args[0])
			if url == "" {
				return codeError(exitUsage, fmt.Errorf("could not resolve %q to a URL", args[0]))
			}
			return openBrowser(url)
		},
	}
}

func resolveURL(arg string) string {
	if strings.HasPrefix(arg, "http") {
		return arg
	}
	entity, id := goodread.Classify(arg)
	switch entity {
	case "book":
		return goodread.BookURL(id)
	case "author":
		return goodread.AuthorURL(id)
	case "series":
		return goodread.SeriesURL(id)
	case "user":
		return goodread.UserURL(id)
	case "list":
		return goodread.ListURL(arg)
	case "genre":
		return goodread.GenreURL(arg)
	case "quote":
		return goodread.QuoteURL(id)
	}
	return ""
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

// cache ─────────────────────────────────────────────────────────────────────

func (a *App) cacheCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cache",
		Short: "Inspect and clear the on-disk page cache",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "info",
			Short: "Show cache location, file count, and size",
			Args:  cobra.NoArgs,
			RunE: func(_ *cobra.Command, _ []string) error {
				files, bytes, err := a.cache.Stats()
				if err != nil {
					return codeError(exitError, err)
				}
				fmt.Printf("dir:   %s\nfiles: %d\nsize:  %s\n", a.cfg.CacheDir(), files, humanize.Bytes(uint64(bytes)))
				return nil
			},
		},
		&cobra.Command{
			Use:   "clear",
			Short: "Delete the entire page cache",
			Args:  cobra.NoArgs,
			RunE: func(_ *cobra.Command, _ []string) error {
				if err := a.cache.Clear(); err != nil {
					return codeError(exitError, err)
				}
				fmt.Println("cache cleared")
				return nil
			},
		},
		&cobra.Command{
			Use:   "path <url>",
			Short: "Print the cache path for a URL",
			Args:  cobra.ExactArgs(1),
			RunE: func(_ *cobra.Command, args []string) error {
				fmt.Println(a.cache.Path(args[0]))
				return nil
			},
		},
	)
	return cmd
}

// info ──────────────────────────────────────────────────────────────────────

func (a *App) infoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info",
		Short: "Show configuration, paths, and the affiliation disclaimer",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			fmt.Printf(`goodread %s

goodread is an independent, open-source tool. It is not affiliated with,
endorsed by, or sponsored by Goodreads or Amazon. It reads only public pages,
at a polite default rate, and respects each page's sign-in wall.

data dir:  %s
cache dir: %s
store:     %s
workers:   %d
delay:     %s
cache TTL: %s
`,
				Version, a.cfg.DataDir, a.cfg.CacheDir(), a.storePath(),
				a.cfg.Workers, a.cfg.Delay, a.cfg.CacheTTL)
			return nil
		},
	}
}
