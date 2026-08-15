package goodread

import (
	"strings"
	"testing"
	"time"
)

const workQuotesPageURL = "https://www.goodreads.com/work/quotes/2792775-the-hunger-games"

func quotesFromCapture(t *testing.T) *Quotes {
	t.Helper()
	e, err := ExtractQuotes(readCapture(t, "work_quotes_2792775.html.gz"), workQuotesPageURL)
	if err != nil {
		t.Fatalf("ExtractQuotes: %v", err)
	}
	q, err := QuotesFrom(e, "2792775", time.Now())
	if err != nil {
		t.Fatalf("QuotesFrom: %v", err)
	}
	return q
}

// TestQuotesNamesItsSubject. Two URLs render this page and a file holding both
// kinds with no way to tell them apart is a file nobody can use.
func TestQuotesNamesItsSubject(t *testing.T) {
	q := quotesFromCapture(t)

	if q.Subject.Type != "Work" {
		t.Errorf("subject typed %q, and /work/quotes is a work", q.Subject.Type)
	}
	if q.Subject.ID != "2792775" {
		t.Errorf("subject id = %q", q.Subject.ID)
	}
	if q.Subject.Title != "The Hunger Games" {
		t.Errorf("subject title = %q, and the heading adds the word Quotes", q.Subject.Title)
	}
	if q.TotalCount == nil || *q.TotalCount != 1226 {
		t.Errorf("total = %v, want 1226", q.TotalCount)
	}
	if q.Complete {
		t.Error("thirty of 1,226 quotes and the record claims to be complete")
	}
}

// TestQuotesTextStopsAtTheAttribution.
//
// The quote, the author, the title and the tags are all inside the same block,
// so taking the block's text gives a quote with "Suzanne Collins, The Hunger
// Games" welded onto the end of it. That reads as part of the quote and there
// is nothing downstream that could notice.
func TestQuotesTextStopsAtTheAttribution(t *testing.T) {
	q := quotesFromCapture(t)

	if len(q.Quotes) != 30 {
		t.Fatalf("%d quotes, and a page holds 30", len(q.Quotes))
	}
	first := q.Quotes[0]
	if first.Text != "You don’t forget the face of the person who was your last hope." {
		t.Errorf("first quote text = %q", first.Text)
	}
	for _, quote := range q.Quotes {
		if quote.Text == "" {
			t.Error("a quote came out with no text at all")
		}
		if strings.Contains(quote.Text, "Suzanne Collins,") {
			t.Errorf("the attribution is inside the quote text: %q", quote.Text)
		}
		if strings.Contains(quote.Text, "likes") {
			t.Errorf("the footer is inside the quote text: %q", quote.Text)
		}
		if strings.HasPrefix(quote.Text, "“") || strings.HasSuffix(quote.Text, "”") {
			t.Errorf("the template's quote marks are still on it: %q", quote.Text)
		}
	}
}

// TestQuotesCarryTheirAttribution. The id is a permalink and the like count is
// the only popularity signal a quote has.
func TestQuotesCarryTheirAttribution(t *testing.T) {
	q := quotesFromCapture(t)

	first := q.Quotes[0]
	if first.ID != "170332" {
		t.Errorf("first quote id = %q", first.ID)
	}
	if first.WebURL == "" || !strings.Contains(first.WebURL, "/quotes/170332") {
		t.Errorf("web url = %q", first.WebURL)
	}
	if first.LikeCount == nil || *first.LikeCount != 19126 {
		t.Errorf("likes = %v, want 19126", first.LikeCount)
	}
	if first.Author == nil || first.Author.Title != "Suzanne Collins" {
		t.Errorf("author = %+v", first.Author)
	}
	if first.Book == nil || first.Book.ID != "2792775" {
		t.Errorf("book = %+v, and the attribution link is the work", first.Book)
	}
	if len(first.Tags) == 0 || first.Tags[0] != "the-hunger-games" {
		t.Errorf("tags = %v", first.Tags)
	}

	var withID, withLikes, withAuthor int
	for _, quote := range q.Quotes {
		if quote.ID != "" {
			withID++
		}
		if quote.LikeCount != nil {
			withLikes++
		}
		if quote.Author != nil {
			withAuthor++
		}
		if quote.Via != "s7" {
			t.Errorf("quote %s says it came from %q", quote.ID, quote.Via)
		}
	}
	if withID < 28 || withLikes < 28 || withAuthor < 28 {
		t.Errorf("%d ids, %d like counts, %d authors out of %d quotes", withID, withLikes, withAuthor, len(q.Quotes))
	}
}

// TestQuotesAreDistinct. The page renders its quotes in several div.quotes
// blocks with an ad between them, and a reader that walked the blocks wrongly
// could easily return the same one twice.
func TestQuotesAreDistinct(t *testing.T) {
	q := quotesFromCapture(t)

	seen := map[string]bool{}
	for _, quote := range q.Quotes {
		if seen[quote.Text] {
			t.Errorf("this quote came out twice: %q", quote.Text)
		}
		seen[quote.Text] = true
	}
}

func TestQuotesSaysWhatTheRestWouldCost(t *testing.T) {
	q := quotesFromCapture(t)

	var said bool
	for _, m := range q.Missed {
		if strings.Contains(m, "1226") && strings.Contains(m, "41") {
			said = true
		}
	}
	if !said {
		t.Errorf("missed does not name both the total and the page count: %v", q.Missed)
	}
}
