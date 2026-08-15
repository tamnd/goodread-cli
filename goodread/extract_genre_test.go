package goodread

import (
	"strings"
	"testing"
	"time"
)

const fantasyURL = "https://www.goodreads.com/genres/fantasy"

func genreFromCapture(t *testing.T) *Genre {
	t.Helper()
	e, err := ExtractGenre(readCapture(t, "genres_fantasy.html.gz"), fantasyURL)
	if err != nil {
		t.Fatalf("ExtractGenre: %v", err)
	}
	g, err := GenreFrom(e, "fantasy", time.Now())
	if err != nil {
		t.Fatalf("GenreFrom: %v", err)
	}
	return g
}

func TestGenreReadsItsHeader(t *testing.T) {
	g := genreFromCapture(t)

	if g.Name != "Fantasy" {
		t.Errorf("name = %q", g.Name)
	}
	if g.Slug != "fantasy" {
		t.Errorf("slug = %q", g.Slug)
	}
	if g.WebURL != fantasyURL {
		t.Errorf("web url = %q", g.WebURL)
	}
}

// TestGenreRelatedIsTheGenreGraph is the reason this surface is read at all.
//
// The trap it guards: the page also renders the site wide genre nav, twenty odd
// links that are identical on every genre page. Picking those up would give a
// graph where fantasy relates to Business and Cookbooks, which looks like data
// and means nothing.
func TestGenreRelatedIsTheGenreGraph(t *testing.T) {
	g := genreFromCapture(t)

	if len(g.Related) < 10 {
		t.Fatalf("%d related genres, and the page lists two dozen", len(g.Related))
	}
	byName := map[string]bool{}
	for _, r := range g.Related {
		if r.ID == "" || r.Title == "" {
			t.Errorf("related %+v is missing half of itself", r)
		}
		if r.Type != "Genre" {
			t.Errorf("related %q typed %q", r.Title, r.Type)
		}
		byName[r.Title] = true
	}
	for _, want := range []string{"Urban Fantasy", "Mythology", "Dragons", "Epic Fantasy"} {
		if !byName[want] {
			t.Errorf("%q is not in the related genres, so the right box was not found", want)
		}
	}
	// The site nav's own genres. If these turn up, the query picked up the
	// navigation instead of the section.
	for _, notWanted := range []string{"Business", "Cookbooks", "Christian"} {
		if byName[notWanted] {
			t.Errorf("%q is in the related genres, so this is the site nav and not the graph", notWanted)
		}
	}
}

// TestGenreBooksComeOutOfTheTooltip.
//
// The visible box is a cover and a link. Everything else the page knows about
// these books is in the escaped HTML inside a new Tip() call, so a card with a
// title and nothing else means the tooltip read stopped working.
func TestGenreBooksComeOutOfTheTooltip(t *testing.T) {
	g := genreFromCapture(t)

	if len(g.Books) < 20 {
		t.Fatalf("%d books, and the page features several shelves of them", len(g.Books))
	}
	var withAuthor, withRating, withYear, withDesc int
	for _, b := range g.Books {
		if b.Book.ID == "" || b.Title == "" {
			t.Errorf("book %+v has no id or no title", b)
		}
		if b.Via != "s5" {
			t.Errorf("book %q says it came from %q", b.Title, b.Via)
		}
		if b.ImageURL == "" {
			t.Errorf("book %q has no cover, which is the one thing the visible box does carry", b.Title)
		}
		if len(b.Contributors) > 0 {
			withAuthor++
		}
		if b.AverageRating != nil {
			withRating++
			if *b.AverageRating < 1 || *b.AverageRating > 5 {
				t.Errorf("book %q rated %v", b.Title, *b.AverageRating)
			}
		}
		if b.PublishedAt != "" {
			withYear++
		}
		if b.Description != "" {
			withDesc++
		}
	}
	half := len(g.Books) / 2
	if withAuthor < half {
		t.Errorf("only %d of %d books name an author", withAuthor, len(g.Books))
	}
	if withRating < half {
		t.Errorf("only %d of %d books carry a rating", withRating, len(g.Books))
	}
	if withYear < half {
		t.Errorf("only %d of %d books carry a year", withYear, len(g.Books))
	}
	if withDesc < half {
		t.Errorf("only %d of %d books carry a description", withDesc, len(g.Books))
	}
}

// TestGenreBooksAreDeduplicated. A book featured in both New Releases and Most
// Read This Week is one book, and the same id twice would double count it in
// anything cumulative.
func TestGenreBooksAreDeduplicated(t *testing.T) {
	g := genreFromCapture(t)

	seen := map[string]string{}
	for _, b := range g.Books {
		if prev, dup := seen[b.Book.ID]; dup {
			t.Errorf("book %s appears twice, as %q and %q", b.Book.ID, prev, b.Title)
		}
		seen[b.Book.ID] = b.Title
	}
}

// TestGenreSaysWhatItIsNot. There is no count of the genre's membership on this
// page, which is exactly why the caveat is unconditional.
func TestGenreSaysWhatItIsNot(t *testing.T) {
	g := genreFromCapture(t)

	var said bool
	for _, m := range g.Missed {
		if strings.Contains(m, "rotates") {
			said = true
		}
	}
	if !said {
		t.Errorf("nothing in missed says the featured books rotate: %v", g.Missed)
	}
}
