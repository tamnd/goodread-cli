package goodread

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

// The search tests, against the bytes Goodreads served on 2026-08-16.
//
// Every one of them asserts a value that is in the capture rather than a shape,
// because a test that only checks a field is non empty passes just as happily
// on a parser that reads the wrong element.

func TestExtractSuggestReadsEveryField(t *testing.T) {
	body := readCapture(t, "book_auto_complete_hunger_games.json.gz")
	e, err := ExtractSuggest(body, SearchAutocompleteURL("hunger games"), "hunger games")
	if err != nil {
		t.Fatalf("ExtractSuggest: %v", err)
	}
	rec, err := SuggestFrom(e, SearchQuery{Query: "hunger games"}, time.Now().UTC())
	if err != nil {
		t.Fatalf("SuggestFrom: %v", err)
	}

	if rec.Kind != "search" || rec.Surfaces[0] != "s14" {
		t.Errorf("record is kind %q off %v, want a search off s14", rec.Kind, rec.Surfaces)
	}
	if rec.QID == "" {
		t.Error("no qid, so nothing ties these ranks to the run that made them")
	}
	if len(rec.Results) == 0 {
		t.Fatal("no results")
	}

	h := rec.Results[0]
	if h.Book.ID != "2767052" {
		t.Errorf("book id = %q, want 2767052", h.Book.ID)
	}
	// The whole reason this surface is worth reading: the work id, free.
	if h.Work == nil || h.Work.ID != "2792775" {
		t.Errorf("work = %+v, want work 2792775", h.Work)
	}
	if h.Rank != 1 {
		t.Errorf("rank = %d, want 1", h.Rank)
	}
	if h.Title != "The Hunger Games (The Hunger Games, #1)" {
		t.Errorf("title = %q", h.Title)
	}
	if h.TitleBare != "The Hunger Games" {
		t.Errorf("title bare = %q", h.TitleBare)
	}
	if h.Series == nil || h.Series.Series.Title != "The Hunger Games" || h.Series.Number == nil || *h.Series.Number != 1 {
		t.Errorf("series = %+v, want The Hunger Games #1", h.Series)
	}
	if h.Series != nil && h.Series.Series.Resolved {
		t.Error("the series came out of a title suffix and has no id, so it must not claim to be resolved")
	}
	// avgRating arrives as the string "4.35" and has to land as a number.
	if h.AverageRating == nil || *h.AverageRating != 4.35 {
		t.Errorf("average rating = %v, want 4.35", h.AverageRating)
	}
	if h.RatingsCount == nil || *h.RatingsCount != 10233933 {
		t.Errorf("ratings count = %v, want 10233933", h.RatingsCount)
	}
	if h.NumPages == nil || *h.NumPages != 374 {
		t.Errorf("num pages = %v, want 374", h.NumPages)
	}
	if !strings.Contains(h.ImageURL, "2767052._SX50_") {
		t.Errorf("image url = %q", h.ImageURL)
	}
	if h.WebURL != BaseURL+"/book/show/2767052-the-hunger-games" {
		t.Errorf("web url = %q, want the book url with no tracking on it", h.WebURL)
	}
	// The suggest endpoint hands over a clean URL and puts the tracking in
	// fields of its own, so there is no tracked link to keep and the four facts
	// land as themselves.
	if h.TrackedURL != "" {
		t.Errorf("tracked url = %q, and this endpoint writes clean links", h.TrackedURL)
	}
	if h.QID == "" || !h.FromSearch || !h.FromSRP {
		t.Errorf("row tracking = qid %q, from_search %v, from_srp %v", h.QID, h.FromSearch, h.FromSRP)
	}
	if h.DescriptionHTML == "" || !strings.Contains(h.DescriptionHTML, "<br/>") {
		t.Errorf("description html = %q, want the endpoint's own markup", h.DescriptionHTML)
	}
	if h.Description == "" || strings.Contains(h.Description, "<br") {
		t.Errorf("description = %q, want the same text with the tags out", h.Description)
	}
	if h.DescriptionTruncated == nil || !*h.DescriptionTruncated {
		t.Errorf("description truncated = %v, want true", h.DescriptionTruncated)
	}
	if h.DescriptionURL == "" {
		t.Error("no description url, so nothing says where the rest of it is")
	}
	if len(h.Contributors) != 1 {
		t.Fatalf("contributors = %d, want 1", len(h.Contributors))
	}
	a := h.Contributors[0]
	if a.ID != "153394" || a.Name != "Suzanne Collins" || a.Role != "Author" {
		t.Errorf("author = %+v", a)
	}
	if a.WorksURL == "" || !strings.Contains(a.WorksURL, "/author/list/") {
		t.Errorf("author works url = %q, want the author list page", a.WorksURL)
	}
	if a.IsGRAuthor == nil || *a.IsGRAuthor {
		t.Errorf("is goodreads author = %v, want a false that is present", a.IsGRAuthor)
	}
	if a.Via != "s14" || h.Via != "s14" {
		t.Errorf("via = %q / %q, want s14 on both", a.Via, h.Via)
	}

	// The endpoint has no total and no pages, and the record has to say so
	// rather than looking like it returned everything.
	if !containsSubstring(rec.Missed, "does not paginate") {
		t.Errorf("missed = %v, want the note about there being no total", rec.Missed)
	}
	if len(e.Unknown) > 0 {
		t.Logf("the suggest endpoint grew fields this model does not read: %v", e.Unknown)
	}
}

