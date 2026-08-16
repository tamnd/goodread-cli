package goodread

import (
	"encoding/json"
	"strings"
)

// The search record, for both of the surfaces that answer a query.
//
// Two surfaces, one shape, on purpose. /book/auto_complete is allowed and
// answers with JSON; /search is disallowed and answers with markup. A caller
// that wants to render results should not have to branch on which one replied,
// so both land here and every hit says which surface it came from.
//
// They do not carry the same fields, and the record does not pretend they do.
// Suggest has the page count, the work id and the description; the search page
// has the published year, the edition count, the pagination and the totals.
// A field the answering surface did not carry is absent rather than zero, which
// is the same rule the rest of the model follows.

// Search types, as the site names them in search_type.
//
// Named rather than typed as strings at the call sites, because four of the
// five are readable only in the sense that they return an HTTP response.
const (
	SearchTypeBooks  = "books"
	SearchTypePeople = "people"
	SearchTypeLists  = "lists"
	SearchTypeGroups = "groups"
	SearchTypeQuotes = "quotes"
)

// SearchTabs is what the site offers and what each of them actually does when
// nobody is signed in.
//
// Measured on 2026-08-16 rather than assumed. books answers. people answers
// with zero rows, which is the site's answer and not a bug in this tool. lists,
// groups and quotes answer with an AWS WAF challenge, which is a block, and a
// block is reported rather than solved.
var SearchTabs = []struct {
	Type     string
	Label    string
	Readable bool
	Note     string
}{
	{SearchTypeBooks, "Books", true, ""},
	{SearchTypeGroups, "Groups", false, "the site answers this tab with a WAF challenge when nobody is signed in"},
	{SearchTypeQuotes, "Quotes", false, "the site answers this tab with a WAF challenge when nobody is signed in"},
	{SearchTypePeople, "People", true, "the site answers this tab with zero rows when nobody is signed in"},
	{SearchTypeLists, "Listopia", false, "the site answers this tab with a WAF challenge when nobody is signed in"},
}

// SearchTabInfo looks a tab up by its search_type.
func SearchTabInfo(searchType string) (string, bool, string) {
	for _, t := range SearchTabs {
		if t.Type == searchType {
			return t.Label, t.Readable, t.Note
		}
	}
	return searchType, false, "this tool has not measured this tab"
}

// SearchQuery is everything that goes into one search request.
//
// Field and Source are the site's own parameters, search[field] and
// search[source]. Field narrows a books search to title or author and is the
// difference between finding a book called Hunger and every book by an author
// whose name contains it.
type SearchQuery struct {
	Query string `json:"query"`

	// Type is the tab. Empty means books, which is what the site defaults to.
	Type string `json:"type,omitempty"`

	// Field is all, title or author.
	Field string `json:"field,omitempty"`

	// Source is goodreads. The site accepts others and they are not this tool's
	// business, but the parameter is carried because the tabs' own URLs set it.
	Source string `json:"source,omitempty"`

	Page int `json:"page,omitempty"`
}

// Norm fills the defaults the site applies when the parameters are missing.
func (q SearchQuery) Norm() SearchQuery {
	if q.Type == "" {
		q.Type = SearchTypeBooks
	}
	if q.Source == "" {
		q.Source = "goodreads"
	}
	if q.Page < 1 {
		q.Page = 1
	}
	return q
}

// SearchRecord is one query and what came back.
//
// The page metadata is on the record rather than thrown away, because a page of
// twenty hits out of eight hundred is a different fact from twenty hits out of
// twenty, and nothing downstream can tell the two apart from the rows alone.
type SearchRecord struct {
	Envelope

	Query      string `json:"query"`
	SearchType string `json:"search_type,omitempty"`
	Field      string `json:"field,omitempty"`
	Source     string `json:"source,omitempty"`

	WebURL string `json:"web_url,omitempty"`

	// Page is the first page read and PagesWalked is every page that was, so a
	// multi page walk says what it covered rather than looking like one page
	// with a suspiciously large number of rows.
	Page        int   `json:"page,omitempty"`
	PagesWalked []int `json:"pages_walked,omitempty"`

	// TotalResults is the site's own count and Approximate says how it phrased
	// it. The page prints "about 816", and a number the site itself hedges on
	// should not arrive downstream looking exact.
	TotalResults *int64 `json:"total_results,omitempty"`
	Approximate  bool   `json:"approximate,omitempty"`

	// Elapsed is the site's search time in seconds, off the same line as the
	// total. Its own number, not a measurement of this tool.
	Elapsed *float64 `json:"elapsed,omitempty"`

	// Showing is the range the title line prints, "showing 1-20 of 816 books".
	Showing *ShowRange `json:"showing,omitempty"`

	// QID ties every rank on this page to the run that produced it. Ranks from
	// two different qids are not comparable, and without it nothing downstream
	// could know that.
	QID string `json:"qid,omitempty"`

	// Tabs is the other searches the page offers, with this tool's measured
	// verdict on each. A tab that answers with a challenge is marked unreadable
	// here rather than being left for a user to discover by spending a request.
	Tabs []SearchTab `json:"tabs,omitempty"`

	// Genre is the genre the query mapped to, when the page offers one. It is
	// the site's own reading of the query and it exists on no other surface.
	Genre *Ref `json:"genre,omitempty"`

	RelatedShelves []ShelfCount `json:"related_shelves,omitempty"`

	NextPage string `json:"next_page,omitempty"`
	LastPage *int   `json:"last_page,omitempty"`

	Results []SearchHit `json:"results,omitempty"`

	Extra map[string]json.RawMessage `json:"extra,omitempty"`
}

