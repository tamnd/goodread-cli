package goodread

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ExtractSeries reads a series page.
//
// The best of the six Rails surfaces by a distance, because it is the one that
// carries its own JSON. SeriesHeader has the title and the description, and
// each SeriesList island has its books already typed: book id, work id, page
// count, average rating, ratings count, publication year and the author.
// Nothing here is scraped off a rendered string.
//
// So this is level 1, the same rung as the book page, even though there is no
// __NEXT_DATA__ anywhere on it. See reactprops.go for why the two count as the
// same thing.
func ExtractSeries(body []byte) (*Extractor, error) {
	e := NewExtractor("s3")

	if raw, err := ReactProps(body, "SeriesHeader"); err == nil {
		var h seriesHeaderProps
		if json.Unmarshal(raw, &h) == nil {
			// The header renders "<name> Series", so the suffix comes off. A
			// series actually called something Series still keeps its name,
			// because the page would render that one as "X Series Series".
			e.set("title", LevelNextData, seriesTitle(h.Title))
			e.set("description", LevelNextData, strings.TrimSpace(h.Description.HTML))
			primary, total := parseSeriesSubtitle(h.Subtitle)
			if primary > 0 {
				e.set("primary_work_count", LevelNextData, primary)
			}
			if total > 0 {
				e.set("total_work_count", LevelNextData, total)
			}
		}
	}

	var books []seriesPageEntry
	for _, raw := range ReactPropsAll(body, "SeriesList") {
		var p seriesListProps
		if json.Unmarshal(raw, &p) != nil {
			continue
		}
		for i, entry := range p.Series {
			pos := ""
			if i < len(p.SeriesHeaders) {
				pos = p.SeriesHeaders[i]
			}
			books = append(books, seriesPageEntry{Position: pos, Book: entry.ScrapedBook})
		}
	}
	e.set("series_books", LevelNextData, books)

	// The og: tags are read anyway, both as a cross check and so that the day
	// the islands go this degrades to a title rather than to nothing.
	og := parseOpenGraph(body)
	e.set("title", LevelMeta, seriesTitle(og["title"]))
	e.set("web_url", LevelMeta, og["url"])
	e.set("image_url", LevelMeta, og["image"])
	e.set("description", LevelMeta, og["description"])

	if len(e.Fields) == 0 {
		return e, fmt.Errorf("no series data on this page")
	}

	// A series long enough to paginate returns one page and says how many works
	// there are in total, so the shortfall can be named rather than implied.
	if raw, err := ReactProps(body, "FullPagePaginationControls"); err == nil {
		var p paginationProps
		if json.Unmarshal(raw, &p) == nil {
			if p.Page > 0 {
				e.set("page", LevelNextData, p.Page)
			}
			if p.NumWorks > len(books) {
				e.Miss("this page carries %d of %d works in the series. `goodread series <id> --page N` reads the rest.",
					len(books), p.NumWorks)
			}
		}
	}
	return e, nil
}

// seriesPageEntry is one book and the header it was rendered under.
//
// The island keeps the two in parallel arrays, books in series and headers in
// seriesHeaders, so they are paired here rather than anywhere later. A pairing
// done by index has to happen where the arrays are, not three functions away.
type seriesPageEntry struct {
	Position string
	Book     rawBook
}

type paginationProps struct {
	NumWorks int `json:"numWorks"`
	Page     int `json:"currentPageNumber"`
	PerPage  int `json:"perPage"`
}