func TestExtractSearchReadsThePage(t *testing.T) {
	body := readCapture(t, "search_books_hunger_games.html.gz")
	u := SearchURL(SearchQuery{Query: "the hunger games"})
	e, err := ExtractSearch(body, u)
	if err != nil {
		t.Fatalf("ExtractSearch: %v", err)
	}
	rec, err := SearchFrom(e, SearchQuery{Query: "the hunger games"}, time.Now().UTC())
	if err != nil {
		t.Fatalf("SearchFrom: %v", err)
	}

	if rec.Surfaces[0] != "s10" {
		t.Errorf("surfaces = %v, want s10", rec.Surfaces)
	}
	if rec.Query != "the hunger games" {
		t.Errorf("query = %q", rec.Query)
	}
	if rec.Page != 1 {
		t.Errorf("page = %d, want 1", rec.Page)
	}
	if rec.TotalResults == nil || *rec.TotalResults != 816 {
		t.Errorf("total = %v, want 816", rec.TotalResults)
	}
	if !rec.Approximate {
		t.Error("the page says about 816 and the record does not say it is approximate")
	}
	if rec.Elapsed == nil || *rec.Elapsed != 0.08 {
		t.Errorf("elapsed = %v, want 0.08", rec.Elapsed)
	}
	if rec.Showing == nil || rec.Showing.From != 1 || rec.Showing.To != 20 || rec.Showing.Of != 816 || rec.Showing.Kind != "books" {
		t.Errorf("showing = %+v, want 1-20 of 816 books", rec.Showing)
	}
	if rec.QID != "gK2HyUg6E1" {
		t.Errorf("qid = %q, want gK2HyUg6E1", rec.QID)
	}
	if rec.Genre == nil || rec.Genre.ID != "the-hunger-games" {
		t.Errorf("genre = %+v, want the-hunger-games", rec.Genre)
	}
	if rec.LastPage == nil || *rec.LastPage != 41 {
		t.Errorf("last page = %v, want 41", rec.LastPage)
	}
	if !strings.Contains(rec.NextPage, "page=2") {
		t.Errorf("next page = %q", rec.NextPage)
	}

	if len(rec.RelatedShelves) != 11 {
		t.Errorf("related shelves = %d, want 11", len(rec.RelatedShelves))
	}
	if len(rec.RelatedShelves) > 0 {
		s := rec.RelatedShelves[0]
		if s.Name != "fantasy" || s.Count == nil || *s.Count != 19926073 {
			t.Errorf("first shelf = %+v", s)
		}
	}

	if len(rec.Tabs) != 5 {
		t.Fatalf("tabs = %d, want 5", len(rec.Tabs))
	}
	byType := map[string]SearchTab{}
	for _, tab := range rec.Tabs {
		byType[tab.Type] = tab
	}
	if !byType["books"].Selected {
		t.Error("the books tab is the one that answered and is not marked selected")
	}
	if byType["books"].WebURL != "" {
		t.Error("the selected tab is a span with no href and must not carry a url")
	}
	for _, typ := range []string{"lists", "groups", "quotes"} {
		if byType[typ].Readable {
			t.Errorf("the %s tab is marked readable, and it answers with a challenge", typ)
		}
		if byType[typ].Note == "" {
			t.Errorf("the %s tab is marked unreadable and does not say why", typ)
		}
	}
	if !byType["people"].Readable {
		t.Error("the people tab answers, with zero rows, and is marked unreadable")
	}

	if len(rec.Results) != 20 {
		t.Fatalf("results = %d, want 20", len(rec.Results))
	}
	h := rec.Results[0]
	if h.Rank != 1 || h.Book.ID != "2767052" {
		t.Errorf("first hit = rank %d, book %q", h.Rank, h.Book.ID)
	}
	if h.Title != "The Hunger Games (The Hunger Games, #1)" {
		t.Errorf("title = %q", h.Title)
	}
	if h.Work == nil || h.Work.ID != "2792775" {
		t.Errorf("work = %+v, want 2792775 off the editions link", h.Work)
	}
	if h.EditionsCount == nil || *h.EditionsCount != 80 {
		t.Errorf("editions count = %v, want 80", h.EditionsCount)
	}
	if h.AverageRating == nil || *h.AverageRating != 4.35 {
		t.Errorf("average rating = %v, want 4.35", h.AverageRating)
	}
	if h.RatingsCount == nil || *h.RatingsCount != 10233676 {
		t.Errorf("ratings count = %v, want 10233676", h.RatingsCount)
	}
	if h.PublishedAt != "2008" {
		t.Errorf("published = %q, want 2008", h.PublishedAt)
	}
	if h.WebURL != BaseURL+"/book/show/2767052-the-hunger-games" {
		t.Errorf("web url = %q, want the tracking stripped", h.WebURL)
	}
	if !strings.Contains(h.TrackedURL, "qid=gK2HyUg6E1") {
		t.Errorf("tracked url = %q, want the link the page wrote", h.TrackedURL)
	}
	if len(h.Contributors) != 1 || h.Contributors[0].ID != "153394" {
		t.Errorf("contributors = %+v", h.Contributors)
	}
	if !strings.Contains(h.ImageURL, "_SX50_") {
		t.Errorf("image url = %q, want the thumbnail the row renders", h.ImageURL)
	}
	if h.Via != "s10" {
		t.Errorf("via = %q, want s10", h.Via)
	}

	// Ranks are the site's, per page, and every row carries one.
	for i, hit := range rec.Results {
		if hit.Rank != i+1 {
			t.Errorf("row %d has rank %d, and the page numbers them in order", i, hit.Rank)
		}
	}
}

