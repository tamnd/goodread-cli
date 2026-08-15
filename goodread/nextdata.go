package goodread

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ErrNoNextData means the page did not ship a Next.js payload.
//
// This is not always a failure. Only the book page is a Next.js route; author,
// series, list, genre, editions and quotes are still Rails templates. See
// SurfaceHasNextData.
var ErrNoNextData = errors.New("no __NEXT_DATA__ on this page")

// NextData is the Next.js payload, decoded only as far as we need it.
//
// The Apollo cache stays as RawMessage. A book page carries 129 entities and a
// quick read wants four of them, so decoding the lot on the way in is work
// thrown away.
type NextData struct {
	BuildID string          `json:"buildId"`
	Page    string          `json:"page"`
	Props   json.RawMessage `json:"props"`
}

// nextData pulls the Next.js payload out of a page.
//
// It matches on the id attribute rather than on position, because Goodreads
// ships several inline scripts and their order is not stable. The content is
// JSON, not JavaScript, so it goes straight to encoding/json with nothing
// evaluated.
//
// The tag is found by scanning rather than by parsing the document into a DOM.
// Pages are a megabyte and a crawl reads a lot of them, and building a node
// tree to read one script tag is most of the cost of the parse for none of the
// benefit.
func nextData(body []byte) (*NextData, error) {
	raw, err := scriptByID(body, "__NEXT_DATA__")
	if err != nil {
		return nil, err
	}
	var nd NextData
	if err := json.Unmarshal(raw, &nd); err != nil {
		return nil, fmt.Errorf("decode __NEXT_DATA__: %w", err)
	}
	return &nd, nil
}

// scriptByID returns the body of the first <script> tag carrying the given id.
//
// Deliberately not a regex over the JSON. Find the tag boundaries, hand the
// bytes to a decoder, and work with the tree. A regex that reaches inside JSON
// works until a description contains the character it anchors on.
func scriptByID(body []byte, id string) (json.RawMessage, error) {
	s := string(body)
	needle := `id="` + id + `"`
	at := strings.Index(s, needle)
	if at < 0 {
		// Attribute order is not guaranteed, so fall back to the single-quoted
		// and unquoted spellings before giving up.
		for _, alt := range []string{`id='` + id + `'`, `id=` + id} {
			if at = strings.Index(s, alt); at >= 0 {
				break
			}
		}
	}
	if at < 0 {
		return nil, ErrNoNextData
	}
	open := strings.LastIndex(s[:at], "<script")
	if open < 0 {
		return nil, ErrNoNextData
	}
	start := strings.IndexByte(s[at:], '>')
	if start < 0 {
		return nil, ErrNoNextData
	}
	start += at + 1
	end := strings.Index(s[start:], "</script>")
	if end < 0 {
		return nil, ErrNoNextData
	}
	return json.RawMessage(strings.TrimSpace(s[start : start+end])), nil
}

// ApolloState returns the normalised GraphQL cache from the payload.
func (n *NextData) ApolloState() (Apollo, error) {
	var props struct {
		PageProps struct {
			ApolloState Apollo `json:"apolloState"`
		} `json:"pageProps"`
	}
	if err := json.Unmarshal(n.Props, &props); err != nil {
		return nil, fmt.Errorf("decode pageProps: %w", err)
	}
	if len(props.PageProps.ApolloState) == 0 {
		return nil, errors.New("__NEXT_DATA__ carried no apolloState")
	}
	return props.PageProps.ApolloState, nil
}

// SurfaceHasNextData reports whether a surface is a Next.js route.
//
// Measured 2026-08-15 against every surface in Ops. Only the book page is.
// This is a correction to the spec, which assumed the whole site had moved and
// designed a three level ladder on that basis. It has not, so for s2 through
// s7 the ladder starts at level 2 and level 3 is not debt to be driven down,
// it is the only thing there is. Recording that here rather than discovering it
// per caller keeps the extraction report honest about which zero is which.
func SurfaceHasNextData(surface string) bool {
	return surface == "s1"
}

// SurfaceSource says what a surface actually serves, for the ladder report.
//
// Worth spelling out rather than defaulting everything that is not Next.js to
// "Rails template", because three of these are not HTML at all and calling them
// templates would be wrong in a report whose whole job is to be accurate about
// where data comes from.
func SurfaceSource(surface string) string {
	switch surface {
	case "s1":
		return "Next.js, Apollo cache inline"
	case "s8":
		return "XML sitemap"
	case "s9":
		return "plain text"
	case "s14":
		return "public JSON"
	default:
		return "Rails HTML, og: tags and selectors"
	}
}
