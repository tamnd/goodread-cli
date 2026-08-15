package goodread

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"
)

func bookFromCapture(t *testing.T, name string) *Book {
	t.Helper()
	e := extractCapture(t, name)
	b, err := BookFrom(e, time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("BookFrom(%s): %v", name, err)
	}
	return b
}

// TestBookFromTheRealPage checks the mapping end to end.
func TestBookFromTheRealPage(t *testing.T) {
	b := bookFromCapture(t, "book_show_2767052.html.gz")

	if b.Title != "The Hunger Games" {
		t.Errorf("title = %q", b.Title)
	}
	if b.LegacyID != 2767052 {
		t.Errorf("legacy_id = %d, want 2767052", b.LegacyID)
	}
	if !strings.HasPrefix(b.ID, "kca://book/") {
		t.Errorf("id = %q, want a kca book id", b.ID)
	}
	if b.ISBN13 != "9780439023481" {
		t.Errorf("isbn13 = %q", b.ISBN13)
	}
	if b.NumPages == nil || *b.NumPages != 374 {
		t.Errorf("num_pages = %v, want 374", b.NumPages)
	}
	if b.Kind != "book" {
		t.Errorf("kind = %q", b.Kind)
	}
	if len(b.Surfaces) != 1 || b.Surfaces[0] != "s1" {
		t.Errorf("surfaces = %v", b.Surfaces)
	}
	if b.BuildID == "" {
		t.Error("no build_id, so nothing can say which deploy this was read from")
	}

	// The two descriptions are two cache fields and the record keeps both.
	if b.Description == "" || b.DescriptionStripped == "" {
		t.Error("want both the markup and the stripped description")
	}
	if b.Description == b.DescriptionStripped {
		t.Error("the two descriptions are identical, so one of them is being read from the wrong key")
	}
	if strings.Contains(b.DescriptionStripped, "<br") {
		t.Error("the stripped description still carries markup")
	}

	if b.Work == nil || !strings.HasPrefix(b.Work.ID, "kca://work/") {
		t.Errorf("work ref = %+v", b.Work)
	}
	if len(b.Contributors) == 0 || b.Contributors[0].Role != "Author" {
		t.Errorf("contributors = %+v", b.Contributors)
	}
	if len(b.Genres) == 0 || b.Genres[0].WebURL == "" {
		t.Errorf("genres = %+v, want the slug carrying URL too", b.Genres)
	}
	if len(b.Links) == 0 {
		t.Error("no links block, which is where the ebook and audiobook availability lives")
	}
}

// TestSeriesPositionSurvives is the reason Position is a string.
func TestSeriesPositionSurvives(t *testing.T) {
	b := bookFromCapture(t, "book_show_2767052.html.gz")
	if len(b.Series) == 0 {
		t.Fatal("The Hunger Games is book one of a series and the record has no series")
	}
	s := b.Series[0]
	if s.Position != "1" {
		t.Errorf("position = %q, want 1", s.Position)
	}
	if s.Number == nil || *s.Number != 1 {
		t.Errorf("number = %v, want 1", s.Number)
	}
	if s.Series.ID == "" {
		t.Error("the series ref has no id, so nothing can follow it")
	}
}

// TestPositionParsing holds the shapes Goodreads actually uses.
func TestPositionParsing(t *testing.T) {
	cases := []struct {
		in   string
		want *float64
	}{
		{"1", ptr(1.0)},
		{"2.5", ptr(2.5)}, // novellas get half positions
		{"1-3", nil},      // omnibus editions get ranges, and that is not an error
		{"", nil},
		{"1 of 3", nil},
	}
	for _, c := range cases {
		got, ok := parsePosition(c.in)
		if (c.want == nil) == ok {
			t.Errorf("parsePosition(%q) ok = %v, want %v", c.in, ok, c.want != nil)
			continue
		}
		if c.want != nil && got != *c.want {
			t.Errorf("parsePosition(%q) = %v, want %v", c.in, got, *c.want)
		}
	}
}

func ptr[T any](v T) *T { return &v }

