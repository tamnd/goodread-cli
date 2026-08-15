package goodread

import (
	"strings"
	"testing"
	"time"
)

func authorFromCapture(t *testing.T, name, pageURL, id string) *Author {
	t.Helper()
	e, err := ExtractAuthor(readCapture(t, name), pageURL)
	if err != nil {
		t.Fatalf("ExtractAuthor: %v", err)
	}
	a, err := AuthorFrom(e, id, time.Now())
	if err != nil {
		t.Fatalf("AuthorFrom: %v", err)
	}
	return a
}

const rowlingURL = "https://www.goodreads.com/author/show/1077326.J_K_Rowling"

// TestAuthorReadsThePersonalFields covers what only this surface has.
//
// A book page names an author and gives you a Contributor. This page is the
// only place the biography, the birth date, the influences and the aggregate
// over everything they wrote exist at all.
func TestAuthorReadsThePersonalFields(t *testing.T) {
	a := authorFromCapture(t, "author_show_1077326.html.gz", rowlingURL, "1077326")

	if a.Name != "J.K. Rowling" {
		t.Errorf("name = %q", a.Name)
	}
	if a.LegacyID != 1077326 {
		t.Errorf("legacy id = %d", a.LegacyID)
	}
	if a.BornAt != "July 31, 1965" {
		t.Errorf("born at = %q", a.BornAt)
	}
	if !strings.Contains(a.BornIn, "Yate") {
		t.Errorf("born in = %q, and the page says Yate", a.BornIn)
	}
	if a.Website != "http://www.jkrowling.com" {
		t.Errorf("website = %q", a.Website)
	}
	if a.Twitter != "jk_rowling" {
		t.Errorf("twitter = %q", a.Twitter)
	}
	if a.ImageURL == "" {
		t.Error("no image url")
	}

	// The full biography, not the truncated one. Every long text on these pages
	// is rendered twice and the visible copy stops mid sentence.
	if len(a.Bio) < 3000 {
		t.Errorf("bio is %d characters, which is the truncated copy and not the whole one", len(a.Bio))
	}
	if a.BioHTML == "" || !strings.Contains(a.BioHTML, "<") {
		t.Error("no bio html, and the links in it are pen names worth keeping")
	}
}

// TestAuthorGenresAndInfluences. Both are label and value sibling divs with no
// microdata, and both render twice, so both are places to get a duplicate.
func TestAuthorGenresAndInfluences(t *testing.T) {
	a := authorFromCapture(t, "author_show_1077326.html.gz", rowlingURL, "1077326")

	if len(a.Genres) == 0 {
		t.Fatal("no genres")
	}
	for _, g := range a.Genres {
		if g.Name == "" || g.WebURL == "" {
			t.Errorf("genre %+v is missing half of itself", g)
		}
	}
	if len(a.Influences) == 0 {
		t.Fatal("no influences, and the page lists four")
	}
	seen := map[string]bool{}
	for _, inf := range a.Influences {
		if inf.ID == "" || inf.Title == "" {
			t.Errorf("influence %+v is missing its id or its name", inf)
		}
		if seen[inf.ID] {
			t.Errorf("influence %s appears twice, which is the truncated copy leaking in", inf.Title)
		}
		seen[inf.ID] = true
	}
}

// TestAuthorStatsAreOverTheirBooks.
//
// This aggregate is over everything the author wrote, which is a different
// population from any one book's stats. The distribution is not on this page,
// so it is absent rather than flat.
func TestAuthorStatsAreOverTheirBooks(t *testing.T) {
	a := authorFromCapture(t, "author_show_1077326.html.gz", rowlingURL, "1077326")

	if a.Stats == nil {
		t.Fatal("no stats, and the page carries an aggregateRating")
	}
	if a.Stats.AverageRating == nil || *a.Stats.AverageRating < 3 || *a.Stats.AverageRating > 5 {
		t.Errorf("average rating = %v", a.Stats.AverageRating)
	}
	if a.Stats.RatingsCount == nil || *a.Stats.RatingsCount < 1_000_000 {
		t.Errorf("ratings count = %v, and this author has forty million", a.Stats.RatingsCount)
	}
	if len(a.Stats.RatingsCountDist) != 0 {
		t.Errorf("a distribution appeared from somewhere: %v", a.Stats.RatingsCountDist)
	}
	if a.FollowersCount == nil || *a.FollowersCount < 100_000 {
		t.Errorf("followers = %v", a.FollowersCount)
	}
}

