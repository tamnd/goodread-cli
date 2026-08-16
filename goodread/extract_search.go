package goodread

import (
	"bytes"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// ExtractSearch reads /search.
//
// Disallowed, so nothing gets here without --no-robots, and the record says so
// through the robots note the client stamps on it.
//
// Level 3 throughout, and on this page that is not a shortcut. There is no
// __NEXT_DATA__, no application/ld+json and no og: tag worth reading: the
// title element and the markup are the whole of it. The rows are the one part
// that is structured, since they carry schema.org microdata, which is why they
// are read by the same function that reads the author and Listopia pages.
func ExtractSearch(body []byte, pageURL string) (*Extractor, error) {
	e := NewExtractor("s10")

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return e, fmt.Errorf("parse search page: %w", err)
	}

	// The title line is the only place the page states the range, the total and
	// the kind of thing it counted, all three in one sentence.
	title := strings.TrimSpace(doc.Find("title").First().Text())
	if m := reSearchTitle.FindStringSubmatch(title); m != nil {
		e.set("query", LevelSelector, m[1])
		e.set("showing", LevelSelector, ShowRange{
			From: commaFreeInt(m[2]),
			To:   commaFreeInt(m[3]),
			Of:   commaFreeInt(m[4]),
			Kind: m[5],
		})
		e.set("total_results", LevelSelector, commaFreeInt(m[4]))
	}

	// The sub nav line says the page number, the total again with the site's
	// own hedge on it, and how long the search took.
	sub := strings.TrimSpace(doc.Find("h3.searchSubNavContainer").First().Text())
	if m := reSearchSubNav.FindStringSubmatch(sub); m != nil {
		e.set("page", LevelSelector, int(commaFreeInt(m[1])))
		e.set("total_results", LevelSelector, commaFreeInt(m[3]))
		if m[2] != "" {
			e.set("approximate", LevelSelector, true)
		}
		if v := parseFloat(m[4]); v > 0 {
			e.set("elapsed", LevelSelector, v)
		}
	}

	e.set("tabs", LevelSelector, searchTabs(doc))
	e.set("related_shelves", LevelSelector, relatedShelves(doc))

	// The genre link is the site's own reading of the query, and it appears on
	// no other surface. A query that maps to nothing has no link and no field.
	if a := doc.Find("div.greyText > a.actionLink[href^='/genres/']").First(); a.Length() > 0 {
		href, _ := a.Attr("href")
		if slug := pathSegmentAfter(href, "/genres/"); slug != "" {
			e.set("genre", LevelSelector, Ref{
				Type: "Genre", ID: slug, Key: "Genre:" + slug,
				Title: strings.TrimSpace(a.Text()), Resolved: true,
			})
		}
	}

	if next, ok := doc.Find("a.next_page[href]").First().Attr("href"); ok {
		e.set("next_page", LevelSelector, absURL(next))
		e.set("qid", LevelSelector, paramOf(next, "qid"))
	}
	if last := lastPage(doc); last > 0 {
		e.set("last_page", LevelSelector, last)
	}

	e.set("results", LevelSelector, searchHits(doc))
	e.set("web_url", LevelSelector, pageURL)
	e.set("search_type", LevelSelector, paramOf(pageURL, "search_type"))
	e.set("field", LevelSelector, paramOf(pageURL, "search[field]"))

	if len(e.Fields) == 0 {
		return e, fmt.Errorf("no search data on this page")
	}
	return e, nil
}

func init() {
	for _, f := range []SelectorField{
		{"s10", "Search", "showing", "title", "v0.3.1", "the range, the total and the kind are one sentence in the title and nowhere else"},
		{"s10", "Search", "total_results", "h3.searchSubNavContainer", "v0.3.1", "the only line that says the site hedged the total"},
		{"s10", "Search", "elapsed", "h3.searchSubNavContainer", "v0.3.1", "the site's own search time, in the same line as the total"},
		{"s10", "Search", "tabs", "div.tabs span, div.tabs a", "v0.3.1", "the tabs are spans and anchors with ids and no data attributes"},
		{"s10", "Search", "genre", "div.greyText > a.actionLink", "v0.3.1", "the site's own mapping from query to genre, published nowhere else"},
		{"s10", "Search", "related_shelves", "h2:contains(Related Shelves) ~ div a[href^=/shelf/show/]", "v0.3.1", "the shelf counts are bare text next to each link"},
		{"s10", "Search", "results", "tr[itemtype$=schema.org/Book]", "v0.3.1", "microdata marks the rows, and nothing marks the numbers inside them"},
		{"s10", "Search", "last_page", "div.pagination a", "v0.3.1", "the last page is the largest number in the pager"},
	} {
		RegisterSelector(f)
	}
}

