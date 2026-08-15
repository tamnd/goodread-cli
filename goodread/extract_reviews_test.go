package goodread

import (
	"strings"
	"testing"
	"time"
)

// TestReviewsComeOffTheBookPage is the whole argument for reading the cache
// instead of /book/reviews.
//
// robots.txt disallows /book/reviews. The book page carries the sample anyway,
// with rating, text, like count and the reader behind each one, and reading it
// there costs no extra request and breaks no rule.
func TestReviewsComeOffTheBookPage(t *testing.T) {
	b := bookFromCapture(t, "book_show_2767052.html.gz")

	if len(b.Reviews) != 30 {
		t.Fatalf("%d reviews, want the 30 the page carries", len(b.Reviews))
	}
	if b.Via["reviews"] == "" {
		t.Error("reviews has no via entry, so nothing says where they came from")
	}

	var withText, withRating, withUser, withLikes int
	for _, r := range b.Reviews {
		if r.ID == "" {
			t.Fatal("a review with no id, which nothing downstream could key on")
		}
		if r.Text != "" {
			withText++
		}
		if r.Rating != nil {
			withRating++
			if *r.Rating < 1 || *r.Rating > 5 {
				t.Errorf("review %s rated %d", r.ID, *r.Rating)
			}
		}
		if r.User != nil && r.User.Title != "" {
			withUser++
		}
		if r.LikeCount != nil && *r.LikeCount > 0 {
			withLikes++
		}
		if r.CreatedAt == nil {
			t.Errorf("review %s has no created date", r.ID)
		}
		if r.Via == "" {
			t.Errorf("review %s does not say which surface it came from", r.ID)
		}
	}
	if withText < 25 {
		t.Errorf("%d of 30 reviews carry text", withText)
	}
	if withRating < 25 {
		t.Errorf("%d of 30 reviews carry a rating", withRating)
	}
	if withUser < 25 {
		t.Errorf("%d of 30 reviews name their reader", withUser)
	}
	if withLikes == 0 {
		t.Error("no review carries a like count, which the cache does carry")
	}
}

// TestShelvingsAreNotReviews holds the distinction the model draws.
//
// A shelving is where a reader filed the book and what they tagged it. A review
// is what they wrote. The same person produces both and they are different
// facts, so flattening them would lose the tags entirely.
func TestShelvingsAreNotReviews(t *testing.T) {
	b := bookFromCapture(t, "book_show_2767052.html.gz")

	if len(b.Shelvings) == 0 {
		t.Fatal("no shelvings, and the page carries 30")
	}
	var named, tagged int
	for _, s := range b.Shelvings {
		if s.ShelfName != "" {
			named++
		}
		if len(s.Extra["tags"]) > 0 {
			tagged++
		}
	}
	if named == 0 {
		t.Error("no shelving carries a shelf name")
	}
	if tagged == 0 {
		t.Error("no shelving carries its tags, which is the part only this surface has")
	}
}

// TestReviewsSaySoWhenTheyAreASample. Returning 30 of 274,808 without saying so
// is the failure mode the missed block exists to prevent.
func TestReviewsSaySoWhenTheyAreASample(t *testing.T) {
	b := bookFromCapture(t, "book_show_2767052.html.gz")

	var said string
	for _, m := range b.Missed {
		if strings.Contains(m, "reviews") {
			said = m
		}
	}
	if said == "" {
		t.Fatal("30 reviews of a quarter million and nothing in missed says it is a sample")
	}
	if !strings.Contains(said, "30 of") {
		t.Errorf("the sentence does not give both numbers: %q", said)
	}
	if !strings.Contains(said, "--no-robots") {
		t.Errorf("the sentence does not name the flag that reads more: %q", said)
	}
}