// card turns the listing shape into the model's BookCard.
//
// rawBook is the v0.2.0 struct and it is reused deliberately. The series page,
// the Listopia rows and the autocomplete endpoint all serialise a book the same
// way, which is not a coincidence: it is one Rails serialiser behind all three.
// Its number fields are RawMessage because autocomplete quotes them and the
// React props do not.
func cardOf(r rawBook, via string) BookCard {
	c := BookCard{
		Book:        Ref{Type: "Book", ID: r.BookID, Key: "Book:" + r.BookID, Title: r.BookTitle, Resolved: r.BookID != ""},
		Title:       strings.TrimSpace(r.Title),
		TitleBare:   strings.TrimSpace(r.BookTitle),
		ImageURL:    r.ImageURL,
		Description: strings.TrimSpace(r.Description.HTML),
		Via:         via,
	}
	if c.Book.Title == "" {
		c.Book.Title = c.Title
	}
	if r.WorkID != "" {
		c.Work = &Ref{Type: "Work", ID: r.WorkID, Key: "Work:" + r.WorkID, Resolved: true}
	}
	if r.NumPages > 0 {
		n := r.NumPages
		c.NumPages = &n
	}
	if v := parseRawFloat(r.AvgRating); v > 0 {
		c.AverageRating = &v
	}
	if v := parseRawInt64(r.RatingsCount); v > 0 {
		c.RatingsCount = &v
	}
	if r.Author.Name != "" {
		c.Contributors = []Contributor{{
			ID:       strconv.FormatInt(r.Author.ID, 10),
			LegacyID: r.Author.ID,
			Name:     r.Author.Name,
			Role:     "Author",
			WebURL:   r.Author.ProfileURL,
			Via:      via,
		}}
	}
	return c
}

// SeriesFrom turns a series extraction into a record.
func SeriesFrom(e *Extractor, id string, retrievedAt time.Time) (*Series, error) {
	if e == nil {
		return nil, fmt.Errorf("no extraction")
	}
	s := &Series{Envelope: envelopeOf(e, "series", retrievedAt)}
	s.ID = numericPrefix(id)
	s.LegacyID, _ = strconv.ParseInt(s.ID, 10, 64)
	s.WebURL = firstString(e, "web_url")
	if s.WebURL == "" {
		s.WebURL = SeriesURL(id)
	}
	s.Title, _ = e.Fields["title"].(string)
	s.Description, _ = e.Fields["description"].(string)
	if n, ok := int64Of(e.Fields["primary_work_count"]); ok {
		s.PrimaryWorkCount = &n
	}
	if n, ok := int64Of(e.Fields["total_work_count"]); ok {
		s.TotalWorkCount = &n
	}

	entries, _ := e.Fields["series_books"].([]seriesPageEntry)
	for _, entry := range entries {
		b := SeriesBookCard{
			BookCard: cardOf(entry.Book, "s3"),
			Position: strings.TrimSpace(entry.Position),
		}
		b.Number = seriesNumber(entry.Position)
		s.Books = append(s.Books, b)
	}
	if len(s.Books) == 0 {
		e.Miss("no books on this series page. the page mounts its books as React islands, so this usually means the markup moved rather than that the series is empty.")
		s.Missed = append(s.Missed, e.Missed[len(e.Missed)-1])
	}
	return s, nil
}

var reSeriesSubtitle = regexp.MustCompile(`(?i)([\d,]+)\s+primary works?[^\d]+([\d,]+)\s+total works?`)

// parseSeriesSubtitle reads "7 primary works • 9 total works".
//
// Two numbers and not one. The difference is companion volumes and omnibus
// editions, and a series recorded with only one of them has the wrong length in
// everything built on top of it.
func parseSeriesSubtitle(s string) (primary, total int64) {
	m := reSeriesSubtitle.FindStringSubmatch(s)
	if m == nil {
		return 0, 0
	}
	return commaFreeInt(m[1]), commaFreeInt(m[2])
}

var reSeriesNumber = regexp.MustCompile(`^(?:Books?\s+)?(\d+(?:\.\d+)?)$`)

// seriesNumber parses "Book 0.5" and gives up on "Books 1-3".
//
// Giving up is the right outcome for a range. An omnibus of books one through
// three sits at no single position, and taking the first number would file it
// somewhere it does not belong.
func seriesNumber(position string) *float64 {
	m := reSeriesNumber.FindStringSubmatch(strings.TrimSpace(position))
	if m == nil {
		return nil
	}
	n, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return nil
	}
	return &n
}

// seriesTitle strips the one word the header adds.
func seriesTitle(s string) string {
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), " Series"))
}

// commaFreeInt reads "235,951" the way a page prints it.
func commaFreeInt(s string) int64 {
	n, err := strconv.ParseInt(strings.ReplaceAll(strings.TrimSpace(s), ",", ""), 10, 64)
	if err != nil {
		return 0
	}
	return n
}
