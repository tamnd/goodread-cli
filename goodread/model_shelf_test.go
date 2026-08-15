package goodread

import (
	"strings"
	"testing"
	"time"
)

// These run against rows in the shape ParseShelfRSS and ParseShelf return,
// rather than against a capture, because both shelf routes are disallowed by
// robots.txt and capturing one is the thing --no-robots exists to make a
// person's own decision. The parsers on the other side of this are already
// tested. What is tested here is the fold into the record, which is where the
// four states of a missing field either survive or get flattened.

func shelfRows() []ShelfBook {
	return []ShelfBook{
		{
			ShelfID: "1/read", UserID: "1", BookID: "2767052",
			Title: "The Hunger Games (The Hunger Games, #1)", AuthorName: "Suzanne Collins",
			ISBN: "0439023483", NumPages: 374, AvgRating: 4.35, Rating: 5,
			Shelves:  []string{"read", "favourites"},
			ReviewID: "1234", ReviewText: "still the best of the three",
			DateAdded:     time.Date(2019, 3, 1, 0, 0, 0, 0, time.UTC),
			DateRead:      time.Date(2023, 7, 14, 0, 0, 0, 0, time.UTC),
			CoverURL:      "https://images.gr-assets.com/books/1.jpg",
			BookPublished: 2008,
		},
		{
			// The row that matters: shelved, never rated, never read.
			ShelfID: "1/read", UserID: "1", BookID: "1885",
			Title: "Pride and Prejudice", AuthorName: "Jane Austen",
			DateAdded: time.Date(2021, 1, 5, 0, 0, 0, 0, time.UTC),
		},
	}
}

func TestShelfFromKeepsThePersonsSideApart(t *testing.T) {
	s := &Shelf{ShelfID: "1/read", UserID: "1", Name: "read", BooksCount: 2, URL: "https://www.goodreads.com/review/list_rss/1?shelf=read"}
	rec := ShelfFrom(s, shelfRows(), "rss", time.Now())

	if rec.ID != "1/read" {
		t.Errorf("id = %q, and a shelf is only identified by user and name together", rec.ID)
	}
	if rec.User.ID != "1" || rec.User.Type != "User" {
		t.Errorf("user = %+v", rec.User)
	}
	if rec.Source != "rss" {
		t.Errorf("source = %q", rec.Source)
	}
	if rec.Loaded != 2 {
		t.Errorf("loaded = %d", rec.Loaded)
	}

	first := rec.Books[0]
	if first.Rating == nil || *first.Rating != 5 {
		t.Errorf("their rating = %v", first.Rating)
	}
	if first.AverageRating == nil || *first.AverageRating != 4.35 {
		t.Errorf("the book's average = %v", first.AverageRating)
	}
	// The two ratings are different facts and the record has to hold both, since
	// a person who gives five stars to a book averaging 3.1 is the interesting
	// case and collapsing the pair loses exactly that.
	if first.Rating != nil && first.AverageRating != nil && float64(*first.Rating) == *first.AverageRating {
		t.Error("their rating and the book's average came out the same")
	}
	if first.ISBN != "0439023483" {
		t.Errorf("isbn = %q, and it is the edition they shelved", first.ISBN)
	}
	if len(first.Shelves) != 2 {
		t.Errorf("shelves = %v, and their own tagging exists nowhere else", first.Shelves)
	}
	if first.DateRead == nil || first.DateRead.Year() != 2023 {
		t.Errorf("read date = %v", first.DateRead)
	}
	if first.ReviewText == "" {
		t.Error("the review text was dropped")
	}
}

// TestShelfKeepsUnratedUnrated. Rating 0 out of the parser means they never
// rated it, and a record that printed 0 would be saying they gave it nothing
// out of five.
func TestShelfKeepsUnratedUnrated(t *testing.T) {
	rec := ShelfFrom(&Shelf{ShelfID: "1/read", UserID: "1", Name: "read"}, shelfRows(), "rss", time.Now())
	second := rec.Books[1]

	if second.Rating != nil {
		t.Errorf("their rating = %v on a book they never rated", *second.Rating)
	}
	if second.DateRead != nil {
		t.Errorf("read date = %v on a book they have not read", second.DateRead)
	}
	if second.DateAdded == nil {
		t.Error("added date was dropped, and it is the only date that row has")
	}
}

// TestShelfSaysWhichRouteAndWhatItCost. The two routes carry different fields,
// so a reader comparing two shelf records needs to know which is which before
// the comparison means anything.
func TestShelfSaysWhichRouteAndWhatItCost(t *testing.T) {
	rss := ShelfFrom(&Shelf{ShelfID: "1/read", UserID: "1", BooksCount: 100}, shelfRows(), "rss", time.Now())
	if rss.Level["books"] != LevelNextData {
		t.Errorf("the rss feed read at level %d, and it is the application serialising its own rows", rss.Level["books"])
	}
	var warned bool
	for _, m := range rss.Missed {
		if strings.Contains(m, "--no-robots") && strings.Contains(m, "window") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("the feed's window is not stated: %v", rss.Missed)
	}
	if rss.Complete {
		t.Error("two of a hundred books and the record claims to be complete")
	}

	html := ShelfFrom(&Shelf{ShelfID: "1/read", UserID: "1", BooksCount: 2}, shelfRows(), "html", time.Now())
	if html.Level["books"] != LevelSelector {
		t.Errorf("the html shelf read at level %d, and it is markup being read back", html.Level["books"])
	}
	if !html.Complete {
		t.Error("two of two books and the record does not say it is complete")
	}
	if html.Surfaces[0] != "s11" || rss.Surfaces[0] != "s11" {
		t.Errorf("surfaces = %v and %v, and both routes are s11", html.Surfaces, rss.Surfaces)
	}
}
