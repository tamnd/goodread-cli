package goodread

import (
	"fmt"
	"net/url"
	"strings"
)

// One id parser, used by every command.
//
// v0.2.0 had Classify plus a numericPrefix call at each call site plus a URL
// builder per entity, and the result was that `goodread book` and `goodread
// similar` accepted slightly different things. Every accepted shape is listed
// here and TestParseRefAcceptsEveryShape walks the list, so adding a shape to
// one command adds it to all of them.

// EntityRef is a parsed reference to something on Goodreads.
type EntityRef struct {
	// Entity is the kind: book, work, author, series, list, genre, user,
	// quote, shelf.
	Entity string `json:"entity"`

	// ID is the id in the form that entity's URL wants. Numeric for most,
	// "<num>.<slug>" for a Listopia list, a bare slug for a genre.
	ID string `json:"id"`

	// Slug is the human readable tail a URL carried, kept because it is the
	// only title-ish thing a bare reference has and it costs nothing.
	Slug string `json:"slug,omitempty"`

	// Extra carries the second part of a two part reference, which today means
	// the shelf name in gr:shelf/221050/to-read.
	Extra string `json:"extra,omitempty"`

	// Input is what the user actually typed, so an error message can quote it
	// back rather than quoting the tool's guess at it.
	Input string `json:"input"`
}

// URL returns the canonical page for the reference.
func (r EntityRef) URL() string {
	switch r.Entity {
	case "book":
		return BookURL(r.ID)
	case "work":
		return BaseURL + "/work/" + r.ID
	case "author":
		return AuthorURL(r.ID)
	case "series":
		return SeriesURL(r.ID)
	case "list":
		return ListURL(r.ID)
	case "genre":
		return GenreURL(r.ID)
	case "user":
		return UserURL(r.ID)
	case "quote":
		return QuoteURL(r.ID)
	case "shelf":
		return ShelfURL(r.ID, r.Extra, 1)
	}
	return ""
}

func (r EntityRef) String() string { return r.Entity + " " + r.ID }

// refPaths maps a URL path segment to the entity it names.
//
// Ordered longest first, because /work/editions/ and /work/quotes/ both start
// with /work/ and a shorter match would swallow them.
var refPaths = []struct {
	prefix string
	entity string
}{
	{"/work/editions/", "work"},
	{"/work/quotes/", "work"},
	{"/work/shelves/", "work"},
	{"/book/show/", "book"},
	{"/author/show/", "author"},
	{"/list/show/", "list"},
	{"/user/show/", "user"},
	{"/review/list/", "shelf"},
	{"/series/", "series"},
	{"/quotes/", "quote"},
	{"/genres/", "genre"},
	{"/work/", "work"},
}

// ParseRef reads any reference to a Goodreads entity.
//
// Four shapes, all of which people actually paste:
//
//	2767052                                              a bare id
//	https://www.goodreads.com/book/show/2767052-the-...   a URL with a slug
//	https://www.goodreads.com/search?q=dune&page=2        a URL with a query tail
//	gr:book/2767052                                       the gr: URI form
//
// A bare id is a book, because that is what a bare Goodreads id nearly always
// is and every other entity is reachable by naming it.
func ParseRef(s string) (EntityRef, error) {
	in := strings.TrimSpace(s)
	if in == "" {
		return EntityRef{}, fmt.Errorf("empty reference")
	}
	r := EntityRef{Input: in}

	// gr: first, since it is the only form that is unambiguous by construction.
	if rest, ok := strings.CutPrefix(in, "gr:"); ok {
		parts := strings.Split(strings.Trim(rest, "/"), "/")
		if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
			return r, fmt.Errorf("%q is not a gr: reference, want gr:<entity>/<id>", in)
		}
		r.Entity = parts[0]
		r.ID, r.Slug = splitIDSlug(parts[1], r.Entity)
		if len(parts) > 2 {
			r.Extra = parts[2]
		}
		if r.Entity == "shelf" && r.Extra == "" {
			return r, fmt.Errorf("%q names a shelf and no shelf, want gr:shelf/<user>/<shelf>", in)
		}
		return r, nil
	}

	if strings.Contains(in, "goodreads.com") || strings.HasPrefix(in, "/") {
		u, err := url.Parse(in)
		if err != nil {
			return r, fmt.Errorf("%q is not a URL: %w", in, err)
		}
		path := u.Path
		for _, p := range refPaths {
			seg, ok := after(path, p.prefix)
			if !ok {
				continue
			}
			r.Entity = p.entity
			r.ID, r.Slug = splitIDSlug(seg, p.entity)
			// A shelf lives in the query, not the path, which is why the shelf
			// URL is the one that needs the query kept.
			if p.entity == "shelf" {
				r.Extra = u.Query().Get("shelf")
				if r.Extra == "" {
					r.Extra = "read"
				}
			}
			if r.ID == "" {
				return r, fmt.Errorf("%q has no id after %s", in, p.prefix)
			}
			return r, nil
		}
		return r, fmt.Errorf("%q is a Goodreads URL for something this tool does not read", in)
	}

	if isBareID(in) {
		r.Entity = "book"
		r.ID, r.Slug = splitIDSlug(in, "book")
		return r, nil
	}
	return r, fmt.Errorf("%q is not an id, a Goodreads URL, or a gr: reference", in)
}

// ParseRefAs reads a reference and insists on an entity.
//
// `goodread author 2767052` means the author with that id, not the book, so a
// bare id takes the entity of the command that received it. A URL that names a
// different entity is an error rather than a silent reinterpretation, because
// pasting a book URL into the author command is a mistake worth hearing about.
func ParseRefAs(s, entity string) (EntityRef, error) {
	r, err := ParseRef(s)
	if err != nil {
		return r, err
	}
	if r.Entity == entity {
		return r, nil
	}
	// A bare id defaults to book, and defaulting is not the same as being told.
	if r.Entity == "book" && !strings.Contains(r.Input, "/") && !strings.HasPrefix(r.Input, "gr:") {
		r.Entity = entity
		return r, nil
	}
	return r, fmt.Errorf("%q is a %s and this command reads a %s", r.Input, r.Entity, entity)
}

// after returns what follows prefix in path, and whether it was there.
func after(path, prefix string) (string, bool) {
	i := strings.Index(path, prefix)
	if i < 0 {
		return "", false
	}
	rest := path[i+len(prefix):]
	if j := strings.IndexByte(rest, '/'); j >= 0 {
		rest = rest[:j]
	}
	return rest, true
}

// splitIDSlug separates 2767052-the-hunger-games into its two halves.
//
// A list keeps both, because Listopia's URLs are keyed on the whole string and
// truncating one gives a 404. A genre has no numeric id at all and is a slug
// all the way through.
func splitIDSlug(seg, entity string) (id, slug string) {
	seg = strings.TrimSpace(seg)
	if i := strings.IndexAny(seg, "?#"); i >= 0 {
		seg = seg[:i]
	}
	switch entity {
	case "genre":
		return seg, ""
	case "list":
		if i := strings.IndexByte(seg, '.'); i >= 0 {
			return seg, seg[i+1:]
		}
		return seg, ""
	}
	if i := strings.IndexByte(seg, '-'); i >= 0 {
		return seg[:i], seg[i+1:]
	}
	return seg, ""
}
