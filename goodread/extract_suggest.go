package goodread

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ExtractSuggest reads /book/auto_complete.
//
// The allowed search route, and the only surface on the whole site that hands
// over a book id and its work id in the same response without a second fetch.
// That makes it the cheapest way into edition_of, which is why v0.3.0 throwing
// nineteen of its twenty three fields away was worth going back for.
//
// Level 1. It is not a __NEXT_DATA__ payload, but it is the same rung: typed
// JSON the site publishes for its own front end, with no markup between the
// data and the reader. Nothing here is read by selector.
func ExtractSuggest(body []byte, pageURL, query string) (*Extractor, error) {
	e := NewExtractor("s14")

	var rows []suggestRow
	if err := json.Unmarshal(body, &rows); err != nil {
		return e, fmt.Errorf("parse the suggest response: %w", err)
	}

	e.set("query", LevelNextData, query)
	e.set("web_url", LevelNextData, pageURL)
	e.set("search_type", LevelNextData, SearchTypeBooks)

	// Decoded a second time as bare objects, so a field Goodreads adds shows up
	// in the unknown list rather than being dropped by a decoder that only
	// knows the fields somebody wrote down last year.
	var raw []map[string]json.RawMessage
	_ = json.Unmarshal(body, &raw)
	for _, obj := range raw {
		for k := range obj {
			if !suggestKnownFields[k] {
				e.NoteUnknown("suggest." + k)
			}
		}
	}

	hits := make([]SearchHit, 0, len(rows))
	for i, r := range rows {
		hits = append(hits, r.hit(i+1))
		if r.QID != "" {
			e.set("qid", LevelNextData, r.QID)
		}
	}
	e.set("results", LevelNextData, hits)

	if len(hits) == 0 {
		// Not an error. A query with no matches answers with an empty array and
		// so does a query the endpoint does not understand, and neither is this
		// tool failing.
		e.Miss("the suggest endpoint returned no rows, which is either no matches or a query it does not index. it does not say which.")
	}
	return e, nil
}

// suggestRow is one row of the suggest response, every field of it.
//
// Written out in full rather than picking the interesting ones, so that a field
// the endpoint adds tomorrow shows up in the unknown list instead of vanishing.
type suggestRow struct {
	ImageURL      string          `json:"imageUrl"`
	BookID        string          `json:"bookId"`
	WorkID        string          `json:"workId"`
	BookURL       string          `json:"bookUrl"`
	FromSearch    bool            `json:"from_search"`
	FromSRP       bool            `json:"from_srp"`
	QID           string          `json:"qid"`
	Rank          int             `json:"rank"`
	Title         string          `json:"title"`
	BookTitleBare string          `json:"bookTitleBare"`
	NumPages      *int            `json:"numPages"`
	AvgRating     json.RawMessage `json:"avgRating"`
	RatingsCount  json.RawMessage `json:"ratingsCount"`
	KCRPreviewURL string          `json:"kcrPreviewUrl"`
	Author        struct {
		ID                int64  `json:"id"`
		Name              string `json:"name"`
		IsGoodreadsAuthor bool   `json:"isGoodreadsAuthor"`
		ProfileURL        string `json:"profileUrl"`
		WorksListURL      string `json:"worksListUrl"`
	} `json:"author"`
	Description struct {
		HTML           string `json:"html"`
		Truncated      bool   `json:"truncated"`
		FullContentURL string `json:"fullContentUrl"`
	} `json:"description"`
}

// suggestKnownFields is what the struct above covers, for the unknown check.
var suggestKnownFields = map[string]bool{
	"imageUrl": true, "bookId": true, "workId": true, "bookUrl": true,
	"from_search": true, "from_srp": true, "qid": true, "rank": true,
	"title": true, "bookTitleBare": true, "numPages": true, "avgRating": true,
	"ratingsCount": true, "kcrPreviewUrl": true, "author": true, "description": true,
}