// TestSearchPeopleTabIsEmptySignedOut holds the measured fact rather than an
// assumption about it. The tab answers 200 and returns nothing, and a user who
// runs it deserves to be told that is the site's answer and not a parse failure.
func TestSearchPeopleTabIsEmptySignedOut(t *testing.T) {
	body := readCapture(t, "search_people_hunger_games.html.gz")
	u := SearchURL(SearchQuery{Query: "the hunger games", Type: SearchTypePeople})
	e, err := ExtractSearch(body, u)
	if err != nil {
		t.Fatalf("ExtractSearch: %v", err)
	}
	rec, err := SearchFrom(e, SearchQuery{Query: "the hunger games", Type: SearchTypePeople}, time.Now().UTC())
	if err != nil {
		t.Fatalf("SearchFrom: %v", err)
	}
	if rec.SearchType != SearchTypePeople {
		t.Errorf("search type = %q, want people", rec.SearchType)
	}
	if len(rec.Results) != 0 {
		t.Errorf("results = %d, want none: this tab returns nothing when signed out", len(rec.Results))
	}
	if rec.Showing == nil || rec.Showing.Of != 0 || rec.Showing.Kind != "people" {
		t.Errorf("showing = %+v, want 0 people", rec.Showing)
	}
	// An empty answer that says nothing about itself reads as a broken parser.
	if !containsSubstring(rec.Missed, "nobody is signed in") {
		t.Errorf("missed = %v, want the sentence saying this is the site's answer", rec.Missed)
	}
}

// TestBlockedTabsAreReportedNotSolved is the security line, held as a test.
//
// Three of the five tabs answer with an AWS WAF challenge. This tool reports
// that and stops. There is no cookie flag, no browser, and no code path that
// tries to satisfy a challenge, and the refusal happens before a request is
// spent rather than after.
func TestBlockedTabsAreReportedNotSolved(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NoRobots = true
	cfg.Delay = MinDelay

	requests := 0
	c, _ := testClient(t, cfg, func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.WriteHeader(500)
	})
	c.SetWarnWriter(nil)

	for _, typ := range []string{SearchTypeLists, SearchTypeGroups, SearchTypeQuotes} {
		_, err := c.GetSearchRecord(context.Background(), SearchQuery{Query: "dune", Type: typ}, 1)
		if err == nil {
			t.Fatalf("the %s tab came back with no error, and it is not readable", typ)
		}
		if !strings.Contains(err.Error(), "blocked") {
			t.Errorf("the %s tab failed with %v, want a block", typ, err)
		}
	}
	if requests != 0 {
		t.Errorf("%d requests were spent on tabs known to answer with a challenge", requests)
	}
}