var (
	// Search results for "the hunger games" (showing 1-20 of 816 books)
	reSearchTitle = regexp.MustCompile(`Search results for "(.*)"\s*\(showing\s+([\d,]+)-([\d,]+)\s+of\s+([\d,]+)\s+(\w+)\)`)

	// Page 1 of about 816 results (0.08 seconds)
	reSearchSubNav = regexp.MustCompile(`Page\s+([\d,]+)\s+of\s+(about\s+)?([\d,]+)\s+results?(?:\s+\(([\d.]+)\s+seconds?\))?`)

	reShelfCount = regexp.MustCompile(`\(([\d,]+)\)`)
	rePageNumber = regexp.MustCompile(`^\d+$`)
)

// searchHits reads the result rows.
//
// schemaBookRows does the reading, because a search row and an author page row
// are the same Rails partial down to the byte, and reading the same markup in
// two places is how the two readings drift apart. What is added on top is the
// part that is only true of a search: the rank, the query session it belongs
// to, and the tracked link the page actually wrote.
func searchHits(doc *goquery.Document) []SearchHit {
	cards := schemaBookRows(doc.Selection, "s10")
	rows := doc.Find("tr[itemtype$='schema.org/Book']")
	out := make([]SearchHit, 0, len(cards))
	for i, c := range cards {
		h := SearchHit{BookCard: c, Rank: i + 1}
		if i < rows.Length() {
			href, _ := rows.Eq(i).Find("a.bookTitle").First().Attr("href")
			if href != "" {
				h.TrackedURL = absURL(href)
				h.WebURL = untrack(absURL(href))
				if r := commaFreeInt(paramOf(href, "rank")); r > 0 {
					h.Rank = int(r)
				}
			}
		}
		h.Series = seriesFromTitle(c.Title)
		out = append(out, h)
	}
	return out
}

// searchTabs reads the tab strip.
//
// The selected tab is a span with no href, since the page does not link to
// itself, so its URL is absent and Selected is what says which one it was. The
// readable verdict comes from what this tool measured rather than from the
// page, because the page offers all five and three of them answer with a
// challenge.
func searchTabs(doc *goquery.Document) []SearchTab {
	var out []SearchTab
	doc.Find("div.tabs span[id$='Link'], div.tabs a[id$='Link']").Each(func(_ int, s *goquery.Selection) {
		id, _ := s.Attr("id")
		typ := strings.TrimSuffix(id, "Link")
		if typ == "" {
			return
		}
		label, readable, note := SearchTabInfo(typ)
		if text := strings.TrimSpace(s.Text()); text != "" {
			label = text
		}
		tab := SearchTab{Type: typ, Label: label, Readable: readable, Note: note}
		if href, ok := s.Attr("href"); ok {
			tab.WebURL = absURL(href)
		} else {
			tab.Selected = true
		}
		out = append(out, tab)
	})
	return out
}

// relatedShelves reads the right hand column.
//
// The counts are site wide and not about these results, which the record says
// in the type's own comment rather than here, since that caveat has to travel
// with the data rather than with the parser.
func relatedShelves(doc *goquery.Document) []ShelfCount {
	var out []ShelfCount
	body := sectionBody(doc, "Related Shelves")
	body.Find("a[href^='/shelf/show/']").Each(func(_ int, a *goquery.Selection) {
		href, _ := a.Attr("href")
		name := strings.TrimSpace(a.Text())
		slug := pathSegmentAfter(href, "/shelf/show/")
		if name == "" || slug == "" {
			return
		}
		sc := ShelfCount{
			Shelf:  Ref{Type: "Shelf", ID: slug, Key: "Shelf:" + slug, Title: name, Resolved: true},
			Name:   name,
			WebURL: absURL(href),
		}
		// The count is a sibling span rather than anything inside the link, so
		// it is read off the text that follows.
		if m := reShelfCount.FindStringSubmatch(a.Next().Text()); m != nil {
			if v := commaFreeInt(m[1]); v > 0 {
				sc.Count = &v
			}
		}
		out = append(out, sc)
	})
	return out
}