// hit turns one row into a SearchHit.
func (r suggestRow) hit(order int) SearchHit {
	id := r.BookID
	title := r.Title
	bare := r.BookTitleBare
	if bare == "" {
		bare = TitleWithoutSeries(title)
	}

	h := SearchHit{
		BookCard: BookCard{
			Book:      Ref{Type: "Book", ID: id, Key: "Book:" + id, Title: bare, Resolved: id != ""},
			Title:     title,
			TitleBare: bare,
			ImageURL:  r.ImageURL,
			Via:       "s14",
		},
		Rank:            r.Rank,
		WebURL:          untrack(absURL(r.BookURL)),
		QID:             r.QID,
		FromSearch:      r.FromSearch,
		FromSRP:         r.FromSRP,
		DescriptionHTML: r.Description.HTML,
		PreviewURL:      r.KCRPreviewURL,
	}
	if h.Rank == 0 {
		h.Rank = order
	}
	if r.WorkID != "" {
		h.Work = &Ref{Type: "Work", ID: r.WorkID, Key: "Work:" + r.WorkID, Resolved: true}
	}
	if r.NumPages != nil && *r.NumPages > 0 {
		n := *r.NumPages
		h.NumPages = &n
	}
	// avgRating arrives quoted, "4.35", which is the endpoint's habit and not a
	// number this record should pass on as a string.
	if v := parseRawFloat(r.AvgRating); v > 0 {
		h.AverageRating = &v
	}
	if v := parseRawInt64(r.RatingsCount); v > 0 {
		h.RatingsCount = &v
	}
	if r.Description.HTML != "" {
		h.Description = cleanHTML(r.Description.HTML)
		truncated := r.Description.Truncated
		h.DescriptionTruncated = &truncated
		h.DescriptionURL = r.Description.FullContentURL
	}
	if r.Author.Name != "" {
		aid := ""
		if r.Author.ID > 0 {
			aid = strconv.FormatInt(r.Author.ID, 10)
		}
		isGR := r.Author.IsGoodreadsAuthor
		h.Contributors = []Contributor{{
			ID:         aid,
			LegacyID:   r.Author.ID,
			Name:       r.Author.Name,
			Role:       "Author",
			WebURL:     r.Author.ProfileURL,
			WorksURL:   r.Author.WorksListURL,
			IsGRAuthor: &isGR,
			Via:        "s14",
		}}
	}
	h.Series = seriesFromTitle(title)
	return h
}

// seriesFromTitle reads the "(The Hunger Games, #1)" suffix.
//
// The reference gets a name and no id, and is marked unresolved, because the
// suffix is text and nothing on either search surface says which series it
// points at. An unresolved reference with a name is honest; inventing a key off
// the name would create edges to a series that may not exist.
func seriesFromTitle(title string) *SeriesEntry {
	open := strings.LastIndex(title, " (")
	if open < 0 || !strings.HasSuffix(strings.TrimSpace(title), ")") {
		return nil
	}
	inner := strings.TrimSuffix(strings.TrimSpace(title[open+2:]), ")")
	hash := strings.LastIndex(inner, "#")
	if hash < 0 {
		return nil
	}
	name := strings.TrimRight(strings.TrimSpace(inner[:hash]), ",")
	pos := strings.TrimSpace(inner[hash+1:])
	if name == "" {
		return nil
	}
	entry := &SeriesEntry{
		Series:   Ref{Type: "Series", Title: name},
		Position: pos,
	}
	if n, ok := parsePosition(pos); ok {
		entry.Number = &n
	}
	return entry
}

// SuggestFrom turns a suggest extraction into a search record.
func SuggestFrom(e *Extractor, q SearchQuery, retrievedAt time.Time) (*SearchRecord, error) {
	if e == nil {
		return nil, fmt.Errorf("no extraction")
	}
	q = q.Norm()
	rec := &SearchRecord{Envelope: envelopeOf(e, "search", retrievedAt)}
	rec.Query = q.Query
	if v := firstString(e, "query"); v != "" {
		rec.Query = v
	}
	rec.SearchType = firstString(e, "search_type")
	rec.Source = q.Source
	rec.WebURL = firstString(e, "web_url")
	rec.QID = firstString(e, "qid")
	rec.Page = 1
	rec.PagesWalked = []int{1}
	rec.Results, _ = e.Fields["results"].([]SearchHit)
	rec.Tabs = knownTabs("")

	// Said every time, because the endpoint has no total and no pagination.
	// Twenty rows out of an unknown number looks like twenty rows out of twenty
	// to anything that only reads the results.
	e.Miss("the suggest endpoint returns a fixed handful of rows, says no total and does not paginate. use --deep with --no-robots for the search page, which does.")
	rec.Missed = append(rec.Missed, e.Missed[len(e.Missed)-1])
	return rec, nil
}

// knownTabs is the tab list built from what this tool measured, for the surface
// that does not render one.
func knownTabs(selected string) []SearchTab {
	out := make([]SearchTab, 0, len(SearchTabs))
	for _, t := range SearchTabs {
		out = append(out, SearchTab{
			Type:     t.Type,
			Label:    t.Label,
			Readable: t.Readable,
			Note:     t.Note,
			Selected: t.Type == selected,
		})
	}
	return out
}
