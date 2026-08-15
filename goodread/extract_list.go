package goodread

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// ExtractList reads a Listopia page.
//
// The one surface that carries a fact nothing else on the site has: a ranking
// with the votes behind it. Every other page ranks by rating or by popularity,
// which are both derived from the same numbers a book page already gives you.
// A Listopia rank is people choosing, and the score and the vote count are two
// different measurements of that choosing, so both are kept.
//
// No ld+json, no og:, no React island worth reading. The title comes off a
// twitter: tag at level 2 and everything else is selectors.
func ExtractList(body []byte, pageURL string) (*Extractor, error) {
	e := NewExtractor("s4")

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return e, fmt.Errorf("parse list page: %w", err)
	}

	// twitter:title renders "Best Books Ever (79111 books)", so it carries both
	// the name and the size. It is a meta tag rather than a selector, which is
	// why it is level 2 even though it needs unpicking.
	meta := parseMetaNames(body)
	title, count := parseListTwitterTitle(meta["twitter:title"])
	e.set("title", LevelMeta, title)
	if count > 0 {
		e.set("books_count", LevelMeta, count)
	}
	// Deliberately not description. The meta description on this surface is a
	// generated blurb naming the first few books, "79,111 books based on 295014
	// votes: The Hunger Games by...", and not one word of it is the description
	// the list's owner wrote. Taking it would fill the field with something that
	// reads plausibly and is not the thing the field means.

	// The header renders "Listopia > Best Books Ever" and the crumb comes off.
	head := strings.TrimSpace(doc.Find("div.pageHeader h2").First().Text())
	if i := strings.LastIndex(head, ">"); i >= 0 {
		head = strings.TrimSpace(head[i+1:])
	}
	e.set("title", LevelSelector, head)

	// The description div also holds the flag link, which is chrome and not
	// description, so it comes out before the text is taken.
	desc := doc.Find("div.mediumText.u-paddingBottomMedium").First().Clone()
	desc.Find("a.flag, div.right").Remove()
	e.set("description", LevelSelector, strings.TrimSpace(desc.Text()))

	e.set("books", LevelSelector, listCards(doc))

	// The meta description says "79,111 books based on 295014 votes", which is
	// the only place the vote total for the whole list appears.
	if m := reListVotes.FindStringSubmatch(meta["description"]); m != nil {
		e.set("votes_total", LevelMeta, commaFreeInt(m[1]))
	}
	if n := showingOf(doc); n > 0 {
		e.set("books_count", LevelSelector, n)
	}
	if p := currentPage(doc); p > 0 {
		e.set("page", LevelSelector, p)
	}
	// The whole segment and not its numeric prefix. A list URL is
	// /list/show/1.Best_Books_Ever and the slug is not decoration: the bare
	// /list/show/1 redirects, so the id that round trips is the whole thing.
	if id := pathSegmentAfter(pageURL, "/list/show/"); id != "" {
		e.set("id", LevelSelector, id)
	}

	if len(e.Fields) == 0 {
		return e, fmt.Errorf("no list data on this page")
	}
	return e, nil
}

func init() {
	for _, f := range []SelectorField{
		{"s4", "List", "title", "div.pageHeader h2", "v0.3.0", "the crumb is part of the heading and comes off"},
		{"s4", "List", "description", "div.mediumText.u-paddingBottomMedium", "v0.3.0", "no meta tag carries the full text"},
		{"s4", "List", "rank", "tr td.number", "v0.3.0", "the rank is a bare cell with no class of its own"},
		{"s4", "List", "score", "a[onclick*=score_explanation]", "v0.3.0", "the score is the text of the link that explains scores"},
		{"s4", "List", "votes", "a[id^=loading_link_]", "v0.3.0", "the vote count is the label of an Ajax link"},
		{"s5", "Genre", "related", "h2:contains(Related Genres) ~ div a[href^=/genres/]", "v0.3.0", "the genre graph exists nowhere else on the site"},
		{"s5", "Genre", "books", "div.coverRow div.bookBox", "v0.3.0", "the featured shelf is markup with no data attributes"},
		{"s6", "Editions", "editions", "div.editionData", "v0.3.0", "no microdata on the rows, only dataTitle and dataValue pairs"},
		{"s7", "Quotes", "quotes", "div.quote", "v0.3.0", "the only markup a quote has"},
	} {
		RegisterSelector(f)
	}
}

