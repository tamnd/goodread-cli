package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/tamnd/goodread-cli/goodread"
)

// book ──────────────────────────────────────────────────────────────────────

func (a *App) bookCmd() *cobra.Command {
	var withReviews bool
	var check bool
	var depth string
	cmd := &cobra.Command{
		Use:     "book <id|url> [id|url ...]",
		Short:   "Fetch one or more books",
		Args:    cobra.MinimumNArgs(1),
		Example: "  goodread book 2767052\n  goodread book https://www.goodreads.com/book/show/2767052 --format json\n  goodread book 2767052 --check",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if check {
				d, ok := goodread.ParseDepth(depth)
				if !ok {
					return codeError(exitUsage, fmt.Errorf("unknown depth %q, want one of %s", depth, depthList()))
				}
				return a.bookCheck(ctx, args, d)
			}
			var books []*goodread.ScrapedBook
			for _, arg := range args {
				_, id := goodread.Classify(arg)
				if id == "" {
					id = arg
				}
				b, err := a.client.GetBook(ctx, id)
				if err != nil {
					if len(args) == 1 {
						return mapFetchErr(err)
					}
					a.progressf("book %s: %v", arg, err)
					continue
				}
				if a.store != nil {
					_ = a.store.Put("book", b.BookID, b.URL, b)
				}
				books = append(books, b)
			}
			if withReviews {
				// reviews embedded on the page are surfaced via the reviews command;
				// here we just keep the book records.
				_ = withReviews
			}
			return a.renderOrEmpty(books, len(books))
		},
	}
	cmd.Flags().BoolVar(&withReviews, "with-reviews", false, "also fetch embedded reviews (use the reviews command for detail)")
	cmd.Flags().BoolVar(&check, "check", false, "read the book into the v0.3.0 model and reconcile its numbers")
	cmd.Flags().StringVar(&depth, "depth", "meta", "how much to read: "+depthList())
	return cmd
}

func depthList() string {
	var out []string
	for _, d := range goodread.Depths() {
		out = append(out, string(d))
	}
	return strings.Join(out, ", ")
}

// bookCheck reads a book with the extractor and reconciles its statistics.
//
// Two reconciliations. The histogram has to sum to the ratings count, and the
// mean derived from the histogram has to match the published average. The
// second is the one worth having: five integers with no labels are exactly the
// kind of thing that ends up reversed, and no amount of looking at the numbers
// would tell you.
func (a *App) bookCheck(ctx context.Context, args []string, depth goodread.Depth) error {
	type result struct {
		ID      string              `json:"id"`
		Title   string              `json:"title"`
		Check   goodread.StatsCheck `json:"check"`
		Missed  []string            `json:"missed,omitempty"`
		Fields  int                 `json:"fields"`
		Surface string              `json:"surface"`
	}

	var out []result
	problems := 0
	for _, arg := range args {
		_, id := goodread.Classify(arg)
		if id == "" {
			id = arg
		}
		rec, err := a.client.GetBookRecord(ctx, id, depth)
		if err != nil {
			if len(args) == 1 {
				return mapFetchErr(err)
			}
			a.progressf("book %s: %v", arg, err)
			continue
		}
		r := result{
			ID:     id,
			Title:  rec.Book.Title,
			Check:  rec.Book.Stats.Check(),
			Missed: rec.Book.Missed,
			Fields: len(rec.Book.Via),
		}
		if len(rec.Book.Surfaces) > 0 {
			r.Surface = rec.Book.Surfaces[0]
		}
		if !r.Check.OK {
			problems++
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return codeError(exitNoData, fmt.Errorf("nothing read"))
	}

	if a.format == string(FormatJSON) || a.format == string(FormatJSONL) {
		if err := a.render(out); err != nil {
			return err
		}
	} else {
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		if !a.noHeader {
			fmt.Fprintln(w, "id\ttitle\tfields\tbuckets sum\tratings\tderived\tpublished\tok")
		}
		for _, r := range out {
			fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%d\t%.4f\t%.2f\t%v\n",
				r.ID, truncate(r.Title, 40), r.Fields,
				r.Check.SumOfBuckets, r.Check.RatingsCount, r.Check.DerivedMean, r.Check.Published, r.Check.OK)
		}
		if err := w.Flush(); err != nil {
			return err
		}
		for _, r := range out {
			for _, p := range r.Check.Problems {
				fmt.Fprintf(os.Stderr, "%s: %s\n", r.ID, p)
			}
		}
	}
	if problems > 0 {
		return codeError(exitPartial, fmt.Errorf("%d book(s) did not reconcile", problems))
	}
	return nil
}

