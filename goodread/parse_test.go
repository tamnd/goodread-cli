package goodread

import "testing"

func TestParseAutocompleteBooks(t *testing.T) {
	body := []byte(`[
	  {
	    "imageUrl": "https://images.gr-assets.com/books/1.jpg",
	    "bookId": "2767052",
	    "bookUrl": "/book/show/2767052-the-hunger-games",
	    "title": "The Hunger Games (The Hunger Games, #1)",
	    "bookTitleBare": "The Hunger Games",
	    "numPages": 374,
	    "avgRating": "4.34",
	    "ratingsCount": 9123456,
	    "author": {"id": 153394, "name": "Suzanne Collins", "profileUrl": "/author/show/153394"},
	    "description": {"html": "Could you survive on your own&hellip;"}
	  }
	]`)
	books := ParseAutocompleteBooks(body)
	if len(books) != 1 {
		t.Fatalf("got %d books, want 1", len(books))
	}
	b := books[0]
	if b.BookID != "2767052" {
		t.Errorf("BookID = %q", b.BookID)
	}
	if b.TitleWithoutSeries != "The Hunger Games" {
		t.Errorf("TitleWithoutSeries = %q", b.TitleWithoutSeries)
	}
	if b.AuthorID != "153394" || b.AuthorName != "Suzanne Collins" {
		t.Errorf("author = (%q, %q)", b.AuthorID, b.AuthorName)
	}
	if b.Pages != 374 {
		t.Errorf("Pages = %d", b.Pages)
	}
	if b.AvgRating != 4.34 {
		t.Errorf("AvgRating = %v", b.AvgRating)
	}
	if b.RatingsCount != 9123456 {
		t.Errorf("RatingsCount = %d", b.RatingsCount)
	}
	if b.SeriesName != "The Hunger Games" || b.SeriesPosition != "1" {
		t.Errorf("series = (%q, %q), want (The Hunger Games, 1)", b.SeriesName, b.SeriesPosition)
	}
	if b.URL != "https://www.goodreads.com/book/show/2767052-the-hunger-games" {
		t.Errorf("URL = %q", b.URL)
	}
}

func TestParseAutocompleteBooksBadJSON(t *testing.T) {
	if got := ParseAutocompleteBooks([]byte("not json")); got != nil {
		t.Errorf("bad JSON should return nil, got %v", got)
	}
}

func TestCleanQuote(t *testing.T) {
	cases := []struct{ in, want string }{
		{"“Real or not real?”\n    \n  ―", "Real or not real?"},
		{"  plain text  ", "plain text"},
		{"<i>You love me.</i> Real or not real?", "You love me. Real or not real?"},
	}
	for _, c := range cases {
		if got := cleanQuote(c.in); got != c.want {
			t.Errorf("cleanQuote(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCleanBio(t *testing.T) {
	if got := cleanBio("edit data\n  Suzanne Collins is the author of\n the Hunger Games."); got != "Suzanne Collins is the author of the Hunger Games." {
		t.Errorf("cleanBio = %q", got)
	}
}

func TestAtoiClean(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"9,123,456", 9123456},
		{"374 pages", 374},
		{"", 0},
		{"no digits", 0},
	}
	for _, c := range cases {
		if got := atoiClean(c.in); got != c.want {
			t.Errorf("atoiClean(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}
