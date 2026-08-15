package goodread

import (
	"encoding/json"
	"fmt"
	"html"
	"regexp"
)

// The second kind of embedded state, found while writing the Rails extractors.
//
// The story so far said only the book page carries structured data, and that
// everything else is selectors over rendered HTML. That is half right. The
// Rails pages mount React islands, and an island is handed its props as JSON in
// a data-react-props attribute. Most of them are ads and header state and carry
// nothing worth reading, but the series page mounts SeriesHeader and SeriesList
// and those two carry the whole page: book id, work id, page count, average
// rating, ratings count, text reviews count, publication date, edition count
// and the author, per book, already typed.
//
// So a data-react-props payload is level 1 the same way the Apollo cache is.
// Both are the state the page rendered itself from, which is the thing a
// selector is only ever an approximation of.
//
// Measured on the captures: series has SeriesHeader and SeriesList, and author,
// list, genre, editions and quotes have nothing but GoogleBannerAd,
// HeaderStoreConnector, LoginInterstitial and friends. Those five stay on level
// 2 and level 3.

var reReactIsland = regexp.MustCompile(`data-react-class="ReactComponents\.([A-Za-z0-9_]+)"\s+data-react-props="([^"]*)"`)

// ReactProps returns the props of the first island of a given component class.
//
// The attribute is HTML escaped, so it is unescaped before decoding rather than
// pattern matched through the escaping. The value is JSON and goes to the
// decoder with nothing evaluated, the same rule the Next.js payload follows.
func ReactProps(body []byte, component string) (json.RawMessage, error) {
	for _, m := range reReactIsland.FindAllSubmatch(body, -1) {
		if string(m[1]) != component {
			continue
		}
		raw := json.RawMessage(html.UnescapeString(string(m[2])))
		if !json.Valid(raw) {
			return nil, fmt.Errorf("%s props are not valid JSON", component)
		}
		return raw, nil
	}
	return nil, fmt.Errorf("no ReactComponents.%s island on this page", component)
}

// ReactPropsAll returns the props of every island of a given class.
//
// Needed because a page can mount the same component more than once. The series
// page mounts SeriesList once per run of consecutive books, so Harry Potter
// arrives as one island of two books and one of eight, and reading only the
// first would have returned a two book series with no error anywhere.
func ReactPropsAll(body []byte, component string) []json.RawMessage {
	var out []json.RawMessage
	for _, m := range reReactIsland.FindAllSubmatch(body, -1) {
		if string(m[1]) != component {
			continue
		}
		raw := json.RawMessage(html.UnescapeString(string(m[2])))
		if json.Valid(raw) {
			out = append(out, raw)
		}
	}
	return out
}

// ReactIslands names every island on a page, for `goodread extraction`.
//
// Worth being able to print, because the next time one of these Rails pages
// grows a component with real props in it, this is the report that says so.
func ReactIslands(body []byte) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range reReactIsland.FindAllSubmatch(body, -1) {
		name := string(m[1])
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}