// TestPublicationTimeAcceptsNegatives is the case a naive conversion drops.
//
// Anything published before 1970 has a negative publicationTime, which is a
// large share of the classics on the site.
func TestPublicationTimeAcceptsNegatives(t *testing.T) {
	got := timeFromMillis(-2208988800000) // 1900-01-01
	if got == nil {
		t.Fatal("a pre-1970 publication time was dropped")
	}
	if got.Year() != 1900 {
		t.Errorf("year = %d, want 1900", got.Year())
	}

	b := bookFromCapture(t, "book_show_2767052.html.gz")
	if b.PublicationTime == nil {
		t.Fatal("no publication time")
	}
	if b.PublicationTime.Year() != 2008 {
		t.Errorf("publication year = %d, want 2008", b.PublicationTime.Year())
	}
}

// TestStatsCarryTheirPopulation is the field that stops two different
// populations being averaged together by accident.
func TestStatsCarryTheirPopulation(t *testing.T) {
	b := bookFromCapture(t, "book_show_2767052.html.gz")
	if b.Stats == nil {
		t.Fatal("no stats")
	}
	if b.Stats.Via == "" {
		t.Error("stats with no via, so nothing downstream can tell an edition's numbers from a work's")
	}

	// via is not omitempty, so it survives a round trip even when empty. A
	// consumer that reads the numbers without reading via is the thing this
	// guards against, and hiding the key would let it happen quietly.
	raw, err := json.Marshal(Stats{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"via"`) {
		t.Errorf("an empty Stats marshals to %s, with no via key", raw)
	}
}

// TestStatsCheckOnRealNumbers is what `goodread book <id> --check` runs.
func TestStatsCheckOnRealNumbers(t *testing.T) {
	for _, name := range []string{"book_show_2767052.html.gz", "book_show_1885.html.gz"} {
		b := bookFromCapture(t, name)
		c := b.Stats.Check()
		if !c.Checked {
			t.Fatalf("%s: nothing was checked", name)
		}
		if !c.OK {
			t.Errorf("%s: %s", name, strings.Join(c.Problems, "; "))
		}
		if math.Abs(c.DerivedMean-c.Published) > 0.02 {
			t.Errorf("%s: derived %.4f against published %.2f", name, c.DerivedMean, c.Published)
		}
	}
}

// TestStatsCheckCatchesAReversedDistribution is the failure the check exists
// for. Five integers with no labels are exactly the thing somebody reverses,
// and nothing about the values gives it away.
func TestStatsCheckCatchesAReversedDistribution(t *testing.T) {
	b := bookFromCapture(t, "book_show_2767052.html.gz")
	dist := b.Stats.RatingsCountDist
	reversed := make([]int64, len(dist))
	for i, n := range dist {
		reversed[len(dist)-1-i] = n
	}
	s := *b.Stats
	s.RatingsCountDist = reversed

	c := s.Check()
	if c.OK {
		t.Fatalf("a reversed distribution passed the check: derived %.4f against published %.2f", c.DerivedMean, c.Published)
	}
	if c.SumOfBuckets != *s.RatingsCount && relDrift(float64(c.SumOfBuckets), float64(*s.RatingsCount)) > 0.01 {
		t.Error("the sum changed under reversal, so this test proved the wrong thing")
	}
	if !strings.Contains(strings.Join(c.Problems, " "), "reversed") {
		t.Errorf("the check complained but not about the reversal: %v", c.Problems)
	}
}

// TestConnSaysWhenItIsIncomplete holds the field a consumer most needs to see.
func TestConnSaysWhenItIsIncomplete(t *testing.T) {
	raw, err := json.Marshal(Conn{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"complete":false`) {
		t.Errorf("an empty Conn marshals to %s, hiding the false that matters most", raw)
	}

	e := extractCapture(t, "book_show_2767052.html.gz")
	w, ok := WorkFrom(e, time.Now())
	if !ok {
		t.Fatal("no work off the book page")
	}
	if w.Editions == nil {
		t.Fatal("the book page names the editions page and the work does not carry it")
	}
	if w.Editions.Complete {
		t.Error("the editions connection says complete, and the book page loads no editions at all")
	}
	if w.Editions.NextURL == "" {
		t.Error("an incomplete connection with no next url is a dead end")
	}
}

// TestWorkCarriesTheCuratedLists holds the fields nothing else in the family
// has. Places and characters are why a graph can answer "which books are set in
// Dublin" instead of grepping descriptions for it.
func TestWorkCarriesTheCuratedLists(t *testing.T) {
	e := extractCapture(t, "book_show_2767052.html.gz")
	w, ok := WorkFrom(e, time.Now())
	if !ok {
		t.Fatal("no work")
	}
	if w.LegacyID != 2792775 {
		t.Errorf("work legacy id = %d, want 2792775", w.LegacyID)
	}
	if len(w.Places) == 0 {
		t.Error("no places on a work that has them")
	}
	if len(w.Characters) == 0 {
		t.Error("no characters")
	}
	if len(w.AwardsWon) == 0 {
		t.Fatal("no awards")
	}

	// awardsWon carries nominations as well as wins, and a nomination reported
	// as a win is a wrong claim about somebody's book.
	wins, noms := 0, 0
	for _, a := range w.AwardsWon {
		if a.Name == "" {
			t.Error("an award with no name")
		}
		if a.HasWon == nil {
			t.Errorf("award %q does not say whether it was won", a.Name)
			continue
		}
		if *a.HasWon {
			wins++
		} else {
			noms++
		}
	}
	if wins == 0 || noms == 0 {
		t.Errorf("%d wins and %d nominations, so the designation is not being read", wins, noms)
	}
}

// TestFourStatesOfAMissingField is one subtest per row of the table in
// ~/notes/Spec/3008/03_model.md section 8.
//
// Rows two and three are the ones that get conflated and they mean genuinely
// different things. A book with a blank publisher is a book Goodreads has no
// publisher for. A book with no publisher key at all, read at a depth that
// never fetched details, is a book nobody asked about.
func TestFourStatesOfAMissingField(t *testing.T) {
	t.Run("the surface was not read", func(t *testing.T) {
		b := bookFromCapture(t, "book_show_2767052.html.gz")
		// Quotes and not reviews. Reviews used to be the example here and are
		// now read from the page cache, which is the outcome this row is meant
		// to be the opposite of.
		if _, ok := b.Via["quotes"]; ok {
			t.Error("quotes has a via entry and the book page never read the quotes surface")
		}
		var said string
		for _, m := range b.Missed {
			if strings.Contains(m, "quote") {
				said = m
			}
		}
		if said == "" {
			t.Fatal("the quotes surface was not read and nothing in missed says so")
		}
		if !strings.Contains(said, "goodread quotes") {
			t.Errorf("the missed sentence does not name the command that would fix it: %q", said)
		}
	})

	t.Run("the surface was read and had no such field", func(t *testing.T) {
		e := NewExtractor("s1")
		e.set("title", LevelNextData, "A Book")
		b, err := BookFrom(e, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if b.Publisher != "" {
			t.Errorf("publisher = %q, want empty", b.Publisher)
		}
		if _, ok := b.Via["publisher"]; ok {
			t.Error("publisher has a via entry and the page never mentioned it")
		}
		if len(b.Surfaces) == 0 {
			t.Error("the surface that was read is not recorded, which is what tells this row from the one above")
		}
		raw, err := json.Marshal(b)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), `"publisher"`) {
			t.Errorf("an unmentioned publisher is in the JSON: %s", raw)
		}
	})

	t.Run("the surface published it as empty", func(t *testing.T) {
		e := NewExtractor("s1")
		e.set("title", LevelNextData, "A Book")
		e.set("publisher", LevelNextData, "")
		b, err := BookFrom(e, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if b.Publisher != "" {
			t.Errorf("publisher = %q, want empty", b.Publisher)
		}
		if b.Via["publisher"] != "s1" {
			t.Errorf("via[publisher] = %q, want s1: an empty value the page published is not the same as one it never mentioned", b.Via["publisher"])
		}
		if _, counted := b.Level["publisher"]; counted {
			t.Error("an empty publisher was counted as a field the ladder answered")
		}
	})

	t.Run("the surface published a value", func(t *testing.T) {
		b := bookFromCapture(t, "book_show_2767052.html.gz")
		if b.Publisher != "Scholastic Press" {
			t.Errorf("publisher = %q", b.Publisher)
		}
		if b.Via["publisher"] != "s1" {
			t.Errorf("via[publisher] = %q, want s1", b.Via["publisher"])
		}
		if b.Level["publisher"] != LevelNextData {
			t.Errorf("level[publisher] = %d, want %d", b.Level["publisher"], LevelNextData)
		}
	})
}

// TestEmptyListsAreRowThreeOnTheRealPage anchors the same rule to a capture.
//
// Pride and Prejudice has an empty places list in the cache and The Hunger
// Games has a full one, which is the two rows side by side on real data.
func TestEmptyListsAreRowThreeOnTheRealPage(t *testing.T) {
	plain := extractCapture(t, "book_show_1885.html.gz")
	if _, ok := plain.Fields["places"]; ok {
		t.Error("an empty places list was stored as a value")
	}
	if _, ok := plain.Empty["places"]; !ok {
		t.Error("an empty places list was not recorded as published and blank")
	}

	w, ok := WorkFrom(plain, time.Now())
	if !ok {
		t.Fatal("no work")
	}
	if len(w.Places) != 0 {
		t.Errorf("places = %v, want none", w.Places)
	}
	if w.Via["places"] != "s1" {
		t.Error("the page published an empty places list and the record does not say so")
	}
}

// TestRecordRoundTrips checks nothing in the model is unmarshalable, which is
// easy to break with a raw message or a pointer and never notice until a cache
// read fails in the field.
func TestRecordRoundTrips(t *testing.T) {
	b := bookFromCapture(t, "book_show_2767052.html.gz")
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	var back Book
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if back.Title != b.Title || back.LegacyID != b.LegacyID {
		t.Error("identity did not survive the round trip")
	}
	if back.NumPages == nil || *back.NumPages != *b.NumPages {
		t.Error("num_pages did not survive")
	}
	if back.Stats == nil || len(back.Stats.RatingsCountDist) != 5 {
		t.Error("the distribution did not survive")
	}
	if back.PublicationTime == nil || !back.PublicationTime.Equal(*b.PublicationTime) {
		t.Error("publication time did not survive")
	}
	if len(back.Contributors) != len(b.Contributors) {
		t.Error("contributors did not survive")
	}
}

// TestExtraKeepsWhatTheModelDoesNotKnow is the field that stops the extractor
// silently discarding whatever Goodreads ships next.
func TestExtraKeepsWhatTheModelDoesNotKnow(t *testing.T) {
	e := extractCapture(t, "book_show_2767052.html.gz")
	b, err := BookFrom(e, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Extra) == 0 {
		t.Fatal("the book entity carries fields the model has no home for and none of them were kept")
	}
	// __typename is on every entity, says nothing the cache key does not, and
	// keeping it would make every record look like it had an unmodelled field.
	if _, ok := b.Extra["__typename"]; ok {
		t.Error("__typename is in extra")
	}
	for field := range b.Extra {
		if bookKnown[ParseFieldKey(field).Name] {
			t.Errorf("%s is modelled and is also in extra", field)
		}
	}
	if len(e.Unknown) == 0 {
		t.Error("extra was filled and nothing was counted as unknown, so verify would not report it")
	}

	// Everything in extra has to be valid JSON, since it is handed on whole.
	for field, raw := range b.Extra {
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			t.Errorf("extra[%s] is not valid JSON: %v", field, err)
		}
	}

	w, ok := WorkFrom(e, time.Now())
	if !ok {
		t.Fatal("no work")
	}
	for field := range w.Extra {
		if workKnown[ParseFieldKey(field).Name] {
			t.Errorf("work field %s is modelled and is also in extra", field)
		}
	}
}

