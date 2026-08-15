package goodread

import (
	"strings"
	"testing"
	"time"
)

const editionsURL = "https://www.goodreads.com/work/editions/2792775-the-hunger-games"

func editionsFromCapture(t *testing.T) *Editions {
	t.Helper()
	e, err := ExtractEditions(readCapture(t, "work_editions_2792775.html.gz"), editionsURL)
	if err != nil {
		t.Fatalf("ExtractEditions: %v", err)
	}
	ed, err := EditionsFrom(e, "2792775", time.Now())
	if err != nil {
		t.Fatalf("EditionsFrom: %v", err)
	}
	return ed
}

func TestEditionsReadsItsHeader(t *testing.T) {
	ed := editionsFromCapture(t)

	if ed.Title != "The Hunger Games" {
		t.Errorf("title = %q", ed.Title)
	}
	if ed.Work.ID != "2792775" {
		t.Errorf("work id = %q", ed.Work.ID)
	}
	if ed.Work.Type != "Work" {
		t.Errorf("subject typed %q, and an editions page is keyed by work", ed.Work.Type)
	}
	if ed.TotalCount == nil || *ed.TotalCount != 527 {
		t.Errorf("total = %v, want 527", ed.TotalCount)
	}
	if ed.Page != 1 {
		t.Errorf("page = %d", ed.Page)
	}
	if ed.Complete {
		t.Error("ten of 527 editions and the record claims to be complete")
	}
}

// TestEditionsReadsEveryLabelledField. This is the most regular markup on the
// site and there is no excuse for dropping a field on it.
func TestEditionsReadsEveryLabelledField(t *testing.T) {
	ed := editionsFromCapture(t)

	if len(ed.Editions) != 10 {
		t.Fatalf("%d editions, and the page shows 10", len(ed.Editions))
	}
	first := ed.Editions[0]
	if first.Book.ID != "2767052" {
		t.Errorf("first edition book id = %q", first.Book.ID)
	}
	if first.Title != "The Hunger Games (The Hunger Games, #1)" {
		t.Errorf("title = %q", first.Title)
	}
	if first.Format != "Hardcover" {
		t.Errorf("format = %q", first.Format)
	}
	if first.NumPages == nil || *first.NumPages != 374 {
		t.Errorf("pages = %v, want 374", first.NumPages)
	}
	if first.Publisher != "Scholastic Press" {
		t.Errorf("publisher = %q", first.Publisher)
	}
	if first.PublishedAt != "October 14th 2008" {
		t.Errorf("published = %q, and the date is kept as written because a page prints it in a dozen shapes", first.PublishedAt)
	}
	if first.ISBN13 != "9780439023481" {
		t.Errorf("isbn13 = %q", first.ISBN13)
	}
	if first.ISBN != "0439023483" {
		t.Errorf("isbn10 = %q", first.ISBN)
	}
	if first.ASIN != "0439023483" {
		t.Errorf("asin = %q", first.ASIN)
	}
	if first.Language != "English" {
		t.Errorf("language = %q", first.Language)
	}
	if first.AverageRating == nil || *first.AverageRating != 4.35 {
		t.Errorf("average = %v, want 4.35", first.AverageRating)
	}
	if first.RatingsCount == nil || *first.RatingsCount != 9811629 {
		t.Errorf("ratings = %v, want 9811629", first.RatingsCount)
	}
	if len(first.Contributors) == 0 || first.Contributors[0].Name != "Suzanne Collins" {
		t.Errorf("contributors = %+v", first.Contributors)
	}
	if first.Contributors[0].ID != "153394" {
		t.Errorf("author id = %q", first.Contributors[0].ID)
	}
	if first.Via != "s6" {
		t.Errorf("via = %q", first.Via)
	}
}

// TestEditionsDiffer is the point of the surface. Ten rows that all agree on
// format and page count would mean the labelled fields are being read off the
// first row and copied.
func TestEditionsDiffer(t *testing.T) {
	ed := editionsFromCapture(t)

	formats, isbns, ids := map[string]bool{}, map[string]bool{}, map[string]bool{}
	var withFormat, withISBN, withLang int
	for _, e := range ed.Editions {
		if e.Book.ID == "" {
			t.Errorf("edition %q has no book id", e.Title)
		}
		if ids[e.Book.ID] {
			t.Errorf("book %s appears twice on the page", e.Book.ID)
		}
		ids[e.Book.ID] = true
		if e.Format != "" {
			formats[e.Format] = true
			withFormat++
		}
		if e.ISBN13 != "" || e.ISBN != "" {
			isbns[e.ISBN13+e.ISBN] = true
			withISBN++
		}
		if e.Language != "" {
			withLang++
		}
	}
	if len(formats) < 2 {
		t.Errorf("every edition has the same format %v, so the row read is not per row", formats)
	}
	if withISBN > 0 && len(isbns) != withISBN {
		t.Errorf("%d editions carry an ISBN but only %d distinct ones", withISBN, len(isbns))
	}
	if withFormat < 8 {
		t.Errorf("%d of %d editions carry a format", withFormat, len(ed.Editions))
	}
	if withLang < 8 {
		t.Errorf("%d of %d editions carry a language", withLang, len(ed.Editions))
	}
}

// TestEditionsSaysWhatTheRestWouldCost. 527 editions at ten a page is 53
// requests, and that is a decision to hand to the caller with the number in it.
func TestEditionsSaysWhatTheRestWouldCost(t *testing.T) {
	ed := editionsFromCapture(t)

	var said bool
	for _, m := range ed.Missed {
		if strings.Contains(m, "527") && strings.Contains(m, "53") {
			said = true
		}
	}
	if !said {
		t.Errorf("missed does not name both the total and the page count: %v", ed.Missed)
	}
}

// TestParseFormatLine. The designation and the format are two facts on one
// line, and reading the line from the left folds them into each other.
func TestParseFormatLine(t *testing.T) {
	for _, c := range []struct {
		in           string
		name, format string
		pages        int
	}{
		{"Hardcover, 374 pages", "", "Hardcover", 374},
		{"Reprint Edition, Kindle Edition, 387 pages", "Reprint Edition", "Kindle Edition", 387},
		{"Special Edition, Kindle Edition, 387 pages", "Special Edition", "Kindle Edition", 387},
		{"Reprint, Paperback, 374 pages", "Reprint", "Paperback", 374},
		{"Paperback", "", "Paperback", 0},
		{"1,024 pages", "", "", 1024},
		{"", "", "", 0},
	} {
		name, format, pages := parseFormatLine(c.in)
		got := 0
		if pages != nil {
			got = *pages
		}
		if name != c.name || format != c.format || got != c.pages {
			t.Errorf("parseFormatLine(%q) = %q, %q, %d, want %q, %q, %d",
				c.in, name, format, got, c.name, c.format, c.pages)
		}
	}
}

func TestPageCount(t *testing.T) {
	for _, c := range []struct {
		total   int64
		perPage int
		want    int
	}{
		{527, 10, 53},
		{520, 10, 52},
		{1, 10, 1},
		{0, 10, 0},
		{100, 0, 0},
	} {
		if got := pageCount(c.total, c.perPage); got != c.want {
			t.Errorf("pageCount(%d, %d) = %d, want %d", c.total, c.perPage, got, c.want)
		}
	}
}
