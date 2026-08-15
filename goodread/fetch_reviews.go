package goodread

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// The full review set, behind --no-robots.
//
// Worth knowing before turning it on, and worth correcting in the spec:
// /book/reviews paginates to ten pages of thirty and stops. Page 11 of The
// Hunger Games returns 200 with no reviews in it, so the reachable set is about
// 300 of 275,256, not the corpus. Ten requests, not the nine thousand the spec
// estimated. The estimate is still printed, because ten requests at the two
// second floor is twenty seconds of somebody else's bandwidth.
//
// The page is not HTML. It answers with a JavaScript one liner,
// Element.update("reviews", "<escaped html>"), which is why a plain goquery
// parse of the response finds nothing at all.

// MaxReviewPages is where Goodreads stops paginating, measured rather than
// assumed. Asking for more returns a page with no reviews on it.
const MaxReviewPages = 10

// ReviewsPerPage is what one page of /book/reviews carries.
const ReviewsPerPage = 30

var reElementUpdate = regexp.MustCompile(`(?s)^Element\.update\("reviews",\s*(".*")\)\s*;?\s*$`)

// GetReviewPages walks /book/reviews and returns what it finds.
//
// Disallowed, so the caller needs --no-robots and the client refuses without
// it. Pages are requested in order and the walk stops as soon as one comes back
// empty, which is how the ten page ceiling is discovered rather than assumed.
func (c *Client) GetReviewPages(ctx context.Context, bookID string, maxPages int) ([]Review, error) {
	op, ok := LookupOp("reviews")
	if !ok {
		return nil, fmt.Errorf("no registered op for reviews")
	}
	if maxPages <= 0 || maxPages > MaxReviewPages {
		maxPages = MaxReviewPages
	}

	seen := map[string]bool{}
	var out []Review
	for page := 1; page <= maxPages; page++ {
		u := op.URL(bookID, strconv.Itoa(page))
		body, code, err := c.FetchAccept(ctx, u, AcceptAny)
		if err != nil {
			return out, err
		}
		if code == 404 {
			return out, ErrNotFound
		}
		reviews, err := ParseReviewPage(body, u)
		if err != nil {
			return out, err
		}
		if len(reviews) == 0 {
			break
		}
		fresh := 0
		for _, r := range reviews {
			if seen[r.ID] {
				continue
			}
			seen[r.ID] = true
			fresh++
			out = append(out, r)
		}
		// Goodreads repeats the featured review at the top of each page, so a
		// page of nothing new is the end just as much as an empty one is.
		if fresh == 0 {
			break
		}
	}
	return out, nil
}

// ParseReviewPage reads one /book/reviews response.
//
// Level 3, selectors, and not a rung down from something better: this surface
// has no __NEXT_DATA__ and no ld+json, so the markup is the only source there
// is. The ids are the legacy integers here and not the kca ids the book page
// cache carries, which is why the two sets are merged on nothing.
func ParseReviewPage(body []byte, pageURL string) ([]Review, error) {
	frag, err := reviewFragment(body)
	if err != nil {
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(frag))
	if err != nil {
		return nil, err
	}

	var out []Review
	doc.Find("div.review").Each(func(_ int, sel *goquery.Selection) {
		id, _ := sel.Attr("id")
		id = strings.TrimPrefix(id, "review_")
		if id == "" {
			return
		}
		r := Review{ID: id, Via: "s12"}
		if n, err := strconv.ParseInt(id, 10, 64); err == nil {
			r.LegacyID = n
		}

		// Each filled star is one span.staticStar.p10. An unrated review has
		// none of them and gets no rating, not a zero.
		if stars := sel.Find("span.staticStars span.staticStar.p10").Length(); stars > 0 {
			v := stars
			r.Rating = &v
		}

		// Two copies of the text: a truncated one for display and the whole
		// thing hidden behind it. The hidden one is the review.
		text := strings.TrimSpace(sel.Find("span[id^='freeText']:not([id^='freeTextContainer'])").First().Text())
		if text == "" {
			text = strings.TrimSpace(sel.Find("span.readable").First().Text())
		}
		r.Text = text

		if d := strings.TrimSpace(sel.Find("a.reviewDate").First().Text()); d != "" {
			if t, err := time.Parse("Jan 2, 2006", d); err == nil {
				r.CreatedAt = &t
			}
		}
		if n := parseLikes(sel.Find("span.likesCount").First().Text()); n > 0 {
			r.LikeCount = &n
		}

		user := sel.Find("a.user").First()
		if name, ok := user.Attr("name"); ok && name != "" {
			href, _ := user.Attr("href")
			uid := extractIDFromPath(href, "/user/show/")
			r.User = &Ref{Type: "User", ID: uid, Key: "User:" + uid, Title: name, Resolved: true}
		}

		if shelves := shelvesOf(sel); len(shelves) > 0 {
			if b, err := json.Marshal(shelves); err == nil {
				r.Extra = map[string]json.RawMessage{"shelves": b}
			}
		}
		out = append(out, r)
	})
	return out, nil
}