// TestAuthorWorksSaysItIsASample. Ten rows against 665 distinct works, and a
// record that did not say so would look complete.
func TestAuthorWorksSaysItIsASample(t *testing.T) {
	a := authorFromCapture(t, "author_show_1077326.html.gz", rowlingURL, "1077326")

	if a.Works == nil {
		t.Fatal("no works connection")
	}
	if a.Works.TotalCount == nil || *a.Works.TotalCount != 665 {
		t.Errorf("total works = %v, want 665", a.Works.TotalCount)
	}
	if a.Works.Loaded != len(a.Books) || a.Works.Loaded == 0 {
		t.Errorf("loaded = %d against %d books", a.Works.Loaded, len(a.Books))
	}
	if a.Works.Complete {
		t.Error("the connection claims to be complete with ten of 665")
	}
	var said bool
	for _, m := range a.Missed {
		if strings.Contains(m, "665") {
			said = true
		}
	}
	if !said {
		t.Errorf("nothing in missed says this is a sample: %v", a.Missed)
	}
}

// TestAuthorBookRowsCarryTheirWork. The editions link on each row is a free
// book to work mapping, which normally costs a request.
func TestAuthorBookRowsCarryTheirWork(t *testing.T) {
	a := authorFromCapture(t, "author_show_1077326.html.gz", rowlingURL, "1077326")

	if len(a.Books) != 10 {
		t.Fatalf("%d book rows, want the 10 the page renders", len(a.Books))
	}
	var withWork, withRating, withEditions, withYear, withRole int
	for _, b := range a.Books {
		if b.Book.ID == "" || b.Title == "" {
			t.Errorf("row %+v has no id or no title", b)
		}
		if b.Via != "s2" {
			t.Errorf("row %q says it came from %q", b.Title, b.Via)
		}
		if b.TitleBare == b.Title && strings.Contains(b.Title, "(") {
			t.Errorf("row %q kept its series suffix in the bare title", b.Title)
		}
		if b.Work != nil {
			withWork++
		}
		if b.AverageRating != nil && b.RatingsCount != nil {
			withRating++
		}
		if b.EditionsCount != nil {
			withEditions++
		}
		if b.PublishedAt != "" {
			withYear++
		}
		for _, c := range b.Contributors {
			if c.Role == "Illustrator" {
				withRole++
			}
		}
	}
	if withWork < 8 {
		t.Errorf("%d of %d rows carry a work id", withWork, len(a.Books))
	}
	if withRating < 8 {
		t.Errorf("%d of %d rows carry a rating and a count", withRating, len(a.Books))
	}
	if withEditions < 8 {
		t.Errorf("%d of %d rows carry an edition count", withEditions, len(a.Books))
	}
	if withYear < 8 {
		t.Errorf("%d of %d rows carry a publication year", withYear, len(a.Books))
	}
	// The roles are the reason contributors are not a list of names. An
	// illustrator recorded as an author is a wrong fact nothing downstream can
	// detect.
	if withRole == 0 {
		t.Error("no row names an illustrator, and several of these have one")
	}
}

// TestAuthorLevels holds the honest claim about this surface: no level 1 at
// all, the counts from microdata, and everything personal from selectors.
func TestAuthorLevels(t *testing.T) {
	e, err := ExtractAuthor(readCapture(t, "author_show_1077326.html.gz"), rowlingURL)
	if err != nil {
		t.Fatalf("ExtractAuthor: %v", err)
	}
	if n := e.Levels.Count(LevelNextData); n != 0 {
		t.Errorf("%d fields claim level 1 on a Rails page: %v", n, e.Levels.Fields(LevelNextData))
	}
	for _, f := range []string{"average_rating", "ratings_count", "name"} {
		if e.Levels[f] != LevelMeta {
			t.Errorf("%s came from level %d, want level 2", f, e.Levels[f])
		}
	}
	for _, f := range []string{"bio", "born_at", "author_books"} {
		if e.Levels[f] != LevelSelector {
			t.Errorf("%s came from level %d, want level 3", f, e.Levels[f])
		}
	}
}