// ShowRange is the "1-20 of 816 books" line, split.
type ShowRange struct {
	From int64 `json:"from"`
	To   int64 `json:"to"`
	Of   int64 `json:"of"`

	// Kind is the noun the line uses, books or people, which is the only place
	// the page says what it counted.
	Kind string `json:"kind,omitempty"`
}

// SearchTab is one of the site's search tabs.
type SearchTab struct {
	Type     string `json:"type"`
	Label    string `json:"label,omitempty"`
	WebURL   string `json:"web_url,omitempty"`
	Selected bool   `json:"selected,omitempty"`

	// Readable is this tool's measured verdict and is never omitted, because
	// false is the value a caller most needs to see.
	Readable bool   `json:"readable"`
	Note     string `json:"note,omitempty"`
}

// ShelfCount is a shelf and how many books are on it site wide.
//
// The count is over the whole site and not over these results, which is worth
// saying because the block sits next to the results and reads like it is about
// them.
type ShelfCount struct {
	Shelf  Ref    `json:"shelf"`
	Name   string `json:"name"`
	WebURL string `json:"web_url,omitempty"`
	Count  *int64 `json:"count,omitempty"`
}

// SearchHit is one result row.
//
// BookCard is embedded rather than copied because a search row is a book as
// rendered in a row, which is exactly what BookCard already models, down to the
// work reference the editions link gives away. What is added here is what only
// a search result has: where it ranked, in which run, and behind which URL.
type SearchHit struct {
	BookCard

	// Rank is the site's, off the rank parameter on the title link, and falls
	// back to the row order when the parameter is missing. It is per page, so
	// rank 1 on page 2 is the twenty first result.
	Rank int `json:"rank,omitempty"`

	// Series is the series suffix in the title, parsed. The row never says the
	// series id, so the reference resolves to a name and nothing more, and it
	// is marked unresolved rather than looking like a real edge.
	Series *SeriesEntry `json:"series,omitempty"`

	WebURL string `json:"web_url,omitempty"`

	// TrackedURL is the link exactly as the page wrote it, with from_search,
	// from_srp, qid and rank still on it. WebURL is the same link with all four
	// removed. Both are kept: the clean one is the id and the tracked one is
	// what the page actually said, and reconstructing the page needs it. Only
	// the search page writes one; the suggest endpoint hands over a clean URL
	// and the same four facts as fields, which is what the three below are.
	TrackedURL string `json:"tracked_url,omitempty"`

	// QID is the row's own copy of the query session. It is the same value for
	// every row of one response, and it is per row here because that is where
	// the suggest endpoint puts it.
	QID string `json:"qid,omitempty"`

	// FromSearch and FromSRP are the endpoint's own flags, which is how the
	// site's front end tells its analytics where a click came from. Carried
	// because they are data the surface published, not because they mean much.
	FromSearch bool `json:"from_search,omitempty"`
	FromSRP    bool `json:"from_srp,omitempty"`

	// DescriptionHTML is the suggest endpoint's own markup, and Truncated is
	// its own admission that it cut the text off. BookCard.Description holds
	// the same thing with the tags taken out.
	DescriptionHTML      string `json:"description_html,omitempty"`
	DescriptionTruncated *bool  `json:"description_truncated,omitempty"`
	DescriptionURL       string `json:"description_url,omitempty"`

	// PreviewURL is the Kindle Cloud Reader preview, usually null.
	PreviewURL string `json:"preview_url,omitempty"`
}

// searchTrackingParams are the four the site hangs off a result link.
var searchTrackingParams = []string{"from_search", "from_srp", "qid", "rank"}

// untrack strips the search tracking from a result URL.
//
// The same book reached from search and reached directly has to be one thing,
// and it is not if half the URLs carry the query session that found them.
func untrack(u string) string {
	i := strings.IndexByte(u, '?')
	if i < 0 {
		return u
	}
	base, query := u[:i], u[i+1:]
	var keep []string
	for _, part := range strings.Split(query, "&") {
		name := part
		if j := strings.IndexByte(part, '='); j >= 0 {
			name = part[:j]
		}
		tracking := false
		for _, p := range searchTrackingParams {
			if name == p {
				tracking = true
				break
			}
		}
		if !tracking {
			keep = append(keep, part)
		}
	}
	if len(keep) == 0 {
		return base
	}
	return base + "?" + strings.Join(keep, "&")
}