// TestSampleIDsAreDistinct guards the list verify --sample reads.
//
// A duplicate would quietly shrink the sample, and a non numeric id would make
// the URL nonsense, and neither would look wrong in the output.
func TestSampleIDsAreDistinct(t *testing.T) {
	seen := map[string]bool{}
	for _, id := range SampleIDs {
		if seen[id] {
			t.Errorf("%s is in the sample twice", id)
		}
		seen[id] = true
		if numericPrefix(id) == "" {
			t.Errorf("%q has no numeric id in it", id)
		}
	}
	if len(SampleIDs) < 5 {
		t.Errorf("%d ids is a thin sample", len(SampleIDs))
	}
}

func TestDepth(t *testing.T) {
	if d, ok := ParseDepth(""); !ok || d != DepthMeta {
		t.Errorf("the empty depth = %q, want meta", d)
	}
	if _, ok := ParseDepth("everything"); ok {
		t.Error("an unknown depth was accepted")
	}
	// quick and meta are the same request and differ only in how much of the
	// cache gets decoded, so they must cost the same.
	if DepthQuick.Requests() != DepthMeta.Requests() {
		t.Error("quick and meta are one request each and the costs disagree")
	}
	if DepthFull.Requests() <= DepthMeta.Requests() {
		t.Error("full reads two more pages and does not cost more")
	}
}