// author ────────────────────────────────────────────────────────────────────

func (a *App) authorCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "author <id|url> [id|url ...]",
		Short:   "Fetch one or more authors",
		Args:    cobra.MinimumNArgs(1),
		Example: "  goodread author 153394\n  goodread author 153394 1077326 --format csv",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			var authors []*goodread.Author
			for _, arg := range args {
				_, id := goodread.Classify(arg)
				if id == "" {
					id = arg
				}
				au, err := a.client.GetAuthor(ctx, id)
				if err != nil {
					if len(args) == 1 {
						return mapFetchErr(err)
					}
					a.progressf("author %s: %v", arg, err)
					continue
				}
				if a.store != nil {
					_ = a.store.Put("author", au.AuthorID, au.URL, au)
				}
				authors = append(authors, au)
			}
			return a.renderOrEmpty(authors, len(authors))
		},
	}
}

// series ────────────────────────────────────────────────────────────────────

func (a *App) seriesCmd() *cobra.Command {
	var booksOnly bool
	cmd := &cobra.Command{
		Use:   "series <id|url>",
		Short: "Fetch a series and its books",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, id := goodread.Classify(args[0])
			if id == "" {
				id = args[0]
			}
			s, books, err := a.client.GetSeries(cmd.Context(), id)
			if err != nil {
				return mapFetchErr(err)
			}
			if a.store != nil {
				_ = a.store.Put("series", s.SeriesID, s.URL, s)
			}
			if booksOnly {
				if a.limit > 0 && len(books) > a.limit {
					books = books[:a.limit]
				}
				return a.renderOrEmpty(books, len(books))
			}
			return a.renderOrEmpty([]*goodread.Series{s}, 1)
		},
	}
	cmd.Flags().BoolVar(&booksOnly, "books", false, "list the series' books instead of the series header")
	return cmd
}

// list ──────────────────────────────────────────────────────────────────────

func (a *App) listCmd() *cobra.Command {
	var booksOnly bool
	cmd := &cobra.Command{
		Use:     "list <id|url>",
		Short:   "Fetch a Listopia list and its books",
		Args:    cobra.ExactArgs(1),
		Example: "  goodread list 1.Best_Books_Ever --books",
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			if e, got := goodread.Classify(args[0]); e == "list" && got != "" {
				id = got
			}
			l, books, err := a.client.GetList(cmd.Context(), id)
			if err != nil {
				return mapFetchErr(err)
			}
			if a.store != nil {
				_ = a.store.Put("list", l.ListID, l.URL, l)
			}
			if booksOnly {
				if a.limit > 0 && len(books) > a.limit {
					books = books[:a.limit]
				}
				return a.renderOrEmpty(books, len(books))
			}
			return a.renderOrEmpty([]*goodread.List{l}, 1)
		},
	}
	cmd.Flags().BoolVar(&booksOnly, "books", false, "list the list's books instead of the list header")
	return cmd
}

// quote ─────────────────────────────────────────────────────────────────────

func (a *App) quoteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "quote <url|author-id|book-id>",
		Short: "Fetch quotes from a quotes page, author, or book",
		Args:  cobra.ExactArgs(1),
		Example: "  goodread quote https://www.goodreads.com/author/quotes/153394\n" +
			"  goodread quote https://www.goodreads.com/work/quotes/2792775",
		RunE: func(cmd *cobra.Command, args []string) error {
			url := quotesURL(args[0])
			quotes, err := a.client.GetQuotes(cmd.Context(), url)
			if err != nil {
				return mapFetchErr(err)
			}
			if a.store != nil {
				for _, q := range quotes {
					_ = a.store.Put("quote", q.QuoteID, q.URL, q)
				}
			}
			quotes = limitQuotes(quotes, a.limit)
			return a.renderOrEmpty(quotes, len(quotes))
		},
	}
}

