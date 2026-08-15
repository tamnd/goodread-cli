package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/tamnd/goodread-cli/goodread"
)

// The table printers for the six Rails surfaces.
//
// Same reason printBook exists. The generic renderer flattens a record into
// columns, and these records are mostly nested: an author with ten books, a
// list with a hundred ranked rows, a genre with twenty related genres. Flattened
// they come out as one row with slices printed as Go values, which is not
// something a person reads.

func printAuthor(w io.Writer, au *goodread.Author) {
	fmt.Fprintf(w, "%s\n\n", au.Name)

	line(w, "id", fmt.Sprintf("%d", au.LegacyID))
	if au.BornAt != "" {
		born := au.BornAt
		if au.BornIn != "" {
			born += " in " + au.BornIn
		}
		line(w, "born", born)
	}
	line(w, "died", au.DiedAt)
	line(w, "website", au.Website)
	if au.Twitter != "" {
		line(w, "twitter", "@"+strings.TrimPrefix(au.Twitter, "@"))
	}
	if len(au.Genres) > 0 {
		var names []string
		for _, g := range au.Genres {
			names = append(names, g.Name)
		}
		line(w, "genres", strings.Join(names, ", "))
	}
	if len(au.Influences) > 0 {
		var names []string
		for _, r := range au.Influences {
			names = append(names, r.Label())
		}
		line(w, "influences", strings.Join(names, ", "))
	}
	if au.FollowersCount != nil {
		line(w, "followers", comma(*au.FollowersCount))
	}
	line(w, "url", au.WebURL)

	// Said as "over their books" rather than left as a bare average, because an
	// author's 4.21 is a mean over everything they wrote and reads exactly like
	// a book's 4.21 when it is printed without the population next to it.
	if au.Stats != nil && au.Stats.AverageRating != nil {
		fmt.Fprintln(w)
		count := int64(0)
		if au.Stats.RatingsCount != nil {
			count = *au.Stats.RatingsCount
		}
		fmt.Fprintf(w, "%.2f over their books, from %s ratings\n", *au.Stats.AverageRating, comma(count))
	}

	if au.Bio != "" {
		fmt.Fprintln(w)
		fmt.Fprintln(w, wrap(au.Bio, 78))
	}

	if len(au.Books) > 0 {
		fmt.Fprintln(w)
		if au.Works != nil && au.Works.TotalCount != nil {
			fmt.Fprintf(w, "books (%d of %s)\n", len(au.Books), comma(*au.Works.TotalCount))
		} else {
			fmt.Fprintf(w, "books (%d)\n", len(au.Books))
		}
		printCards(w, au.Books)
	}

	printNotRead(w, au.Missed)
}

func printSeries(w io.Writer, s *goodread.Series) {
	fmt.Fprintf(w, "%s\n\n", s.Title)

	line(w, "id", fmt.Sprintf("%d", s.LegacyID))
	// Both counts, always, and never folded into one. A series of 7 primary
	// works and 9 total works is not a series of 9 books, and the difference is
	// companion volumes that nobody counts when they say how long it is.
	if s.PrimaryWorkCount != nil {
		line(w, "primary works", comma(*s.PrimaryWorkCount))
	}
	if s.TotalWorkCount != nil {
		line(w, "total works", comma(*s.TotalWorkCount))
	}
	line(w, "url", s.WebURL)

	if s.Description != "" {
		fmt.Fprintln(w)
		fmt.Fprintln(w, wrap(s.Description, 78))
	}

	if len(s.Books) > 0 {
		fmt.Fprintln(w)
		for _, b := range s.Books {
			pos := b.Position
			if pos == "" {
				pos = "-"
			}
			fmt.Fprintf(w, "  %-10s %s%s\n", pos, b.Title, ratingSuffix(b.AverageRating, b.RatingsCount))
		}
	}

	printNotRead(w, s.Missed)
}

