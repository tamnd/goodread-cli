package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/tamnd/goodread-cli/goodread"
)

// printBook writes a book record for a person to read.
//
// The generic table renderer flattens a record into columns, which is right for
// a list of books and wrong for one book: it would print the ratings
// distribution as [134095 242700 ...] and drop the not read block entirely.
func printBook(w io.Writer, b *goodread.Book, wk *goodread.Work) {
	fmt.Fprintf(w, "%s\n", title(b))
	if by := byline(b); by != "" {
		fmt.Fprintf(w, "%s\n", by)
	}
	fmt.Fprintln(w)

	line(w, "id", fmt.Sprintf("%d", b.LegacyID))
	if b.Work != nil && wk != nil && wk.LegacyID != 0 {
		line(w, "work", fmt.Sprintf("%d", wk.LegacyID))
	}
	if b.Series != nil {
		var parts []string
		for _, s := range b.Series {
			if s.Position != "" {
				parts = append(parts, s.Series.Label()+" #"+s.Position)
			} else {
				parts = append(parts, s.Series.Label())
			}
		}
		line(w, "series", strings.Join(parts, ", "))
	}
	if b.PublicationTime != nil {
		line(w, "published", b.PublicationTime.Format("2006-01-02"))
	}
	line(w, "publisher", b.Publisher)
	line(w, "format", b.Format)
	if b.NumPages != nil {
		line(w, "pages", fmt.Sprintf("%d", *b.NumPages))
	}
	line(w, "language", b.Language)
	line(w, "isbn13", b.ISBN13)
	if len(b.Genres) > 0 {
		var names []string
		for _, g := range b.Genres {
			names = append(names, g.Name)
		}
		line(w, "genres", strings.Join(names, ", "))
	}
	line(w, "url", b.WebURL)

	if b.Stats != nil {
		fmt.Fprintln(w)
		printHistogram(w, b.Stats)
	}

	if b.DescriptionStripped != "" {
		fmt.Fprintln(w)
		fmt.Fprintln(w, wrap(b.DescriptionStripped, 78))
	}

	printNotRead(w, b.Missed)
}

// printHistogram draws the ratings distribution.
//
// This is the field v0.2.0 could not get at all, because the page draws it as a
// bar chart and never writes the numbers out. Printing it is the whole visible
// payoff of reading the Apollo cache: a 4.35 from a flat spread is a different
// book from a 4.35 with a spike at five, and the average alone cannot say which
// one you are looking at.
func printHistogram(w io.Writer, s *goodread.Stats) {
	if s.AverageRating != nil {
		count := int64(0)
		if s.RatingsCount != nil {
			count = *s.RatingsCount
		}
		fmt.Fprintf(w, "%.2f average from %s ratings", *s.AverageRating, comma(count))
		if s.TextReviewsCount != nil {
			fmt.Fprintf(w, ", %s text reviews", comma(*s.TextReviewsCount))
		}
		fmt.Fprintln(w)
	}
	if len(s.RatingsCountDist) == 0 {
		return
	}

	var max, total int64
	for _, n := range s.RatingsCountDist {
		if n > max {
			max = n
		}
		total += n
	}
	if max == 0 {
		return
	}
	// Five stars first, because that is the order the page draws them and the
	// order a person expects to read them. The slice itself stays one through
	// five, which is what every test and every consumer relies on.
	for i := len(s.RatingsCountDist) - 1; i >= 0; i-- {
		n := s.RatingsCountDist[i]
		bars := int(n * 40 / max)
		pct := float64(n) * 100 / float64(total)
		fmt.Fprintf(w, "%d star  %-40s %9s  %5.1f%%\n",
			i+1, strings.Repeat("█", bars), comma(n), pct)
	}
}

// printNotRead is the block that makes the difference between a tool you can
// trust and one that quietly gives you a subset.
//
// Printed rather than hidden, and every line names the command that would get
// the thing it is about. A tool that reads a sample and does not say so is
// telling you something false by omission.
func printNotRead(w io.Writer, missed []string) {
	if len(missed) == 0 {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "not read:")
	for _, m := range missed {
		fmt.Fprintf(w, "  %s\n", m)
	}
}

func line(w io.Writer, k, v string) {
	if v == "" {
		return
	}
	fmt.Fprintf(w, "%-11s %s\n", k, v)
}

// title prefers the decorated form, since "(The Hunger Games, #1)" is
// information a person wants and a machine does not.
func title(b *goodread.Book) string {
	if b.TitleComplete != "" {
		return b.TitleComplete
	}
	return b.Title
}

func byline(b *goodread.Book) string {
	var parts []string
	for _, c := range b.Contributors {
		if c.Role != "" && !strings.EqualFold(c.Role, "Author") {
			parts = append(parts, fmt.Sprintf("%s (%s)", c.Name, strings.ToLower(c.Role)))
			continue
		}
		parts = append(parts, c.Name)
	}
	if len(parts) == 0 {
		return ""
	}
	return "by " + strings.Join(parts, ", ")
}

// comma groups thousands, because 10232418 is not a number anybody reads.
func comma(n int64) string {
	s := fmt.Sprintf("%d", n)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}

// wrap breaks a paragraph at a width, on spaces only.
func wrap(s string, width int) string {
	var out strings.Builder
	for i, para := range strings.Split(s, "\n") {
		if i > 0 {
			out.WriteString("\n")
		}
		col := 0
		for j, word := range strings.Fields(para) {
			if j > 0 {
				if col+1+len(word) > width {
					out.WriteString("\n")
					col = 0
				} else {
					out.WriteString(" ")
					col++
				}
			}
			out.WriteString(word)
			col += len(word)
		}
	}
	return out.String()
}