var (
	reListTwitterTitle = regexp.MustCompile(`^(.*?)\s*\(([\d,]+)\s+books?\)\s*$`)
	reListVotes        = regexp.MustCompile(`based on ([\d,]+) votes`)
	reListScore        = regexp.MustCompile(`score:\s*([\d,]+)`)
	reListPeopleVoted  = regexp.MustCompile(`([\d,]+)\s+people voted`)
)

// parseListTwitterTitle splits "Best Books Ever (79111 books)".
//
// A list actually named something ending in a book count keeps its name,
// because the template would render that one with a second parenthesis.
func parseListTwitterTitle(s string) (title string, count int64) {
	m := reListTwitterTitle.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return strings.TrimSpace(s), 0
	}
	return strings.TrimSpace(m[1]), commaFreeInt(m[2])
}

// listCards reads the ranked rows.
//
// The row itself is the shared schema.org shape, so the rank, the score and the
// vote count are all this adds. All three are Listopia's own and none of them
// exist on the book page the row points at.
func listCards(doc *goquery.Document) []ListCard {
	var out []ListCard
	cards := schemaBookRows(doc.Selection, "s4")
	i := 0
	doc.Find("tr[itemtype$='schema.org/Book']").Each(func(_ int, row *goquery.Selection) {
		if row.Find("a.bookTitle").Length() == 0 {
			// schemaBookRows skips a row with no book link, so this has to skip
			// the same ones or the two sequences drift apart by one and every
			// rank after it belongs to the wrong book.
			return
		}
		if i >= len(cards) {
			return
		}
		c := ListCard{BookCard: cards[i]}
		i++
		if n, err := strconv.Atoi(strings.TrimSpace(row.Find("td.number").First().Text())); err == nil {
			c.Rank = n
		}
		text := row.Text()
		if m := reListScore.FindStringSubmatch(text); m != nil {
			if v := commaFreeInt(m[1]); v > 0 {
				c.Score = &v
			}
		}
		if m := reListPeopleVoted.FindStringSubmatch(text); m != nil {
			if v := commaFreeInt(m[1]); v > 0 {
				c.Votes = &v
			}
		}
		out = append(out, c)
	})
	return out
}

// currentPage reads the paginator's own idea of where it is.
func currentPage(doc *goquery.Document) int {
	n, err := strconv.Atoi(strings.TrimSpace(doc.Find("em.current").First().Text()))
	if err != nil {
		return 0
	}
	return n
}

// ListFrom turns a list extraction into a record.
func ListFrom(e *Extractor, id string, retrievedAt time.Time) (*List, error) {
	if e == nil {
		return nil, fmt.Errorf("no extraction")
	}
	l := &List{Envelope: envelopeOf(e, "list", retrievedAt)}
	l.ID = id
	if v := firstString(e, "id"); v != "" {
		l.ID = v
	}
	l.WebURL = ListURL(l.ID)
	l.Title, _ = e.Fields["title"].(string)
	l.Description, _ = e.Fields["description"].(string)
	if n, ok := int64Of(e.Fields["books_count"]); ok {
		l.BooksCount = &n
	}
	if n, ok := int64Of(e.Fields["votes_total"]); ok {
		l.VotersCount = &n
	}
	if p, ok := e.Fields["page"].(int); ok {
		l.Page = p
	}
	l.Books, _ = e.Fields["books"].([]ListCard)

	if l.BooksCount != nil && int64(len(l.Books)) < *l.BooksCount {
		e.Miss("this page carries %d of %d books on the list. `goodread list %s --page N` reads the rest.",
			len(l.Books), *l.BooksCount, l.ID)
		l.Missed = append(l.Missed, e.Missed[len(e.Missed)-1])
	}
	return l, nil
}