// reviewFragment unwraps the JavaScript the endpoint answers with.
//
// The argument is a JSON string literal, so the JSON decoder is the right tool
// for unescaping it rather than a hand rolled pass over the backslashes.
func reviewFragment(body []byte) (string, error) {
	s := strings.TrimSpace(string(body))
	m := reElementUpdate.FindStringSubmatch(s)
	if m == nil {
		// A plain HTML body is worth accepting rather than refusing, since the
		// same parse works on it and the endpoint has changed shape before.
		if strings.Contains(s, "<div") {
			return s, nil
		}
		return "", fmt.Errorf("this is not a /book/reviews response")
	}
	var frag string
	if err := json.Unmarshal([]byte(m[1]), &frag); err != nil {
		return "", fmt.Errorf("unwrap the review fragment: %w", err)
	}
	return frag, nil
}

// shelvesOf reads the reader's own shelves off a review block.
func shelvesOf(sel *goquery.Selection) []string {
	var out []string
	sel.Find("div.bookshelves a.actionLinkLite").Each(func(_ int, a *goquery.Selection) {
		if name := strings.TrimSpace(a.Text()); name != "" {
			out = append(out, name)
		}
	})
	return out
}

// parseLikes reads "702 likes" and the empty string alike.
func parseLikes(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, strings.Fields(s)[0])
	n, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// ReviewCost is what `goodread reviews --all` prints before it starts.
type ReviewCost struct {
	// Total is what Goodreads says exists.
	Total int64 `json:"total"`

	// Reachable is what pagination actually reaches, which is the number that
	// matters and the one nothing else tells you.
	Reachable int64 `json:"reachable"`

	Pages    int           `json:"pages"`
	Requests int           `json:"requests"`
	Duration time.Duration `json:"duration"`
}

// EstimateReviews works out the cost of reading the paginated set.
func EstimateReviews(total int64, pace time.Duration) ReviewCost {
	pages := MaxReviewPages
	if total > 0 {
		if n := int((total + ReviewsPerPage - 1) / ReviewsPerPage); n < pages {
			pages = n
		}
	}
	return ReviewCost{
		Total:     total,
		Reachable: int64(pages) * ReviewsPerPage,
		Pages:     pages,
		Requests:  pages,
		Duration:  time.Duration(pages) * pace,
	}
}

// String is the sentence the command prints.
func (c ReviewCost) String() string {
	return fmt.Sprintf(
		"%s reviews exist. /book/reviews paginates to %d pages of %d, so this reads about %s of them in %d requests, roughly %s.",
		commaInt(c.Total), c.Pages, ReviewsPerPage, commaInt(c.Reachable), c.Requests, c.Duration.Round(time.Second))
}
