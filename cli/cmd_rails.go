package cli

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tamnd/goodread-cli/goodread"
)

// The six Rails surfaces on the v0.3.0 model.
//
// Each one follows the same shape as bookCmd: the record goes to json and jsonl
// whole, and the flat formats get the rows inside it, because a csv of one
// record with a nested list in a column is of no use to anybody. --books and
// friends pick the rows out for the table too.

// author ────────────────────────────────────────────────────────────────────

func (a *App) authorCmd() *cobra.Command {
	var booksOnly bool
	cmd := &cobra.Command{
		Use:     "author <id|url> [id|url ...]",
		Short:   "Fetch one or more authors",
		Args:    cobra.MinimumNArgs(1),
		Example: "  goodread author 153394\n  goodread author 1077326 --books\n  goodread author 153394 1077326 --format csv",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			var recs []*goodread.Author
			for _, arg := range args {
				ref, err := goodread.ParseRefAs(arg, "author")
				if err != nil {
					return codeError(exitUsage, err)
				}
				a.verbosef(1, "reading author %s", ref.ID)
				au, err := a.client.GetAuthorRecord(ctx, ref.ID)
				if err != nil {
					if len(args) == 1 {
						return mapFetchErr(err)
					}
					a.progressf("author %s: %v", arg, err)
					continue
				}
				a.reportLevels(au.Envelope)
				if a.store != nil {
					_ = a.store.Put("author", strconv.FormatInt(au.LegacyID, 10), au.WebURL, au)
				}
				recs = append(recs, au)
			}
			if len(recs) == 0 {
				return codeError(exitNotFound, nil)
			}
			if booksOnly {
				var cards []goodread.BookCard
				for _, au := range recs {
					cards = append(cards, au.Books...)
				}
				return a.renderCards(cards)
			}
			return a.renderRecords(recs, func() {
				for i, au := range recs {
					if i > 0 {
						fmt.Fprintln(os.Stdout)
					}
					printAuthor(os.Stdout, au)
				}
			})
		},
	}
	cmd.Flags().BoolVar(&booksOnly, "books", false, "list the author's books instead of the author")
	return cmd
}

// series ────────────────────────────────────────────────────────────────────

func (a *App) seriesCmd() *cobra.Command {
	var booksOnly bool
	cmd := &cobra.Command{
		Use:     "series <id|url>",
		Short:   "Fetch a series and its books",
		Args:    cobra.ExactArgs(1),
		Example: "  goodread series 45175\n  goodread series 45175 --books",
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := goodread.ParseRefAs(args[0], "series")
			if err != nil {
				return codeError(exitUsage, err)
			}
			s, err := a.client.GetSeriesRecord(cmd.Context(), ref.ID)
			if err != nil {
				return mapFetchErr(err)
			}
			a.reportLevels(s.Envelope)
			if a.store != nil {
				_ = a.store.Put("series", strconv.FormatInt(s.LegacyID, 10), s.WebURL, s)
			}
			if booksOnly {
				books := s.Books
				if a.limit > 0 && len(books) > a.limit {
					books = books[:a.limit]
				}
				return a.renderOrEmpty(books, len(books))
			}
			return a.renderRecords([]*goodread.Series{s}, func() { printSeries(os.Stdout, s) })
		},
	}
	cmd.Flags().BoolVar(&booksOnly, "books", false, "list the series' books instead of the series header")
	return cmd
}

// list ──────────────────────────────────────────────────────────────────────

func (a *App) listCmd() *cobra.Command {
	var booksOnly bool
	var page int
	cmd := &cobra.Command{
		Use:     "list <id|url>",
		Short:   "Fetch a Listopia list and its books",
		Args:    cobra.ExactArgs(1),
		Example: "  goodread list 1.Best_Books_Ever --books\n  goodread list 1.Best_Books_Ever --page 2",
		RunE: func(cmd *cobra.Command, args []string) error {
			// The whole segment, slug included. A list id without its slug
			// redirects rather than answering, so this one is not narrowed to
			// its numeric prefix the way the others are.
			id := args[0]
			if e, got := goodread.Classify(args[0]); e == "list" && got != "" {
				id = got
			}
			l, err := a.client.GetListRecord(cmd.Context(), id, page)
			if err != nil {
				return mapFetchErr(err)
			}
			a.reportLevels(l.Envelope)
			if a.store != nil {
				_ = a.store.Put("list", l.ID, l.WebURL, l)
			}
			if booksOnly {
				books := l.Books
				if a.limit > 0 && len(books) > a.limit {
					books = books[:a.limit]
				}
				return a.renderOrEmpty(books, len(books))
			}
			return a.renderRecords([]*goodread.List{l}, func() { printList(os.Stdout, l) })
		},
	}
	cmd.Flags().BoolVar(&booksOnly, "books", false, "list the list's books instead of the list header")
	cmd.Flags().IntVar(&page, "page", 1, "which page of the list to read")
	return cmd
}