// lastPage is the largest numbered page in the pager.
//
// The site does not print how many pages there are, only a window of links with
// a gap in the middle and the last one on the end. The largest number in the
// window is that last one, and it is the only bound the page offers on how far
// a walk could go.
func lastPage(doc *goquery.Document) int {
	last := 0
	doc.Find("div[style*='float: right'] a, div.pagination a").Each(func(_ int, a *goquery.Selection) {
		t := strings.TrimSpace(a.Text())
		if !rePageNumber.MatchString(t) {
			return
		}
		if n := int(commaFreeInt(t)); n > last {
			last = n
		}
	})
	return last
}

// paramOf pulls one query parameter out of a URL without parsing the whole
// thing, since these hrefs come out of markup and are not always well formed.
func paramOf(rawurl, name string) string {
	i := strings.IndexByte(rawurl, '?')
	if i < 0 {
		return ""
	}
	for _, part := range strings.Split(rawurl[i+1:], "&") {
		k, v, ok := strings.Cut(part, "=")
		if !ok || k != name {
			continue
		}
		if unescaped, err := url.QueryUnescape(v); err == nil {
			return unescaped
		}
		return v
	}
	return ""
}

// SearchFrom turns a search extraction into a record.
func SearchFrom(e *Extractor, q SearchQuery, retrievedAt time.Time) (*SearchRecord, error) {
	if e == nil {
		return nil, fmt.Errorf("no extraction")
	}
	q = q.Norm()
	rec := &SearchRecord{Envelope: envelopeOf(e, "search", retrievedAt)}
	rec.Query = q.Query
	if v := firstString(e, "query"); v != "" {
		rec.Query = v
	}
	rec.SearchType = q.Type
	if v := firstString(e, "search_type"); v != "" {
		rec.SearchType = v
	}
	rec.Field = q.Field
	if v := firstString(e, "field"); v != "" {
		rec.Field = v
	}
	rec.Source = q.Source
	rec.WebURL = firstString(e, "web_url")
	rec.QID = firstString(e, "qid")

	rec.Page = q.Page
	if p, ok := e.Fields["page"].(int); ok && p > 0 {
		rec.Page = p
	}
	rec.PagesWalked = []int{rec.Page}

	if n, ok := e.Fields["total_results"].(int64); ok && n > 0 {
		rec.TotalResults = &n
	}
	rec.Approximate, _ = e.Fields["approximate"].(bool)
	if v, ok := e.Fields["elapsed"].(float64); ok {
		rec.Elapsed = &v
	}
	if r, ok := e.Fields["showing"].(ShowRange); ok {
		rec.Showing = &r
	}
	if g, ok := e.Fields["genre"].(Ref); ok {
		rec.Genre = &g
	}
	rec.Tabs, _ = e.Fields["tabs"].([]SearchTab)
	if len(rec.Tabs) == 0 {
		rec.Tabs = knownTabs(rec.SearchType)
	}
	rec.RelatedShelves, _ = e.Fields["related_shelves"].([]ShelfCount)
	rec.NextPage = firstString(e, "next_page")
	if n, ok := e.Fields["last_page"].(int); ok && n > 0 {
		rec.LastPage = &n
	}
	rec.Results, _ = e.Fields["results"].([]SearchHit)

	// The row gives an edition count and a work id and no editions, and it
	// gives a rating and no distribution. Said once here rather than per row.
	if len(rec.Results) > 0 {
		e.Miss("a search row carries the book, its author and its work id, and nothing else about any of them. `goodread book <id>` for the edition, `goodread editions <work id>` for the rest of the printings.")
		rec.Missed = append(rec.Missed, e.Missed[len(e.Missed)-1])
	}
	// The people tab answers, politely, with nothing. That is the site's answer
	// to an anonymous reader and not this tool failing to read the page, and a
	// record that stayed silent about it would look like a parse that missed.
	if len(rec.Results) == 0 && rec.SearchType == SearchTypePeople {
		e.Miss("the people tab answers with zero rows when nobody is signed in. this is what the site returned, not a page this tool could not read.")
		rec.Missed = append(rec.Missed, e.Missed[len(e.Missed)-1])
	}
	if rec.TotalResults != nil && rec.Showing != nil && rec.Showing.To < *rec.TotalResults {
		e.Miss("this is page %d of %d results. the rest are on the following pages, at --pages.", rec.Page, *rec.TotalResults)
		rec.Missed = append(rec.Missed, e.Missed[len(e.Missed)-1])
	}
	return rec, nil
}