func printList(w io.Writer, l *goodread.List) {
	fmt.Fprintf(w, "%s\n\n", l.Title)

	line(w, "id", l.ID)
	if l.BooksCount != nil {
		line(w, "books", comma(*l.BooksCount))
	}
	if l.VotersCount != nil {
		line(w, "voters", comma(*l.VotersCount))
	}
	if l.Page > 0 {
		line(w, "page", fmt.Sprintf("%d", l.Page))
	}
	line(w, "url", l.WebURL)

	if l.Description != "" {
		fmt.Fprintln(w)
		fmt.Fprintln(w, wrap(l.Description, 78))
	}

	if len(l.Books) > 0 {
		fmt.Fprintln(w)
		for _, b := range l.Books {
			// Score and votes both, because they are two measurements. A book
			// can be third by score and first by votes, and printing one of them
			// hides which ranking you are looking at.
			var tail string
			if b.Score != nil && b.Votes != nil {
				tail = fmt.Sprintf("  (score %s, %s votes)", comma(*b.Score), comma(*b.Votes))
			}
			fmt.Fprintf(w, "  %3d. %s%s\n", b.Rank, b.Title, tail)
		}
	}

	printNotRead(w, l.Missed)
}

func printGenre(w io.Writer, g *goodread.Genre) {
	fmt.Fprintf(w, "%s\n\n", g.Name)

	line(w, "slug", g.Slug)
	line(w, "url", g.WebURL)

	if g.Description != "" {
		fmt.Fprintln(w)
		fmt.Fprintln(w, wrap(g.Description, 78))
	}

	if len(g.Related) > 0 {
		var names []string
		for _, r := range g.Related {
			names = append(names, r.Label())
		}
		fmt.Fprintln(w)
		fmt.Fprintln(w, "related")
		fmt.Fprintln(w, wrap(strings.Join(names, ", "), 78))
	}

	if len(g.Books) > 0 {
		fmt.Fprintf(w, "\nfeatured books (%d)\n", len(g.Books))
		printCards(w, g.Books)
	}

	printNotRead(w, g.Missed)
}

func printEditions(w io.Writer, ed *goodread.Editions) {
	fmt.Fprintf(w, "%s\n\n", ed.Title)

	line(w, "work", ed.Work.ID)
	if ed.TotalCount != nil {
		line(w, "editions", comma(*ed.TotalCount))
	}
	if ed.Page > 0 {
		line(w, "page", fmt.Sprintf("%d", ed.Page))
	}
	line(w, "url", ed.WebURL)
	fmt.Fprintln(w)

	for _, e := range ed.Editions {
		fmt.Fprintf(w, "  %s\n", e.Title)
		var bits []string
		if e.Name != "" {
			bits = append(bits, e.Name)
		}
		if e.Format != "" {
			bits = append(bits, e.Format)
		}
		if e.NumPages != nil {
			bits = append(bits, fmt.Sprintf("%d pages", *e.NumPages))
		}
		if e.Language != "" {
			bits = append(bits, e.Language)
		}
		if e.PublishedAt != "" {
			pub := e.PublishedAt
			if e.Publisher != "" {
				pub += ", " + e.Publisher
			}
			bits = append(bits, pub)
		}
		if len(bits) > 0 {
			fmt.Fprintf(w, "    %s\n", strings.Join(bits, " · "))
		}
		// The ISBN is the reason anybody reads this page, so it gets its own
		// line rather than being appended to a run of five other facts.
		var ids []string
		if e.ISBN13 != "" {
			ids = append(ids, "isbn13 "+e.ISBN13)
		}
		if e.ISBN != "" {
			ids = append(ids, "isbn "+e.ISBN)
		}
		if e.ASIN != "" {
			ids = append(ids, "asin "+e.ASIN)
		}
		if len(ids) > 0 {
			fmt.Fprintf(w, "    %s\n", strings.Join(ids, "  "))
		}
		fmt.Fprintf(w, "    %s\n", goodread.BookURL(e.Book.ID))
	}

	printNotRead(w, ed.Missed)
}

