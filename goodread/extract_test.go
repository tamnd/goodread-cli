package goodread

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func extractCapture(t *testing.T, name string) *Extractor {
	t.Helper()
	e, err := ExtractBook(readCapture(t, name))
	if err != nil {
		t.Fatalf("ExtractBook(%s): %v", name, err)
	}
	return e
}

// TestExtractBookAgainstTheRealPage pins the fields the measured page carries.
//
// Values are asserted where they cannot drift (isbn13, page count, the shape of
// the distribution) and only for presence where they can (rating counts move
// every hour).
func TestExtractBookAgainstTheRealPage(t *testing.T) {
	e := extractCapture(t, "book_show_2767052.html.gz")

	want := map[string]any{
		"title":          "The Hunger Games",
		"legacy_id":      2767052,
		"isbn13":         "9780439023481",
		"num_pages":      374,
		"publisher":      "Scholastic Press",
		"language":       "English",
		"format":         "Hardcover",
		"author":         "Suzanne Collins",
		"original_title": "The Hunger Games",
	}
	for field, w := range want {
		got, ok := e.Fields[field]
		if !ok {
			t.Errorf("no %s extracted", field)
			continue
		}
		if got != w {
			t.Errorf("%s = %v, want %v", field, got, w)
		}
	}

	for _, field := range []string{
		"id", "work_id", "work_legacy_id", "build_id", "description", "description_html",
		"image_url", "web_url", "canonical_url", "average_rating", "ratings_count",
		"text_reviews_count", "ratings_dist", "genres", "contributors", "published_at_ms",
	} {
		if _, ok := e.Fields[field]; !ok {
			t.Errorf("no %s extracted", field)
		}
	}
}

// TestRatingsDistributionIsTheWholeShape is the field v0.2.0 could not get,
// because it is drawn as a bar chart and never written out.
func TestRatingsDistributionIsTheWholeShape(t *testing.T) {
	e := extractCapture(t, "book_show_2767052.html.gz")
	dist, ok := e.Fields["ratings_dist"].([]int)
	if !ok {
		t.Fatalf("ratings_dist is %T, want []int", e.Fields["ratings_dist"])
	}
	if len(dist) != 5 {
		t.Fatalf("ratings_dist has %d buckets, want 5", len(dist))
	}
	sum := 0
	for _, n := range dist {
		if n < 0 {
			t.Errorf("negative bucket in %v", dist)
		}
		sum += n
	}
	total, ok := e.Fields["ratings_count"].(int)
	if !ok {
		t.Fatal("no ratings_count")
	}
	if sum != total {
		t.Errorf("distribution sums to %d, ratings_count is %d", sum, total)
	}

	// One through five in order, so the mean derived from the buckets has to
	// match the mean the site publishes. This is what catches a reversed slice,
	// which no amount of eyeballing a bar chart would.
	weighted := 0
	for i, n := range dist {
		weighted += (i + 1) * n
	}
	mean := float64(weighted) / float64(total)
	published, ok := e.Fields["average_rating"].(float64)
	if !ok {
		t.Fatalf("average_rating is %T, want float64", e.Fields["average_rating"])
	}
	if math.Abs(mean-published) > 0.01 {
		t.Errorf("mean from the buckets is %.4f, published average is %.2f: the buckets may be reversed", mean, published)
	}
}

