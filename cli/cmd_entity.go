package cli

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/tamnd/goodread-cli/goodread"
)

// book ──────────────────────────────────────────────────────────────────────

func (a *App) bookCmd() *cobra.Command {
	var withReviews bool
	var check bool
	cmd := &cobra.Command{
		Use:     "book <id|url> [id|url ...]",
		Short:   "Fetch one or more books",
		Args:    cobra.MinimumNArgs(1),
		Example: "  goodread book 2767052\n  goodread book https://www.goodreads.com/book/show/2767052 --json\n  goodread book 2767052 --check",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if check {
				return a.bookCheck(ctx, args, a.depth)
			}
			return a.bookRun(ctx, args)
		},
	}
	cmd.Flags().BoolVar(&withReviews, "with-reviews", false, "also fetch embedded reviews (use the reviews command for detail)")
	cmd.Flags().BoolVar(&check, "check", false, "read the book into the v0.3.0 model and reconcile its numbers")
	_ = withReviews
	return cmd
}

// bookRun reads books into the v0.3.0 model.
//
// This is the command that moved off parse.go. The record it prints carries the
// ratings histogram, the provenance of every field and the not read block, none
// of which the selector path could produce.
func (a *App) bookRun(ctx context.Context, args []string) error {
	var recs []*goodread.BookRecord
	for _, arg := range args {
		ref, err := goodread.ParseRefAs(arg, "book")
		if err != nil {
			return codeError(exitUsage, err)
		}
		a.verbosef(1, "reading %s at depth %s, %d request(s)", ref, a.depth, a.depth.Requests())
		rec, err := a.client.GetBookRecord(ctx, ref.ID, a.depth)
		if err != nil {
			if len(args) == 1 {
				return mapFetchErr(err)
			}
			a.progressf("book %s: %v", arg, err)
			continue
		}
		for f, via := range rec.Book.Via {
			a.verbosef(2, "  %s via %s level %d", f, via, rec.Book.Level[f])
		}
		if a.store != nil {
			_ = a.store.Put("book", strconv.FormatInt(rec.Book.LegacyID, 10), rec.Book.WebURL, rec)
		}
		recs = append(recs, rec)
	}
	if len(recs) == 0 {
		return codeError(exitNotFound, nil)
	}

	switch Format(a.format) {
	case FormatTable:
		for i, rec := range recs {
			if i > 0 {
				fmt.Fprintln(os.Stdout)
			}
			printBook(os.Stdout, rec.Book, rec.Work)
		}
		return nil
	case FormatJSON, FormatJSONL:
		return a.render(recs)
	default:
		// csv, tsv, url, raw and --template all want one flat row per book, so
		// they get the book and not the record around it.
		books := make([]*goodread.Book, 0, len(recs))
		for _, rec := range recs {
			books = append(books, rec.Book)
		}
		return a.render(books)
	}
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
		return codeError(exitNotFound, fmt.Errorf("nothing read"))
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
		return codeError(exitParse, fmt.Errorf("%d book(s) did not reconcile", problems))
	}
	return nil
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
	var booksOnly bool
	cmd := &cobra.Command{
		Use:   "shelf <user-id|url>",
		Short: "Fetch a user's shelf",
		Long: "shelf reads a user's bookshelf. Both routes are disallowed by robots.txt\n" +
			"and need --no-robots.\n\n" +
			"By default it reads the open RSS feed, which returns one window of the\n" +
			"shelf and carries more per row than the rendered page does: author, isbn,\n" +
			"page count, the person's rating, their shelves, their dates and their\n" +
			"review. --html walks the paginated HTML shelf instead, which is the only\n" +
			"way to get the whole shelf and costs a megabyte a page.",
		Args:    cobra.ExactArgs(1),
		Example: "  goodread shelf 1 --shelf read --no-robots\n  goodread shelf 1 --shelf read --html --max-pages 3 --no-robots",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, id := goodread.Classify(args[0])
			if id == "" {
				id = args[0]
			}
			a.verbosef(1, "reading %s shelf %q over %s", id, shelfName, shelfRoute(useHTML))
			rec, err := a.client.GetShelfRecord(cmd.Context(), id, shelfName, useHTML, maxPages)
			if err != nil {
				return mapFetchErr(err)
			}
			a.reportLevels(rec.Envelope)
			if a.store != nil {
				_ = a.store.Put("shelf", rec.ID, rec.WebURL, rec)
			}
			if a.limit > 0 && len(rec.Books) > a.limit {
				rec.Books = rec.Books[:a.limit]
			}
			if booksOnly {
				return a.renderOrEmpty(rec.Books, len(rec.Books))
			}
			return a.renderRecords([]*goodread.ShelfRecord{rec}, func() { printShelf(os.Stdout, rec) })
		},
	}
	cmd.Flags().StringVar(&shelfName, "shelf", "read", "shelf name: read|currently-reading|to-read|<custom>")
	cmd.Flags().IntVar(&maxPages, "max-pages", 1, "maximum pages to walk in --html mode (0 = all)")
	cmd.Flags().BoolVar(&useHTML, "html", false, "walk the paginated HTML shelf instead of the RSS feed")
	cmd.Flags().BoolVar(&booksOnly, "books", false, "list the rows instead of the shelf header")
	return cmd
}