func quotesURL(arg string) string {
	if len(arg) > 4 && (arg[:4] == "http") {
		return arg
	}
	// bare id -> author quotes page
	return goodread.BaseURL + "/author/quotes/" + arg
}

func limitQuotes(q []goodread.Quote, n int) []goodread.Quote {
	if n > 0 && len(q) > n {
		return q[:n]
	}
	return q
}

// user ──────────────────────────────────────────────────────────────────────

func (a *App) userCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "user <id|url>",
		Short: "Fetch a public user profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, id := goodread.Classify(args[0])
			if id == "" {
				id = args[0]
			}
			u, err := a.client.GetUser(cmd.Context(), id)
			if err != nil {
				return mapFetchErr(err)
			}
			if a.store != nil {
				_ = a.store.Put("user", u.UserID, u.URL, u)
			}
			return a.renderOrEmpty([]*goodread.ScrapedUser{u}, 1)
		},
	}
}

// shelf ─────────────────────────────────────────────────────────────────────

func (a *App) shelfCmd() *cobra.Command {
	var shelfName string
	var maxPages int
	var useHTML bool
	cmd := &cobra.Command{
		Use:   "shelf <user-id|url>",
		Short: "Fetch a user's shelf",
		Long: "shelf reads a user's bookshelf. By default it uses the open RSS feed,\n" +
			"which returns rich rows (author, isbn, rating, dates, review text) and is\n" +
			"not WAF-challenged. Use --html to walk the paginated HTML shelf instead.",
		Args:    cobra.ExactArgs(1),
		Example: "  goodread shelf 1 --shelf read\n  goodread shelf 1 --shelf read --html --max-pages 3",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, id := goodread.Classify(args[0])
			if id == "" {
				id = args[0]
			}
			var (
				shelf *goodread.Shelf
				rows  []goodread.ShelfBook
				err   error
			)
			if useHTML {
				shelf, rows, err = a.client.GetShelf(cmd.Context(), id, shelfName, maxPages)
			} else {
				shelf, rows, err = a.client.GetShelfRSS(cmd.Context(), id, shelfName)
			}
			if err != nil {
				return mapFetchErr(err)
			}
			if a.store != nil {
				_ = a.store.Put("shelf", shelf.ShelfID, shelf.URL, shelf)
			}
			rows = limitShelf(rows, a.limit)
			return a.renderOrEmpty(rows, len(rows))
		},
	}
	cmd.Flags().StringVar(&shelfName, "shelf", "read", "shelf name: read|currently-reading|to-read|<custom>")
	cmd.Flags().IntVar(&maxPages, "max-pages", 1, "maximum pages to walk in --html mode (0 = all)")
	cmd.Flags().BoolVar(&useHTML, "html", false, "use the paginated HTML shelf instead of the RSS feed")
	return cmd
}

func limitShelf(s []goodread.ShelfBook, n int) []goodread.ShelfBook {
	if n > 0 && len(s) > n {
		return s[:n]
	}
	return s
}

// genre ─────────────────────────────────────────────────────────────────────

func (a *App) genreCmd() *cobra.Command {
	var booksOnly bool
	cmd := &cobra.Command{
		Use:     "genre <slug|url>",
		Short:   "Fetch a genre and its featured books",
		Args:    cobra.ExactArgs(1),
		Example: "  goodread genre fantasy --books",
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := args[0]
			if e, got := goodread.Classify(args[0]); e == "genre" && got != "" {
				slug = got
			}
			g, err := a.client.GetGenre(cmd.Context(), slug)
			if err != nil {
				return mapFetchErr(err)
			}
			if a.store != nil {
				_ = a.store.Put("genre", g.Slug, g.URL, g)
			}
			if booksOnly {
				ids := g.BookIDs
				if a.limit > 0 && len(ids) > a.limit {
					ids = ids[:a.limit]
				}
				rows := make([]idRow, len(ids))
				for i, id := range ids {
					rows[i] = idRow{BookID: id, URL: goodread.BookURL(id)}
				}
				return a.renderOrEmpty(rows, len(rows))
			}
			return a.renderOrEmpty([]*goodread.Genre{g}, 1)
		},
	}
	cmd.Flags().BoolVar(&booksOnly, "books", false, "list the genre's book ids instead of the genre header")
	return cmd
}