func printQuotes(w io.Writer, q *goodread.Quotes) {
	fmt.Fprintf(w, "quotes of %s\n\n", q.Subject.Label())

	line(w, "subject", q.Subject.Type+" "+q.Subject.ID)
	if q.TotalCount != nil {
		line(w, "quotes", comma(*q.TotalCount))
	}
	if q.Page > 0 {
		line(w, "page", fmt.Sprintf("%d", q.Page))
	}
	line(w, "url", q.WebURL)

	for _, qq := range q.Quotes {
		fmt.Fprintln(w)
		fmt.Fprintln(w, wrap(qq.Text, 78))
		var by []string
		if qq.Author != nil {
			by = append(by, qq.Author.Label())
		}
		if qq.Book != nil && qq.Book.Title != "" {
			by = append(by, qq.Book.Title)
		}
		if qq.LikeCount != nil {
			by = append(by, comma(*qq.LikeCount)+" likes")
		}
		if len(by) > 0 {
			fmt.Fprintf(w, "  %s\n", strings.Join(by, ", "))
		}
	}

	printNotRead(w, q.Missed)
}

// printCards is the shared rows block for author and genre.
func printCards(w io.Writer, cards []goodread.BookCard) {
	for _, c := range cards {
		var by string
		if len(c.Contributors) > 0 {
			by = " by " + c.Contributors[0].Name
		}
		fmt.Fprintf(w, "  %s%s%s\n", c.Title, by, ratingSuffix(c.AverageRating, c.RatingsCount))
	}
}

// ratingSuffix formats the two numbers a listing row carries, or nothing.
//
// Nothing, and not "0.00 (0)", because a row that did not publish a rating and
// a book that nobody has rated are different states and 00_overview.md is
// explicit that the record must not conflate them. The same rule holds where a
// person is reading it.
func ratingSuffix(avg *float64, count *int64) string {
	if avg == nil {
		return ""
	}
	if count == nil {
		return fmt.Sprintf("  %.2f", *avg)
	}
	return fmt.Sprintf("  %.2f (%s)", *avg, comma(*count))
}

func printShelf(w io.Writer, s *goodread.ShelfRecord) {
	fmt.Fprintf(w, "%s, %s\n\n", s.User.Label(), s.Name)

	line(w, "id", s.ID)
	// Named every time. The rss feed and the html shelf carry different fields,
	// so a row with no isbn means "the feed had none" on one route and "the page
	// never prints it" on the other.
	line(w, "read over", shelfRoute(s.Source == "html"))
	if s.TotalCount != nil {
		line(w, "books", comma(*s.TotalCount))
	}
	line(w, "loaded", fmt.Sprintf("%d", s.Loaded))
	line(w, "url", s.WebURL)
	fmt.Fprintln(w)

	for _, b := range s.Books {
		var by string
		if len(b.Contributors) > 0 {
			by = " by " + b.Contributors[0].Name
		}
		fmt.Fprintf(w, "  %s%s\n", b.Title, by)
		var bits []string
		if b.Rating != nil {
			// Theirs, and said so. An average of 4.35 and one person's 3 are
			// both "the rating" until somebody labels them.
			bits = append(bits, fmt.Sprintf("rated %d by them", *b.Rating))
		}
		if b.AverageRating != nil {
			bits = append(bits, fmt.Sprintf("%.2f average", *b.AverageRating))
		}
		if b.DateRead != nil {
			bits = append(bits, "read "+b.DateRead.Format("2006-01-02"))
		} else if b.DateAdded != nil {
			bits = append(bits, "added "+b.DateAdded.Format("2006-01-02"))
		}
		if len(b.Shelves) > 0 {
			bits = append(bits, strings.Join(b.Shelves, ", "))
		}
		if len(bits) > 0 {
			fmt.Fprintf(w, "    %s\n", strings.Join(bits, " · "))
		}
	}

	printNotRead(w, s.Missed)
}