func shelfRoute(useHTML bool) string {
	if useHTML {
		return "the html shelf"
	}
	return "the rss feed"
}

type idRow struct {
	BookID string `json:"book_id"`
	URL    string `json:"url"`
}

// search ────────────────────────────────────────────────────────────────────

func (a *App) searchCmd() *cobra.Command {
	var deep bool
	var booksMode bool
	var searchType, field string
	var pages int
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search books and authors",
		Long: "search returns books matching a query. By default it reads\n" +
			"/book/auto_complete, which robots.txt allows, needs no key and carries the\n" +
			"book id, work id, title, series, author, page count, rating, ratings count\n" +
			"and description.\n\n" +
			"--deep reads /search as well, which robots.txt disallows, so it needs\n" +
			"--no-robots. That page adds the totals, the published year, the edition\n" +
			"count, the pagination, the genre the query mapped to and the related\n" +
			"shelves.\n\n" +
			"--type picks a tab. Signed out, books answers, people answers with nothing,\n" +
			"and lists, groups and quotes answer with a challenge this tool reports\n" +
			"rather than works around.",
		Args: cobra.MinimumNArgs(1),
		Example: "  goodread search \"the hunger games\"\n" +
			"  goodread search dune --deep --no-robots --format json\n" +
			"  goodread search dune --deep --no-robots --pages 3\n" +
			"  goodread search \"le guin\" --deep --no-robots --field author",
		RunE: func(cmd *cobra.Command, args []string) error {
			query := joinArgs(args)
			if booksMode {
				books, err := a.client.SearchBooks(cmd.Context(), query, a.limit)
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
				rec *goodread.SearchRecord
				err error
			)
			if deep {
				sq := goodread.SearchQuery{Query: query, Type: searchType, Field: field}
				rec, err = a.client.GetSearchRecord(cmd.Context(), sq, pages)
			} else {
				if searchType != "" && searchType != goodread.SearchTypeBooks {
					return codeError(exitUsage, fmt.Errorf(
						"--type only applies to --deep, since /book/auto_complete answers about books and nothing else"))
				}
				rec, err = a.client.GetSuggestRecord(cmd.Context(), query)
			}
			if err != nil {
				return mapFetchErr(err)
			}
			a.reportLevels(rec.Envelope)
			return a.emitSearch(rec)
		},
	}
	// --books is the v0.2.0 shape. It keeps working for the scripts that read
	// it and comes out of the help, because the record it predates carries
	// everything it carried and more, and advertising both invites somebody to
	// pick the smaller one.
	cmd.Flags().BoolVar(&booksMode, "books", false, "deprecated: return the rows as v0.2.0 book records")
	_ = cmd.Flags().MarkHidden("books")
	cmd.Flags().BoolVar(&deep, "deep", false, "read /search as well, which needs --no-robots")
	cmd.Flags().StringVar(&searchType, "type", "", "which tab: books, people, lists, groups, quotes")
	cmd.Flags().StringVar(&field, "field", "", "narrow the match: all, title, author")
	cmd.Flags().IntVar(&pages, "pages", 1, "how many pages of /search to walk, at the pace floor")
	// --html is what v0.2.0 called it. Kept working and kept out of the help,
	// because renaming a flag and deleting it in the same release is two
	// breaking changes where one will do.
	cmd.Flags().BoolVar(&deep, "html", false, "deprecated alias for --deep")
	_ = cmd.Flags().MarkHidden("html")
	return cmd
}

// suggestCmd is the allowed surface under its own name.
//
// The same read `search` does by default. It gets a name of its own because
// "search without --deep" is a description of a flag rather than of a thing,
// and a script that wants the endpoint that needs no override should be able to
// ask for it and not have to know that the default happens to be that today.
func (a *App) suggestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "suggest <query>",
		Short: "Read the allowed search endpoint",
		Long: "suggest reads /book/auto_complete, which robots.txt allows. It is a\n" +
			"handful of rows, has no total and does not paginate, and it is the only\n" +
			"surface on the site that hands over a book id and its work id in one\n" +
			"response.",
		Args:    cobra.MinimumNArgs(1),
		Example: "  goodread suggest dune\n  goodread suggest dune --format json",
		RunE: func(cmd *cobra.Command, args []string) error {
			rec, err := a.client.GetSuggestRecord(cmd.Context(), joinArgs(args))
			if err != nil {
				return mapFetchErr(err)
			}
			a.reportLevels(rec.Envelope)
			return a.emitSearch(rec)
		},
	}
}

// emitSearch stores the record, applies --limit and renders it.
//
// --limit trims the rows and nothing else. The totals stay as the site stated
// them, because ten rows of eight hundred and sixteen is still eight hundred
// and sixteen results, and a limit that edited the total would be this tool
// lying about the site.
func (a *App) emitSearch(rec *goodread.SearchRecord) error {
	if a.store != nil {
		_ = a.store.Put("search", rec.Query, rec.WebURL, rec)
	}
	n := len(rec.Results)
	if a.limit > 0 && n > a.limit {
		rec.Results = rec.Results[:a.limit]
	}
	if Format(a.format) == FormatTable {
		printSearch(os.Stdout, rec, n)
		if n == 0 {
			return codeError(exitNotFound, nil)
		}
		return nil
	}
	return a.renderOrEmpty(rec, n)
}

