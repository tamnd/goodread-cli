package goodread

import (
	"testing"
	"time"
)

func seriesFromCapture(t *testing.T, name string) *Series {
	t.Helper()
	e, err := ExtractSeries(readCapture(t, name))
	if err != nil {
		t.Fatalf("ExtractSeries: %v", err)
	}
	s, err := SeriesFrom(e, "45175", time.Now())
	if err != nil {
		t.Fatalf("SeriesFrom: %v", err)
	}
	return s
}

// TestSeriesReadsEveryIsland is the bug this test exists to prevent.
//
// The page mounts one SeriesList per run of consecutive books, so Harry Potter
// arrives as an island of two and an island of eight. Reading only the first
// gives a two book Harry Potter with no error anywhere, which is the worst kind
// of wrong: complete looking and quietly short.
func TestSeriesReadsEveryIsland(t *testing.T) {
	s := seriesFromCapture(t, "series_45175.html.gz")

	if len(s.Books) != 10 {
		t.Fatalf("%d books, want the 10 the page renders", len(s.Books))
	}
	if s.Title != "Harry Potter" {
		t.Errorf("title = %q", s.Title)
	}
	if s.LegacyID != 45175 {
		t.Errorf("legacy id = %d", s.LegacyID)
	}
	if s.Description == "" {
		t.Error("no description, and the header island carries one")
	}
}

// TestSeriesKeepsBothCounts. Seven primary works and nine total works are
// different facts, and a series with one number has the wrong length in
// everything built on it.
func TestSeriesKeepsBothCounts(t *testing.T) {
	s := seriesFromCapture(t, "series_45175.html.gz")

	if s.PrimaryWorkCount == nil || *s.PrimaryWorkCount != 7 {
		t.Errorf("primary work count = %v, want 7", s.PrimaryWorkCount)
	}
	if s.TotalWorkCount == nil || *s.TotalWorkCount != 9 {
		t.Errorf("total work count = %v, want 9", s.TotalWorkCount)
	}
	// Ten books rendered against nine total works is not a contradiction: the
	// count is works and the list is books, and one work here has two of them.
	// Worth pinning so nobody "fixes" the counts to agree with the list.
	if len(s.Books) < int(*s.TotalWorkCount) {
		t.Errorf("%d books for %d works", len(s.Books), *s.TotalWorkCount)
	}
}

// TestSeriesPositions covers the whole numbers, the fractional ones, and what
// happens to a position that is not a number at all.
func TestSeriesPositions(t *testing.T) {
	s := seriesFromCapture(t, "series_45175.html.gz")

	byPos := map[string]SeriesBookCard{}
	for _, b := range s.Books {
		if b.Position == "" {
			t.Errorf("book %q has no position", b.Title)
		}
		byPos[b.Position] = b
	}
	half, ok := byPos["Book 0.5"]
	if !ok {
		t.Fatalf("no Book 0.5, and the prequel is one. positions: %v", byPos)
	}
	if half.Number == nil || *half.Number != 0.5 {
		t.Errorf("Book 0.5 parsed to %v", half.Number)
	}
	first, ok := byPos["Book 1"]
	if !ok {
		t.Fatal("no Book 1")
	}
	if first.Number == nil || *first.Number != 1 {
		t.Errorf("Book 1 parsed to %v", first.Number)
	}

	// A range has a position and no number, which is the correct outcome and
	// not a parse failure. An omnibus of books one through three sits at no
	// single point in the sequence.
	if n := seriesNumber("Books 1-3"); n != nil {
		t.Errorf("Books 1-3 parsed to %v, and it should not parse at all", *n)
	}
	if n := seriesNumber("Book 2"); n == nil || *n != 2 {
		t.Errorf("Book 2 parsed to %v", n)
	}
}

// TestSeriesBooksCarryTheirWork is the traversal that makes this surface worth
// reading. Getting from an edition to its work normally costs a request, and
// the series page hands it over.
func TestSeriesBooksCarryTheirWork(t *testing.T) {
	s := seriesFromCapture(t, "series_45175.html.gz")

	var withWork, withRating, withAuthor, withPages int
	for _, b := range s.Books {
		if b.Book.ID == "" {
			t.Errorf("book %q has no id", b.Title)
		}
		if b.Via != "s3" {
			t.Errorf("book %q says it came from %q", b.Title, b.Via)
		}
		if b.Work != nil && b.Work.ID != "" {
			withWork++
		}
		if b.AverageRating != nil {
			withRating++
			if *b.AverageRating < 1 || *b.AverageRating > 5 {
				t.Errorf("book %q rated %v", b.Title, *b.AverageRating)
			}
		}
		if len(b.Contributors) > 0 && b.Contributors[0].Name != "" {
			withAuthor++
		}
		if b.NumPages != nil {
			withPages++
		}
	}
	if withWork < 8 {
		t.Errorf("%d of %d books carry their work id", withWork, len(s.Books))
	}
	if withRating < 8 {
		t.Errorf("%d of %d books carry a rating", withRating, len(s.Books))
	}
	if withAuthor < 8 {
		t.Errorf("%d of %d books name an author", withAuthor, len(s.Books))
	}
	if withPages < 5 {
		t.Errorf("%d of %d books carry a page count", withPages, len(s.Books))
	}
}

// TestSeriesIsLevelOne. The page has no __NEXT_DATA__ and it is still level 1,
// because the React island props are the state the page rendered itself from.
func TestSeriesIsLevelOne(t *testing.T) {
	body := readCapture(t, "series_45175.html.gz")
	if _, err := nextData(body); err == nil {
		t.Fatal("this page has __NEXT_DATA__, which changes the whole argument here")
	}
	e, err := ExtractSeries(body)
	if err != nil {
		t.Fatalf("ExtractSeries: %v", err)
	}
	if got := e.Levels["series_books"]; got != LevelNextData {
		t.Errorf("series_books came from level %d, want level 1", got)
	}
	if e.Levels.Count(LevelSelector) != 0 {
		t.Errorf("this surface needed %d selectors: %v",
			e.Levels.Count(LevelSelector), e.Levels.Fields(LevelSelector))
	}
}

func TestParseSeriesSubtitle(t *testing.T) {
	for _, c := range []struct {
		in             string
		primary, total int64
	}{
		{"7 primary works • 9 total works", 7, 9},
		{"1 primary work • 1 total work", 1, 1},
		{"1,024 primary works • 2,048 total works", 1024, 2048},
		{"", 0, 0},
		{"9 total works", 0, 0},
	} {
		p, tot := parseSeriesSubtitle(c.in)
		if p != c.primary || tot != c.total {
			t.Errorf("parseSeriesSubtitle(%q) = %d, %d, want %d, %d", c.in, p, tot, c.primary, c.total)
		}
	}
}