// TestContributorsKeepTheirRoles holds the thing v0.2.0 flattened away.
func TestContributorsKeepTheirRoles(t *testing.T) {
	e := extractCapture(t, "book_show_2767052.html.gz")
	raw, err := json.Marshal(e.Fields["contributors"])
	if err != nil {
		t.Fatalf("marshal contributors: %v", err)
	}
	var got []struct {
		Role string `json:"role"`
		Name string `json:"name"`
		ID   string `json:"id"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode contributors: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("no contributors")
	}
	for _, c := range got {
		if c.Role == "" {
			t.Errorf("contributor %q has no role, which is the whole point of keeping the edges", c.Name)
		}
		if c.ID == "" {
			t.Errorf("contributor %q has no id, so nothing can follow the edge", c.Name)
		}
	}
	if got[0].Name != "Suzanne Collins" || got[0].Role != "Author" {
		t.Errorf("primary contributor = %+v, want Suzanne Collins as Author", got[0])
	}
}

// TestLevelsAreRecorded asserts the report is generated and not maintained.
func TestLevelsAreRecorded(t *testing.T) {
	e := extractCapture(t, "book_show_2767052.html.gz")
	if len(e.Levels) != len(e.Fields) {
		t.Errorf("%d fields but %d levels recorded, so something bypassed set", len(e.Fields), len(e.Levels))
	}
	if n := e.Levels.Count(LevelNextData); n < 20 {
		t.Errorf("only %d fields came from the cache, want most of them", n)
	}
	if n := e.Levels.Count(LevelSelector); n != 0 {
		t.Errorf("%d fields on the book page still come from selectors: %v", n, e.Levels.Fields(LevelSelector))
	}
	t.Log(e.Levels.Summary())
}

// TestLevelsAgreeAcrossTheLadder is the cross check level 2 exists for.
//
// When ld+json and the Apollo cache disagree on a field both carry, one of them
// is being read wrong. Finding that out from a test beats finding it out from a
// record six months later.
func TestLevelsAgreeAcrossTheLadder(t *testing.T) {
	for _, name := range []string{"book_show_2767052.html.gz", "book_show_1885.html.gz"} {
		body := readCapture(t, name)
		one, err := ExtractBook(body)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		two := LevelTwo(body)

		compared := 0
		for _, field := range []string{"title", "author", "image_url", "isbn13", "num_pages", "format", "language"} {
			a, okA := one.Fields[field]
			b, okB := two.Fields[field]
			if !okA || !okB {
				continue
			}
			compared++
			if a != b {
				t.Errorf("%s: %s is %v from the cache and %v from ld+json", name, field, a, b)
			}
		}

		// Ratings move between the two blocks being rendered, so they get a
		// tolerance rather than an equality. A one percent gap is the page
		// being a moment out of date; a large one is a parsing error.
		for _, field := range []string{"ratings_count", "text_reviews_count"} {
			a, okA := one.Fields[field].(int)
			b, okB := two.Fields[field].(int)
			if !okA || !okB || b == 0 {
				continue
			}
			compared++
			if delta := math.Abs(float64(a-b)) / float64(b); delta > 0.01 {
				t.Errorf("%s: %s is %d from the cache and %d from ld+json, %.1f%% apart", name, field, a, b, delta*100)
			}
		}
		if compared < 5 {
			t.Errorf("%s: only %d fields were comparable, so this proved little", name, compared)
		}
	}
}

// TestMissedIsGeneratedFromTheKey holds the rule that a missed sentence says a
// number that came from the page rather than one somebody typed.
func TestMissedIsGeneratedFromTheKey(t *testing.T) {
	e := extractCapture(t, "book_show_2767052.html.gz")
	if len(e.Missed) == 0 {
		t.Fatal("the book page samples quotes and reviews and said nothing about it")
	}
	var quotes string
	for _, m := range e.Missed {
		if strings.Contains(m, "quote") {
			quotes = m
		}
	}
	if quotes == "" {
		t.Fatal("no missed sentence about quotes")
	}
	if !strings.Contains(quotes, "goodread quotes") {
		t.Errorf("the quotes miss does not name the command that would fix it: %q", quotes)
	}
	if !strings.Contains(quotes, "2792775") {
		t.Errorf("the quotes miss does not carry the work id, so the command it names is not runnable: %q", quotes)
	}
	t.Log(strings.Join(e.Missed, "\n"))
}

// TestExtractSecondBookPage runs the extractor over a different book so it is
// not fitted to one page.
func TestExtractSecondBookPage(t *testing.T) {
	e := extractCapture(t, "book_show_1885.html.gz")
	for _, field := range []string{"title", "author", "legacy_id", "average_rating", "ratings_count", "ratings_dist"} {
		if _, ok := e.Fields[field]; !ok {
			t.Errorf("no %s on the second book page", field)
		}
	}
	if title := e.Fields["title"]; title != "Pride and Prejudice" {
		t.Errorf("title = %v, want Pride and Prejudice", title)
	}
}

func TestTitleWithoutSeries(t *testing.T) {
	cases := map[string]string{
		"The Hunger Games (The Hunger Games, #1)": "The Hunger Games",
		"Pride and Prejudice":                     "Pride and Prejudice",
		"Something (unfinished":                   "Something (unfinished",
	}
	for in, want := range cases {
		if got := TitleWithoutSeries(in); got != want {
			t.Errorf("TitleWithoutSeries(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOpenGraphOnEveryCapture(t *testing.T) {
	// og: tags are the only structured data the Rails pages carry, so this is
	// not a nicety for them, it is the level 2 that keeps those surfaces from
	// being selectors all the way down.
	for _, c := range loadCaptures(t) {
		og := parseOpenGraph(readCapture(t, c.File))
		if len(og) == 0 {
			t.Logf("%s (%s) carries no og: tags", c.File, c.Surface)
			continue
		}
		if og["url"] == "" && og["title"] == "" {
			t.Errorf("%s has og: tags but neither url nor title", c.File)
		}
	}
}

// TestNoDirectFieldWrites is the lint that keeps the report honest.
//
// Every field write goes through Extractor.set. A field assigned straight to
// the map is invisible to the level accounting, and a report that silently
// under-counts is worse than no report, because it reads as good news.
func TestNoDirectFieldWrites(t *testing.T) {
	files, err := filepath.Glob("extract*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		fn := ""
		for i, line := range strings.Split(string(src), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(line, "func ") {
				fn = line
			}
			if strings.HasPrefix(trimmed, "//") {
				continue
			}
			// set is the one place allowed to touch the maps, since it is the
			// thing everything else has to go through.
			if strings.Contains(fn, ") set(") {
				continue
			}
			// e.Fields[...] = and e.Levels[...] = are the two ways round set.
			for _, bad := range []string{"e.Fields[", "e.Levels[", ".Fields[", ".Levels["} {
				at := strings.Index(trimmed, bad)
				if at < 0 {
					continue
				}
				rest := trimmed[at+len(bad):]
				close := strings.IndexByte(rest, ']')
				if close < 0 {
					continue
				}
				after := strings.TrimSpace(rest[close+1:])
				if strings.HasPrefix(after, "=") && !strings.HasPrefix(after, "==") {
					t.Errorf("%s:%d writes a field directly, bypassing set: %s", f, i+1, trimmed)
				}
			}
		}
	}
}