// TestCommaInt covers the grouping used in those sentences, including the
// boundaries where an off by one puts the comma in the wrong place.
func TestCommaInt(t *testing.T) {
	for _, c := range []struct {
		in   int64
		want string
	}{
		{0, "0"},
		{1, "1"},
		{999, "999"},
		{1000, "1,000"},
		{10232418, "10,232,418"},
		{-1234, "-1,234"},
	} {
		if got := commaInt(c.in); got != c.want {
			t.Errorf("commaInt(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestParseReviewPageOnTheRealResponse reads the captured /book/reviews reply.
//
// The capture is a .js file and not a .html one, because that endpoint answers
// with a JavaScript call and not a document. A parser that assumed HTML found
// nothing at all and said so, which is how that got noticed.
func TestParseReviewPageOnTheRealResponse(t *testing.T) {
	body := readCapture(t, "book_reviews_2767052.js.gz")
	reviews, err := ParseReviewPage(body, "https://www.goodreads.com/book/reviews/2767052")
	if err != nil {
		t.Fatalf("ParseReviewPage: %v", err)
	}
	if len(reviews) != ReviewsPerPage {
		t.Fatalf("%d reviews, want the %d a page carries", len(reviews), ReviewsPerPage)
	}

	var withText, withRating, withUser, withShelves, withDate int
	for _, r := range reviews {
		if r.LegacyID == 0 {
			t.Errorf("review %q has no legacy id, and that is the only id this surface gives", r.ID)
		}
		if r.Via != "s12" {
			t.Errorf("review %s says it came from %q", r.ID, r.Via)
		}
		if r.Text != "" {
			withText++
		}
		if r.Rating != nil {
			withRating++
			if *r.Rating < 1 || *r.Rating > 5 {
				t.Errorf("review %s rated %d, and the stars are counted wrong", r.ID, *r.Rating)
			}
		}
		if r.User != nil && r.User.Title != "" && r.User.ID != "" {
			withUser++
		}
		if len(r.Extra["shelves"]) > 0 {
			withShelves++
		}
		if r.CreatedAt != nil {
			withDate++
		}
	}
	if withText < 25 {
		t.Errorf("%d of %d reviews carry text", withText, len(reviews))
	}
	if withRating < 25 {
		t.Errorf("%d of %d reviews carry a rating", withRating, len(reviews))
	}
	if withUser < 25 {
		t.Errorf("%d of %d reviews name their reader", withUser, len(reviews))
	}
	if withDate < 25 {
		t.Errorf("%d of %d reviews carry a date", withDate, len(reviews))
	}
	if withShelves == 0 {
		t.Error("no review carries the reader's shelves")
	}
}

// TestReviewFragmentRejectsThe406.
//
// Asking that endpoint for HTML gets a 406 with an empty body, which is content
// negotiation and not a block. An empty body has to be an error rather than
// zero reviews, since zero reviews is what ends the pagination walk.
func TestReviewFragmentRejectsThe406(t *testing.T) {
	if _, err := reviewFragment(nil); err == nil {
		t.Error("an empty body parsed as a review page")
	}
	if _, err := reviewFragment([]byte("not javascript and not html")); err == nil {
		t.Error("a body with no reviews in it parsed as a review page")
	}
}

// TestEstimateReviews holds the numbers the cost sentence is built from.
//
// The measured ceiling matters more than the total. 275,256 reviews at 30 a
// page is 9,175 pages by arithmetic, and Goodreads serves ten of them, so an
// estimate that quoted the arithmetic would be off by three orders of
// magnitude in the direction that scares people off.
func TestEstimateReviews(t *testing.T) {
	big := EstimateReviews(275256, 2*time.Second)
	if big.Pages != MaxReviewPages {
		t.Errorf("pages = %d, want the %d ceiling", big.Pages, MaxReviewPages)
	}
	if big.Reachable != int64(MaxReviewPages*ReviewsPerPage) {
		t.Errorf("reachable = %d", big.Reachable)
	}
	if big.Duration != 20*time.Second {
		t.Errorf("duration = %s", big.Duration)
	}

	// A book with 40 reviews is two pages and the estimate has to say two.
	small := EstimateReviews(40, 2*time.Second)
	if small.Pages != 2 {
		t.Errorf("pages = %d for 40 reviews, want 2", small.Pages)
	}
	if !strings.Contains(small.String(), "2 pages") {
		t.Errorf("the sentence does not say two pages: %q", small.String())
	}
}

// TestParseLikes covers the shapes that counter comes in.
func TestParseLikes(t *testing.T) {
	for _, c := range []struct {
		in   string
		want int64
	}{
		{"", 0},
		{"702 likes", 702},
		{"1,454 likes", 1454},
		{"1 like", 1},
		{"like", 0},
	} {
		if got := parseLikes(c.in); got != c.want {
			t.Errorf("parseLikes(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}