type idRow struct {
	BookID string `json:"book_id"`
	URL    string `json:"url"`
}

// search ────────────────────────────────────────────────────────────────────

func (a *App) searchCmd() *cobra.Command {
	var htmlOnly bool
	var booksMode bool
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search books and authors",
		Long: "search returns books and authors matching a query. By default it uses\n" +
			"the open autocomplete endpoint (concise results: type, title, url).\n" +
			"Use --books for rich book records (rating, pages, author, description).",
		Args:    cobra.MinimumNArgs(1),
		Example: "  goodread search \"the hunger games\" -n 10\n  goodread search dune --books --format json",
		RunE: func(cmd *cobra.Command, args []string) error {
			q := joinArgs(args)
			if booksMode {
				books, err := a.client.SearchBooks(cmd.Context(), q, a.limit)
				if err != nil {
					return mapFetchErr(err)
				}
				if a.store != nil {
					for i := range books {
						_ = a.store.Put("book", books[i].BookID, books[i].URL, books[i])
					}
				}
				return a.renderOrEmpty(books, len(books))
			}
			var (
				res []goodread.SearchResult
				err error
			)
			if htmlOnly {
				res, err = a.client.SearchHTML(cmd.Context(), q, a.limit)
			} else {
				res, err = a.client.Search(cmd.Context(), q, a.limit)
			}
			if err != nil {
				return mapFetchErr(err)
			}
			return a.renderOrEmpty(res, len(res))
		},
	}
	cmd.Flags().BoolVar(&booksMode, "books", false, "return rich book records from autocomplete")
	cmd.Flags().BoolVar(&htmlOnly, "html", false, "skip autocomplete and use full HTML search (paginates)")
	return cmd
}

// reviews ───────────────────────────────────────────────────────────────────

func (a *App) reviewsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reviews <book-id|url>",
		Short: "Fetch reviews embedded on a book page",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, id := goodread.Classify(args[0])
			if id == "" {
				id = args[0]
			}
			reviews, err := a.client.GetReviews(cmd.Context(), id)
			if err != nil {
				return mapFetchErr(err)
			}
			if a.store != nil {
				for _, rv := range reviews {
					_ = a.store.Put("review", rv.ReviewID, rv.URL, rv)
				}
			}
			if a.limit > 0 && len(reviews) > a.limit {
				reviews = reviews[:a.limit]
			}
			return a.renderOrEmpty(reviews, len(reviews))
		},
	}
}

// similar ───────────────────────────────────────────────────────────────────

func (a *App) similarCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "similar <book-id|url>",
		Short: "List books similar to a given book",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, id := goodread.Classify(args[0])
			if id == "" {
				id = args[0]
			}
			b, err := a.client.GetBook(cmd.Context(), id)
			if err != nil {
				return mapFetchErr(err)
			}
			ids := b.SimilarBookIDs
			if a.limit > 0 && len(ids) > a.limit {
				ids = ids[:a.limit]
			}
			rows := make([]idRow, len(ids))
			for i, sid := range ids {
				rows[i] = idRow{BookID: sid, URL: goodread.BookURL(sid)}
			}
			return a.renderOrEmpty(rows, len(rows))
		},
	}
}

// id ────────────────────────────────────────────────────────────────────────

func (a *App) idCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "id <url|id> [url|id ...]",
		Short: "Classify a URL or id into (entity, id) without fetching",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			type row struct {
				Input  string `json:"input"`
				Entity string `json:"entity"`
				ID     string `json:"id"`
			}
			rows := make([]row, 0, len(args))
			for _, arg := range args {
				e, id := goodread.Classify(arg)
				rows = append(rows, row{Input: arg, Entity: e, ID: id})
			}
			return a.renderOrEmpty(rows, len(rows))
		},
	}
}

func joinArgs(args []string) string {
	return strings.Join(args, " ")
}

// versionCmd prints build metadata.
func (a *App) versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version, commit, and build date",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Printf("goodread %s (commit %s, built %s)\n", Version, Commit, Date)
			return nil
		},
	}
}