// genre ─────────────────────────────────────────────────────────────────────

func (a *App) genreCmd() *cobra.Command {
	var booksOnly, relatedOnly bool
	cmd := &cobra.Command{
		Use:     "genre <slug|url>",
		Short:   "Fetch a genre, its featured books and its related genres",
		Args:    cobra.ExactArgs(1),
		Example: "  goodread genre fantasy\n  goodread genre fantasy --related\n  goodread genre fantasy --books",
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := args[0]
			if e, got := goodread.Classify(args[0]); e == "genre" && got != "" {
				slug = got
			}
			g, err := a.client.GetGenreRecord(cmd.Context(), slug)
			if err != nil {
				return mapFetchErr(err)
			}
			a.reportLevels(g.Envelope)
			if a.store != nil {
				_ = a.store.Put("genre", g.Slug, g.WebURL, g)
			}
			switch {
			case relatedOnly:
				rel := g.Related
				if a.limit > 0 && len(rel) > a.limit {
					rel = rel[:a.limit]
				}
				return a.renderOrEmpty(rel, len(rel))
			case booksOnly:
				return a.renderCards(g.Books)
			}
			return a.renderRecords([]*goodread.Genre{g}, func() { printGenre(os.Stdout, g) })
		},
	}
	cmd.Flags().BoolVar(&booksOnly, "books", false, "list the featured books instead of the genre")
	cmd.Flags().BoolVar(&relatedOnly, "related", false, "list the related genres, which is the graph this page alone publishes")
	return cmd
}

// editions ──────────────────────────────────────────────────────────────────

func (a *App) editionsCmd() *cobra.Command {
	var page, maxPages int
	cmd := &cobra.Command{
		Use:   "editions <work-id|url>",
		Short: "Fetch the editions of a work",
		Args:  cobra.ExactArgs(1),
		Example: "  goodread editions 2792775\n" +
			"  goodread editions 2792775 --pages 3\n" +
			"  goodread editions https://www.goodreads.com/work/editions/2792775",
		RunE: func(cmd *cobra.Command, args []string) error {
			// A work id and not a book id. The two look identical and the wrong
			// one 404s, so the mistake is named rather than passed through as a
			// bare not found.
			id := args[0]
			if _, got := goodread.Classify(args[0]); got != "" {
				id = got
			}
			ed, err := a.editionPages(cmd.Context(), id, page, maxPages)
			if err != nil {
				return err
			}
			a.reportLevels(ed.Envelope)
			if a.store != nil {
				_ = a.store.Put("editions", ed.Work.ID, ed.WebURL, ed)
			}
			return a.renderRecords([]*goodread.Editions{ed}, func() { printEditions(os.Stdout, ed) })
		},
	}
	cmd.Flags().IntVar(&page, "page", 1, "which page of editions to read")
	cmd.Flags().IntVar(&maxPages, "pages", 1, "how many pages to read from --page onward")
	return cmd
}

// editionPages reads one page or several and folds them into one record.
//
// The folded record keeps the first page's totals and its own count of what was
// actually read, so Complete stays honest whether one page was read or twenty.
func (a *App) editionPages(ctx context.Context, id string, page, maxPages int) (*goodread.Editions, error) {
	if page < 1 {
		page = 1
	}
	if maxPages < 1 {
		maxPages = 1
	}
	var out *goodread.Editions
	for i := 0; i < maxPages; i++ {
		a.verbosef(1, "reading editions page %d of work %s", page+i, id)
		ed, err := a.client.GetEditionsRecord(ctx, id, page+i)
		if err != nil {
			if out == nil {
				return nil, mapFetchErr(err)
			}
			a.progressf("editions page %d: %v", page+i, err)
			break
		}
		if out == nil {
			out = ed
		} else {
			out.Editions = append(out.Editions, ed.Editions...)
		}
		if len(ed.Editions) == 0 {
			break
		}
		if a.limit > 0 && len(out.Editions) >= a.limit {
			break
		}
	}
	if out == nil {
		return nil, codeError(exitNotFound, nil)
	}
	if a.limit > 0 && len(out.Editions) > a.limit {
		out.Editions = out.Editions[:a.limit]
	}
	out.Complete = out.TotalCount != nil && int64(len(out.Editions)) >= *out.TotalCount
	return out, nil
}

