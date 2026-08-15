package goodread

import (
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// The row shapes the Rails surfaces have in common.
//
// Four of the six render a book row and two of them render a quote block, and
// in both cases it is the same partial behind them: the author page and the
// Listopia page emit byte for byte the same tr, and the quotes page and the
// genre page emit byte for byte the same div.quote. Reading each of them twice
// would mean fixing each of them twice the next time the markup moves, so they
// are read once here and the caller passes its own surface id in.

var (
	reMiniRating   = regexp.MustCompile(`([\d.]+)\s+avg rating`)
	reMiniRatings  = regexp.MustCompile(`([\d,]+)\s+ratings?`)
	reMiniEditions = regexp.MustCompile(`([\d,]+)\s+editions?`)
	rePublished    = regexp.MustCompile(`published\s+(-?\d{1,4})`)
	reShowingOf    = regexp.MustCompile(`Showing\s+[\d,]+\s*-\s*[\d,]+\s+of\s+([\d,]+)`)
)

// schemaBookRows reads every schema.org Book row under a selection.
//
// The rows are findable by microdata even though the numbers inside them are
// not, which is the whole reason this is worth doing with a selector at all.
// The minirating string is the only place the average and the ratings count
// appear, and the editions link is the only place the work id does.
func schemaBookRows(sel *goquery.Selection, via string) []BookCard {
	var out []BookCard
	sel.Find("tr[itemtype$='schema.org/Book']").Each(func(_ int, row *goquery.Selection) {
		link := row.Find("a.bookTitle").First()
		href, _ := link.Attr("href")
		id := numericPrefix(extractIDFromPath(href, "/book/show/"))
		if id == "" {
			return
		}
		title := strings.TrimSpace(link.Find("span[itemprop='name']").First().Text())
		if title == "" {
			title = strings.TrimSpace(link.Text())
		}
		c := BookCard{
			Book:      Ref{Type: "Book", ID: id, Key: "Book:" + id, Title: TitleWithoutSeries(title), Resolved: true},
			Title:     title,
			TitleBare: TitleWithoutSeries(title),
			ImageURL:  attr(row.Find("img.bookCover").First(), "src"),
			Via:       via,
		}
		c.Contributors = contributorRows(row, via)

		mini := row.Find("span.minirating").First().Text()
		if m := reMiniRating.FindStringSubmatch(mini); m != nil {
			if v := parseFloat(m[1]); v > 0 {
				c.AverageRating = &v
			}
		}
		if m := reMiniRatings.FindStringSubmatch(mini); m != nil {
			if v := commaFreeInt(m[1]); v > 0 {
				c.RatingsCount = &v
			}
		}

		meta := row.Find("span.greyText.smallText.uitext").First()
		if m := rePublished.FindStringSubmatch(meta.Text()); m != nil {
			c.PublishedAt = m[1]
		}
		ed := meta.Find("a[href*='/work/editions/']").First()
		if href, ok := ed.Attr("href"); ok {
			c.EditionsURL = absURL(href)
			if wid := numericPrefix(extractIDFromPath(href, "/work/editions/")); wid != "" {
				c.Work = &Ref{Type: "Work", ID: wid, Key: "Work:" + wid, Resolved: true}
			}
		}
		if m := reMiniEditions.FindStringSubmatch(ed.Text()); m != nil {
			if v := commaFreeInt(m[1]); v > 0 {
				c.EditionsCount = &v
			}
		}
		out = append(out, c)
	})
	return out
}

// contributorRows reads the authorName__container blocks under a selection.
//
// The role in the span is the reason contributors are not a list of names. An
// illustrator filed as an author is a wrong fact with nothing downstream that
// could notice, and the page does say which is which.
func contributorRows(sel *goquery.Selection, via string) []Contributor {
	var out []Contributor
	sel.Find("div.authorName__container").Each(func(_ int, a *goquery.Selection) {
		link := a.Find("a.authorName").First()
		name := strings.TrimSpace(link.Find("span[itemprop='name']").First().Text())
		if name == "" {
			name = strings.TrimSpace(link.Text())
		}
		if name == "" {
			return
		}
		href, _ := link.Attr("href")
		role := strings.Trim(strings.TrimSpace(a.Find("span.role").First().Text()), "() ")
		if role == "" {
			role = "Author"
		}
		id := numericPrefix(extractIDFromPath(href, "/author/show/"))
		out = append(out, Contributor{
			ID: id, LegacyID: commaFreeInt(id), Name: name, Role: role, WebURL: absURL(href), Via: via,
		})
	})
	return out
}

var reQuoteLikes = regexp.MustCompile(`([\d,]+)\s+likes?`)

// quoteBlocks reads every div.quote under a selection.
//
// The text is taken off quoteText with the two child spans removed rather than
// off the whole block, because the block also holds the attribution line and
// the tag list and taking its text would fold all three into the quote.
func quoteBlocks(sel *goquery.Selection, via string) []Quote {
	var out []Quote
	sel.Find("div.quote").Each(func(_ int, q *goquery.Selection) {
		body := q.Find("div.quoteText").First()
		if body.Length() == 0 {
			return
		}
		text := quoteTextOf(body)
		if text == "" {
			return
		}
		quote := Quote{Text: text, Via: via}

		// The permalink is the only id a quote has. Its slug is the first words
		// of the text, so it is not a stable key across an edit, but there is
		// nothing else and a quote with no id cannot be deduplicated at all.
		if href, ok := q.Find("a[href*='/quotes/']").Last().Attr("href"); ok {
			if id := extractIDFromPath(href, "/quotes/"); id != "" {
				quote.ID = numericPrefix(id)
				quote.WebURL = absURL(href)
			}
		}
		if m := reQuoteLikes.FindStringSubmatch(q.Find("div.quoteFooter a.smallText").First().Text()); m != nil {
			if v := commaFreeInt(m[1]); v > 0 {
				quote.LikeCount = &v
			}
		}
		// The attribution renders as "Name, Title" with the name in a bare span
		// and the title in a link, so the link is what separates them.
		body.Find("span.authorOrTitle").Each(func(_ int, s *goquery.Selection) {
			name := strings.Trim(strings.TrimSpace(s.Text()), ", ")
			if name != "" && quote.Author == nil {
				quote.Author = &Ref{Type: "Author", Title: name}
			}
		})
		if a := body.Find("a.authorOrTitle").First(); a.Length() > 0 {
			title := strings.TrimSpace(a.Text())
			href, _ := a.Attr("href")
			if wid := numericPrefix(extractIDFromPath(href, "/work/quotes/")); wid != "" {
				quote.Book = &Ref{Type: "Work", ID: wid, Key: "Work:" + wid, Title: title, Resolved: true}
			} else if title != "" {
				quote.Book = &Ref{Type: "Work", Title: title}
			}
		}
		// The avatar link is the only place the author id appears, and only on
		// the pages that render one.
		if href, ok := q.Find("a.quoteAvatar").First().Attr("href"); ok {
			if id := numericPrefix(extractIDFromPath(href, "/author/show/")); id != "" {
				if quote.Author == nil {
					quote.Author = &Ref{Type: "Author"}
				}
				quote.Author.ID, quote.Author.Key, quote.Author.Resolved = id, "Author:"+id, true
			}
		}
		q.Find("div.quoteFooter a[href*='/quotes/tag/']").Each(func(_ int, t *goquery.Selection) {
			if tag := strings.TrimSpace(t.Text()); tag != "" {
				quote.Tags = append(quote.Tags, tag)
			}
		})
		out = append(out, quote)
	})
	return out
}

// quoteTextOf takes the quote and leaves the attribution behind.
//
// Long quotes are rendered twice, a truncated freeTextContainer and a hidden
// full freeText, so the hidden one wins where it exists for the same reason it
// wins in a biography. Short quotes have neither and are bare text in the div.
func quoteTextOf(body *goquery.Selection) string {
	clone := body.Clone()
	clone.Find("span.authorOrTitle, a.authorOrTitle, span[id^='quote_book_link']").Remove()
	if full := clone.Find("span[id^='freeText']:not([id^='freeTextContainer'])").First(); full.Length() > 0 {
		return trimQuoteMarks(full.Text())
	}
	if short := clone.Find("span[id^='freeTextContainer']").First(); short.Length() > 0 {
		return trimQuoteMarks(short.Text())
	}
	return trimQuoteMarks(clone.Text())
}

// trimQuoteMarks strips the curly quotes the template wraps every quote in, and
// the dash that starts the attribution line when one is left over.
func trimQuoteMarks(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "“")
	if i := strings.LastIndex(s, "”"); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "...more")
	return strings.Trim(strings.TrimSpace(s), "― \n\t")
}

// showingOf reads the total out of "Showing 1-30 of 1,226".
//
// The one sentence on these pages that says how much was not returned. Without
// it a page of thirty quotes looks like all the quotes there are.
func showingOf(doc *goquery.Document) int64 {
	m := reShowingOf.FindStringSubmatch(doc.Text())
	if m == nil {
		return 0
	}
	return commaFreeInt(m[1])
}
