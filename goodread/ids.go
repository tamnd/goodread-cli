package goodread

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// extractIDFromPath returns the id portion after a path prefix.
// e.g. "/author/show/153394-suzanne-collins" -> "153394".
func extractIDFromPath(href, prefix string) string {
	idx := strings.Index(href, prefix)
	if idx < 0 {
		return ""
	}
	rest := href[idx+len(prefix):]
	if i := strings.IndexAny(rest, "?#"); i >= 0 {
		rest = rest[:i]
	}
	// Numeric prefix up to the first separator; otherwise the whole token.
	for i, c := range rest {
		if c == '-' || c == '/' || c == '.' {
			return rest[:i]
		}
		if c < '0' || c > '9' {
			if i == 0 {
				return rest
			}
			return rest[:i]
		}
	}
	return rest
}

// ExtractID is the exported form of extractIDFromPath.
func ExtractID(rawurl, prefix string) string { return extractIDFromPath(rawurl, prefix) }

// Classify inspects any Goodreads URL or bare id and returns (entity, id).
// A bare numeric id is treated as a book. Returns ("","") when unrecognizable.
func Classify(s string) (entity, id string) {
	s = strings.TrimSpace(s)
	switch {
	case strings.Contains(s, "/book/show/"):
		return "book", extractIDFromPath(s, "/book/show/")
	case strings.Contains(s, "/author/show/"):
		return "author", extractIDFromPath(s, "/author/show/")
	case strings.Contains(s, "/series/"):
		return "series", extractIDFromPath(s, "/series/")
	case strings.Contains(s, "/list/show/"):
		return "list", extractIDFromPath(s, "/list/show/")
	case strings.Contains(s, "/user/show/"):
		return "user", extractIDFromPath(s, "/user/show/")
	case strings.Contains(s, "/quotes/"):
		return "quote", extractIDFromPath(s, "/quotes/")
	case strings.Contains(s, "/genres/"):
		return "genre", extractIDFromPath(s, "/genres/")
	case isBareID(s):
		return "book", numericPrefix(s)
	}
	return "", ""
}

// InferEntityType maps a URL to an entity type, defaulting to book.
func InferEntityType(u string) string {
	if e, _ := Classify(u); e != "" {
		return e
	}
	return "book"
}

// URL builders. Each accepts a bare id (or an id-with-slug) and returns the
// canonical page URL.

func BookURL(id string) string   { return BaseURL + "/book/show/" + numericPrefix(id) }
func AuthorURL(id string) string { return BaseURL + "/author/show/" + numericPrefix(id) }
func SeriesURL(id string) string { return BaseURL + "/series/" + numericPrefix(id) }
func UserURL(id string) string   { return BaseURL + "/user/show/" + numericPrefix(id) }
func QuoteURL(id string) string  { return BaseURL + "/quotes/" + numericPrefix(id) }

// ListURL keeps the "<num>.<slug>" id intact since Listopia uses it verbatim.
func ListURL(id string) string {
	id = strings.TrimSpace(id)
	if strings.Contains(id, "/list/show/") {
		return id
	}
	return BaseURL + "/list/show/" + id
}

// GenreURL keeps the slug as-is (genres have no numeric id).
func GenreURL(slug string) string {
	slug = strings.TrimSpace(slug)
	if i := strings.Index(slug, "/genres/"); i >= 0 {
		slug = slug[i+len("/genres/"):]
		if j := strings.IndexAny(slug, "?#"); j >= 0 {
			slug = slug[:j]
		}
	}
	return BaseURL + "/genres/" + slug
}

// ShelfURL builds a user's shelf page URL.
func ShelfURL(userID, shelf string, page int) string {
	v := url.Values{}
	v.Set("shelf", shelf)
	if page > 1 {
		v.Set("page", itoa(page))
	}
	return BaseURL + "/review/list/" + numericPrefix(userID) + "?" + v.Encode()
}

// ShelfRSSURL builds the RSS-feed form of a user's shelf.
func ShelfRSSURL(userID, shelf string) string {
	v := url.Values{}
	v.Set("shelf", shelf)
	return BaseURL + "/review/list_rss/" + numericPrefix(userID) + "?" + v.Encode()
}

// SearchAutocompleteURL builds the JSON autocomplete endpoint URL.
func SearchAutocompleteURL(q string) string {
	v := url.Values{}
	v.Set("format", "json")
	v.Set("q", q)
	return BaseURL + "/book/auto_complete?" + v.Encode()
}

// SearchHTMLURL builds the full HTML search URL for a page.
func SearchHTMLURL(q string, page int) string {
	return SearchURL(SearchQuery{Query: q, Page: page})
}

// SearchURL builds the search page URL for a whole query.
//
// search[source] is always set, because the site's own tab links set it and a
// request that leaves it off is not the request a browser would have made.
// search[field] is only set when it narrows something, since the site treats
// missing and all as the same thing and the shorter URL is the one that
// matches what the tabs link to.
func SearchURL(sq SearchQuery) string {
	sq = sq.Norm()
	v := url.Values{}
	v.Set("q", sq.Query)
	v.Set("search[source]", sq.Source)
	if sq.Field != "" && sq.Field != "all" {
		v.Set("search[field]", sq.Field)
	}
	v.Set("search_type", sq.Type)
	if sq.Page > 1 {
		v.Set("page", itoa(sq.Page))
	}
	return BaseURL + "/search?" + v.Encode()
}

var reBareID = regexp.MustCompile(`^\d+`)

func isBareID(s string) bool { return reBareID.MatchString(s) && !strings.Contains(s, "://") }

// numericPrefix returns the leading numeric run, or the trimmed input if none.
func numericPrefix(s string) string {
	s = strings.TrimSpace(s)
	if m := reBareID.FindString(s); m != "" {
		return m
	}
	return s
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// EditionsURL builds a work's editions page. The id is a work id and not a book
// id, which is the single most common way to get a 404 out of this surface.
func EditionsURL(workID string, page int) string {
	u := BaseURL + "/work/editions/" + numericPrefix(workID)
	if page > 1 {
		u += "?page=" + strconv.Itoa(page)
	}
	return u
}

// WorkQuotesURL builds a work's quotes page, also keyed by work and not book.
func WorkQuotesURL(workID string, page int) string {
	u := BaseURL + "/work/quotes/" + numericPrefix(workID)
	if page > 1 {
		u += "?page=" + strconv.Itoa(page)
	}
	return u
}

// AuthorQuotesURL is the other half of s7.
//
// Two URLs render the same markup around a different subject, so both are built
// here and the extractor tells them apart by the path it was handed.
func AuthorQuotesURL(authorID string, page int) string {
	u := BaseURL + "/author/quotes/" + numericPrefix(authorID)
	if page > 1 {
		u += "?page=" + strconv.Itoa(page)
	}
	return u
}

// pathSegmentAfter returns the whole path segment following a prefix.
//
// Different from extractIDFromPath, which takes the numeric prefix of that
// segment. Some ids are the whole segment: /list/show/1.Best_Books_Ever needs
// its slug because the bare /list/show/1 redirects rather than answering.
func pathSegmentAfter(s, prefix string) string {
	i := strings.Index(s, prefix)
	if i < 0 {
		return ""
	}
	seg := s[i+len(prefix):]
	if j := strings.IndexAny(seg, "/?#"); j >= 0 {
		seg = seg[:j]
	}
	return seg
}