// reviews ───────────────────────────────────────────────────────────────────

func (a *App) reviewsCmd() *cobra.Command {
	var all bool
	var yes bool
	var maxPages int
	cmd := &cobra.Command{
		Use:   "reviews <book-id|url>",
		Short: "Read a book's reviews",
		Long: "reviews reads the sample of reviews the book page already carries, which\n" +
			"is one allowed request and needs no flag.\n\n" +
			"--all walks /book/reviews, which robots.txt disallows, so it needs\n" +
			"--no-robots as well. It prints what that costs and needs --yes before it\n" +
			"starts. Worth knowing: Goodreads paginates that page to ten pages of\n" +
			"thirty, so --all reaches about 300 reviews and not the whole set.",
		Args:    cobra.ExactArgs(1),
		Example: "  goodread reviews 2767052\n  goodread reviews 2767052 --all --no-robots --yes",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			ref, err := goodread.ParseRefAs(args[0], "book")
			if err != nil {
				return codeError(exitUsage, err)
			}

			a.verbosef(1, "reading the book page for the review sample")
			rec, err := a.client.GetBookRecord(ctx, ref.ID, goodread.DepthQuick)
			if err != nil {
				return mapFetchErr(err)
			}
			reviews := rec.Book.Reviews

			if all {
				var total int64
				if rec.Book.Stats != nil && rec.Book.Stats.TextReviewsCount != nil {
					total = *rec.Book.Stats.TextReviewsCount
				}
				cost := goodread.EstimateReviews(total, a.cfg.Delay)
				fmt.Fprintln(os.Stderr, cost)
				if !yes {
					return codeError(exitUsage, fmt.Errorf(
						"pass --yes to spend those %d requests", cost.Requests))
				}
				more, err := a.client.GetReviewPages(ctx, ref.ID, maxPages)
				if err != nil {
					// Whatever was read before the error is still worth having,
					// and saying how far it got beats returning nothing.
					if len(more) == 0 {
						return mapFetchErr(err)
					}
					a.progressf("stopped after %d reviews: %v", len(more), err)
				}
				reviews = mergeReviews(reviews, more)
			}

			if a.limit > 0 && len(reviews) > a.limit {
				reviews = reviews[:a.limit]
			}
			if a.store != nil {
				for _, rv := range reviews {
					_ = a.store.Put("review", rv.ID, "", rv)
				}
			}
			for _, m := range rec.Book.Missed {
				a.verbosef(1, "not read: %s", m)
			}
			return a.renderOrEmpty(reviews, len(reviews))
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "walk /book/reviews as well, which needs --no-robots and --yes")
	cmd.Flags().BoolVar(&yes, "yes", false, "go ahead with the requests --all costs")
	cmd.Flags().IntVar(&maxPages, "max-pages", goodread.MaxReviewPages, "pages of /book/reviews to walk with --all")
	return cmd
}

// mergeReviews keeps both sets without pretending they are one.
//
// The cache ids are kca uris and the page ids are legacy integers, so nothing
// links a review from one to the same review in the other. Dropping either set
// would lose reviews, so both are kept and the duplication is stated here
// rather than hidden behind a merge that cannot work.
func mergeReviews(cached, paged []goodread.Review) []goodread.Review {
	out := make([]goodread.Review, 0, len(cached)+len(paged))
	out = append(out, cached...)
	out = append(out, paged...)
	return out
}

// similar ───────────────────────────────────────────────────────────────────

// similarCmd is named after a page that no longer answers, and says so.
//
// /book/similar/<id> was the recommendation surface and it is gone: it replies
// with an unrelated canonical, so there is nothing to read. The strip the book
// page renders as "Readers also enjoyed" is not in the anonymous HTML either.
// What is left is the other books the page links to, which are mostly the rest
// of the series and whatever the reviews mention, and calling that a
// recommendation would be this tool inventing a ranking Goodreads did not
// publish. The command stays because a link list is still useful for crawling;
// the name stays because renaming it would break scripts for no gain, and the
// help is where the correction belongs.
func (a *App) similarCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "similar <book-id|url>",
		Short: "List the other books a book's page links to",
		Long: "similar lists the other books linked from a book's page, in the order\n" +
			"the page links them, capped at twenty.\n\n" +
			"These are not recommendations and they are not ranked. Goodreads used\n" +
			"to publish /book/similar/<id> and that page no longer answers, and the\n" +
			"\"Readers also enjoyed\" strip is not in the HTML an anonymous reader\n" +
			"gets. What this returns is usually the rest of the series and the books\n" +
			"mentioned in the reviews on the page.",
		Args: cobra.ExactArgs(1),
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
			a.progressf("note: these are the books this page links to, not recommendations. the page that ranked them is gone.")
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