// quotes ────────────────────────────────────────────────────────────────────

func (a *App) quotesCmd() *cobra.Command {
	var page, maxPages int
	var byAuthor bool
	cmd := &cobra.Command{
		Use:     "quotes <work-id|author-id|url>",
		Short:   "Fetch the quotes attached to a work or an author",
		Aliases: []string{"quote"},
		Args:    cobra.ExactArgs(1),
		Example: "  goodread quotes 2792775\n" +
			"  goodread quotes 153394 --author\n" +
			"  goodread quotes 2792775 --pages 3 --limit 50",
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			// A URL says which subject it is and a bare id cannot, so the flag
			// is only consulted when the argument did not already answer.
			if e, got := goodread.Classify(args[0]); got != "" {
				id = got
				if e == "author" {
					byAuthor = true
				}
			}
			if strings.Contains(args[0], "/author/quotes/") {
				byAuthor = true
			}
			q, err := a.quotePages(cmd.Context(), id, byAuthor, page, maxPages)
			if err != nil {
				return err
			}
			a.reportLevels(q.Envelope)
			if a.store != nil {
				_ = a.store.Put("quotes", q.Subject.ID, q.WebURL, q)
			}
			return a.renderRecords([]*goodread.Quotes{q}, func() { printQuotes(os.Stdout, q) })
		},
	}
	cmd.Flags().BoolVar(&byAuthor, "author", false, "read the author's quotes page instead of a work's")
	cmd.Flags().IntVar(&page, "page", 1, "which page of quotes to read")
	cmd.Flags().IntVar(&maxPages, "pages", 1, "how many pages to read from --page onward")
	return cmd
}

func (a *App) quotePages(ctx context.Context, id string, byAuthor bool, page, maxPages int) (*goodread.Quotes, error) {
	if page < 1 {
		page = 1
	}
	if maxPages < 1 {
		maxPages = 1
	}
	var out *goodread.Quotes
	for i := 0; i < maxPages; i++ {
		a.verbosef(1, "reading quotes page %d of %s", page+i, id)
		q, err := a.client.GetQuotesRecord(ctx, id, byAuthor, page+i)
		if err != nil {
			if out == nil {
				return nil, mapFetchErr(err)
			}
			a.progressf("quotes page %d: %v", page+i, err)
			break
		}
		if out == nil {
			out = q
		} else {
			out.Quotes = append(out.Quotes, q.Quotes...)
		}
		if len(q.Quotes) == 0 {
			break
		}
		if a.limit > 0 && len(out.Quotes) >= a.limit {
			break
		}
	}
	if out == nil {
		return nil, codeError(exitNotFound, nil)
	}
	if a.limit > 0 && len(out.Quotes) > a.limit {
		out.Quotes = out.Quotes[:a.limit]
	}
	out.Complete = out.TotalCount != nil && int64(len(out.Quotes)) >= *out.TotalCount
	return out, nil
}

// shared ────────────────────────────────────────────────────────────────────

// renderRecords sends a record to json whole and to a printer otherwise.
//
// The flat formats fall through to the renderer, which flattens the record's
// top level and drops the nested rows. That is the right answer for csv: a
// caller who wants the rows asks for them with --books.
func (a *App) renderRecords(records any, table func()) error {
	if Format(a.format) == FormatTable {
		table()
		return nil
	}
	return a.render(records)
}

// renderCards is the rows view, with --limit applied.
func (a *App) renderCards(cards []goodread.BookCard) error {
	if a.limit > 0 && len(cards) > a.limit {
		cards = cards[:a.limit]
	}
	return a.renderOrEmpty(cards, len(cards))
}

// reportLevels prints the extraction ladder under -vv.
//
// The one place a caller can see that a field came off a selector rather than
// out of the page's own JSON, which is the difference between a fact that will
// survive the next redesign and one that will not.
func (a *App) reportLevels(env goodread.Envelope) {
	if a.verbose < 2 {
		return
	}
	for f, lvl := range env.Level {
		a.verbosef(2, "  %s via %s level %d", f, env.Via[f], lvl)
	}
	for _, m := range env.Missed {
		a.verbosef(2, "  not read: %s", m)
	}
}
