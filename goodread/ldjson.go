package goodread

import (
	"encoding/json"
	"html"
	"regexp"
	"strings"
)

// LDBook is the schema.org Book block Goodreads ships for search engines.
//
// Thin next to the Apollo cache and worth having for two reasons. It is a cross
// check: when JSON-LD and the cache disagree on the title, the extractor is
// wrong and a test can say so. And it is the fallback that survives a framework
// removal, since this block is there for Google and Google is not going away.
type LDBook struct {
	Type          string `json:"@type"`
	Name          string `json:"name"`
	Image         string `json:"image"`
	BookFormat    string `json:"bookFormat"`
	NumberOfPages int    `json:"numberOfPages"`
	InLanguage    string `json:"inLanguage"`
	ISBN          string `json:"isbn"`
	Awards        string `json:"awards"`
	Author        []struct {
		Type string `json:"@type"`
		Name string `json:"name"`
		URL  string `json:"url"`
	} `json:"author"`
	AggregateRating struct {
		RatingValue float64 `json:"ratingValue"`
		RatingCount int     `json:"ratingCount"`
		ReviewCount int     `json:"reviewCount"`
	} `json:"aggregateRating"`
}

// ldJSONRe finds the block by its type attribute.
//
// Matched non-greedily and on the tag rather than on the JSON, so a script tag
// somewhere else on the page cannot swallow it.
var ldJSONRe = regexp.MustCompile(`(?s)<script[^>]+type=["']application/ld\+json["'][^>]*>(.*?)</script>`)

// parseLDJSON reads the schema.org block, level 2 of the ladder.
func parseLDJSON(body []byte) (*LDBook, bool) {
	m := ldJSONRe.FindSubmatch(body)
	if m == nil {
		return nil, false
	}
	var b LDBook
	if err := json.Unmarshal(m[1], &b); err != nil {
		return nil, false
	}
	// Goodreads HTML-escapes inside this block, so awards and titles come back
	// carrying &apos; and &amp;. Unescaping here means every consumer does not
	// have to remember to.
	b.Name = html.UnescapeString(b.Name)
	b.Awards = html.UnescapeString(b.Awards)
	for i := range b.Author {
		b.Author[i].Name = html.UnescapeString(b.Author[i].Name)
	}
	return &b, b.Type != ""
}

// ogRe finds Open Graph meta tags in either attribute order.
var ogRe = regexp.MustCompile(`<meta[^>]*(?:property=["']og:([a-z_:]+)["'][^>]*content=["']([^"']*)["']|content=["']([^"']*)["'][^>]*property=["']og:([a-z_:]+)["'])[^>]*>`)

// parseOpenGraph reads the og: meta tags, the other half of level 2.
//
// These are the only structured data on the Rails pages, so for author, list,
// genre, editions and quotes this is not a fallback, it is the whole of what
// the page states about itself outside the rendered body.
func parseOpenGraph(body []byte) map[string]string {
	out := map[string]string{}
	for _, m := range ogRe.FindAllSubmatch(body, -1) {
		key, val := string(m[1]), string(m[2])
		if key == "" {
			key, val = string(m[4]), string(m[3])
		}
		if key == "" {
			continue
		}
		out[key] = html.UnescapeString(val)
	}
	return out
}

// TitleWithoutSeries strips the "(Series, #1)" suffix Goodreads appends.
//
// JSON-LD and og:title both carry the decorated form and the Apollo cache
// carries the plain one, so a cross check between the levels has to compare
// like with like. Splitting on the last open paren is enough: a title with a
// real trailing parenthetical is rare and a title with a series suffix is the
// common case.
func TitleWithoutSeries(s string) string {
	i := strings.LastIndex(s, " (")
	if i < 0 || !strings.HasSuffix(s, ")") {
		return s
	}
	return strings.TrimSpace(s[:i])
}

// metaNameRe finds name= meta tags in either attribute order.
//
// Separate from ogRe because the Rails surfaces do not all carry og:. The list
// page states its title and its size in twitter:title and nowhere else that is
// not a rendered heading, so on that page this is the whole of level 2.
var metaNameRe = regexp.MustCompile(`<meta[^>]*(?:name=["']([a-zA-Z_:-]+)["'][^>]*content=["']([^"']*)["']|content=["']([^"']*)["'][^>]*name=["']([a-zA-Z_:-]+)["'])[^>]*>`)

// parseMetaNames reads the name= meta tags, including the twitter: ones.
func parseMetaNames(body []byte) map[string]string {
	out := map[string]string{}
	for _, m := range metaNameRe.FindAllSubmatch(body, -1) {
		key, val := string(m[1]), string(m[2])
		if key == "" {
			key, val = string(m[4]), string(m[3])
		}
		if key == "" {
			continue
		}
		out[key] = html.UnescapeString(val)
	}
	return out
}