// TestSearchRecordCarriesTheRobotsNote is the acceptance line for the disallowed
// half: a record built off /search says it came off a page the site declined.
func TestSearchRecordCarriesTheRobotsNote(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NoRobots = true
	cfg.Delay = MinDelay

	body := readCapture(t, "search_books_hunger_games.html.gz")
	c, _ := testClient(t, cfg, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			t.Errorf("server saw %s, want /search", r.URL.Path)
		}
		_, _ = w.Write(body)
	})
	c.SetWarnWriter(nil)

	rec, err := c.GetSearchRecord(context.Background(), SearchQuery{Query: "the hunger games"}, 1)
	if err != nil {
		t.Fatalf("GetSearchRecord: %v", err)
	}
	if rec.Robots == nil {
		t.Fatal("a record off /search carries no robots note")
	}
	if rec.Robots.Allowed {
		t.Error("the note says allowed = true for a path robots.txt disallows")
	}
	if rec.Robots.Rule != "Disallow: /search" {
		t.Errorf("rule = %q, want the line from the file", rec.Robots.Rule)
	}
}

// TestSuggestRecordCarriesNoRobotsNote is the other half. The allowed surface
// must not be marked, or the mark stops meaning anything.
func TestSuggestRecordCarriesNoRobotsNote(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NoRobots = true
	cfg.Delay = MinDelay

	body := readCapture(t, "book_auto_complete_hunger_games.json.gz")
	c, _ := testClient(t, cfg, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	})
	c.SetWarnWriter(nil)

	rec, err := c.GetSuggestRecord(context.Background(), "hunger games")
	if err != nil {
		t.Fatalf("GetSuggestRecord: %v", err)
	}
	if rec.Robots != nil {
		t.Errorf("the allowed suggest endpoint came back marked %+v", *rec.Robots)
	}
}

// TestSearchRefusedWithoutTheFlag is the default, which is the important one.
func TestSearchRefusedWithoutTheFlag(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Delay = MinDelay

	requests := 0
	c, _ := testClient(t, cfg, func(w http.ResponseWriter, _ *http.Request) {
		requests++
		_, _ = w.Write([]byte("<html></html>"))
	})
	c.SetWarnWriter(nil)

	_, err := c.GetSearchRecord(context.Background(), SearchQuery{Query: "dune"}, 1)
	if err == nil {
		t.Fatal("/search was read with no --no-robots")
	}
	if requests != 0 {
		t.Errorf("%d requests were spent on a path the rules refuse", requests)
	}
}

func containsSubstring(list []string, want string) bool {
	for _, s := range list {
		if strings.Contains(s, want) {
			return true
		}
	}
	return false
}

// TestSearchWalksPagesTheSitesOwnWay is the multi page read.
//
// The second page is the capture with its sub nav and its next link edited, so
// the walk has something to follow and something to disagree with if it built
// the URL itself. The assertion that matters is that page two was asked for
// through the link the first page wrote, qid and all, rather than through a
// page number this tool appended to the query.
func TestSearchWalksPagesTheSitesOwnWay(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NoRobots = true
	cfg.Delay = MinDelay

	first := readCapture(t, "search_books_hunger_games.html.gz")
	second := []byte(strings.NewReplacer(
		"Page 1 of about", "Page 2 of about",
		"page=2", "page=3",
	).Replace(string(first)))

	var seen []string
	c, _ := testClient(t, cfg, func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.RequestURI())
		if r.URL.Query().Get("page") == "2" {
			_, _ = w.Write(second)
			return
		}
		_, _ = w.Write(first)
	})
	c.SetWarnWriter(nil)

	rec, err := c.GetSearchRecord(context.Background(), SearchQuery{Query: "the hunger games"}, 2)
	if err != nil {
		t.Fatalf("GetSearchRecord: %v", err)
	}
	if len(seen) != 2 {
		t.Fatalf("requests = %v, want two pages", seen)
	}
	if !strings.Contains(seen[1], "page=2") || !strings.Contains(seen[1], "qid=") {
		t.Errorf("the second request was %q, and the page's own next link carries the qid", seen[1])
	}
	if len(rec.PagesWalked) != 2 || rec.PagesWalked[0] != 1 || rec.PagesWalked[1] != 2 {
		t.Errorf("pages walked = %v, want 1 and 2", rec.PagesWalked)
	}
	if len(rec.Results) != 40 {
		t.Errorf("results = %d, want the rows of both pages", len(rec.Results))
	}
	// The page level facts stay as the first page stated them, because they
	// describe the search rather than the page.
	if rec.Page != 1 || rec.TotalResults == nil || *rec.TotalResults != 816 {
		t.Errorf("page = %d, total = %v, want the first page's own account", rec.Page, rec.TotalResults)
	}
	// And the next link moves on, so a third page would be asked for correctly.
	if !strings.Contains(rec.NextPage, "page=3") {
		t.Errorf("next page = %q, want the link the second page wrote", rec.NextPage)
	}
}
