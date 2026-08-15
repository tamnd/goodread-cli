package goodread

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Folding a stored record into the graph.
//
// This reads the record as json rather than as its Go type, and that is on
// purpose. The store holds nine kinds of node written by a dozen call sites,
// several of them still on the v0.2.0 shapes, and a type switch over all of
// them would need editing every time a surface is added. The keys it reads are
// the ones 03_model.md fixes, so a record that follows the model indexes
// itself and one that does not is left alone rather than half read.
//
// Nothing here fetches and nothing here fails a command. An index that could
// not be built is a worse search later, not a failed read now, which is why
// Put keeps its own error and this one only surfaces through the store tests.

// NodeURI builds the canonical URI for a node.
//
// From the legacy id, per 04_graph.md section 3, because the legacy id is the
// one that has already survived a site rewrite. A record with no legacy id gets
// no URI and is not indexed, rather than getting a URI built from the kca id
// that a second read of the same thing would not agree with.
func NodeURI(kind, id string) string {
	if kind == "" || id == "" {
		return ""
	}
	return "gr:" + kind + "/" + id
}

// slug is the URI form of a name that has no id of its own.
//
// Lowercases, strips accents to their base letters, replaces any run of non
// alphanumeric characters with a single hyphen and trims hyphens from both
// ends. Stable across versions by contract: changing it renames every genre,
// place and character node in every store anybody has built, so a change here
// is a breaking change and TestSlugIsStable is what says so.
func slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		r = unaccent(r)
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			dash = false
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// unaccent maps the Latin 1 letters people actually meet in author names down
// to their base letter. Not a full Unicode fold, because the alternative is a
// dependency and this covers the accented vowels and the handful of consonants
// that turn up in Goodreads names.
func unaccent(r rune) rune {
	if base, ok := accents[r]; ok {
		return base
	}
	return r
}

var accents = func() map[rune]rune {
	m := map[rune]rune{}
	for base, set := range map[rune]string{
		'a': "àáâãäåā", 'e': "èéêëē", 'i': "ìíîïī", 'o': "òóôõöøō",
		'u': "ùúûüū", 'y': "ýÿ", 'n': "ñń", 'c': "çć", 's': "šś",
		'z': "žź", 'd': "đ", 'l': "ł",
	} {
		for _, r := range set {
			m[r] = base
		}
	}
	return m
}()

// IndexRecord folds one stored record into the node and edge tables.
//
// kind is the store's entity type, id is the key it was stored under and data
// is the marshalled record. A kind with no table is not an error: an editions
// page has no node of its own and is still worth walking, because the editions
// on it are books.
func (s *Store) IndexRecord(kind, id, url string, data []byte) error {
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}

	// The book command stores {"book":{...},"work":{...}}, which is two nodes
	// and the edge between them, so it is unwrapped rather than indexed as one.
	if inner, ok := m["book"].(map[string]any); ok && kind == "book" {
		var first error
		bookURI, err := s.indexOne("book", id, url, inner)
		if err != nil {
			first = err
		}
		if w, ok := m["work"].(map[string]any); ok {
			workURI, err := s.indexOne("work", "", "", w)
			if err != nil && first == nil {
				first = err
			}
			if bookURI != "" && workURI != "" {
				_ = s.PutEdge(bookURI, EdgeEditionOf, workURI, nil, surfaceOf(inner), retrievedAt(inner))
			}
		}
		return first
	}

	if _, err := tableFor(kind); err != nil {
		// No node of its own. Its rows are still books.
		s.indexCards("", m)
		return nil
	}
	_, err := s.indexOne(kind, id, url, m)
	return err
}

// indexOne writes one node and everything it points at, and returns its URI.
func (s *Store) indexOne(kind, fallbackID, url string, m map[string]any) (string, error) {
	uri := uriOf(kind, fallbackID, m)
	if uri == "" {
		return "", nil
	}

	n := Node{
		URI:         uri,
		Kind:        kind,
		ID:          kcaOf(m),
		LegacyID:    int64(keyNum(m, "legacy_id")),
		Title:       titleOf(m),
		Surfaces:    keyStrings(m, "surfaces"),
		RetrievedAt: retrievedAt(m),
		Description: firstKey(m, "description_stripped", "description", "bio_stripped", "bio"),
		AuthorName:  authorNameOf(m),
	}
	if kind == "book" {
		n.ISBN13 = keyStr(m, "isbn13")
		n.ASIN = keyStr(m, "asin")
		n.NumPages = int(keyNum(m, "num_pages"))
		n.Publisher = keyStr(m, "publisher")
		if t := keyTime(m, "publication_time"); !t.IsZero() {
			n.PublishedAt = t.Unix()
		}
		if w, ok := m["work"].(map[string]any); ok {
			n.WorkURI = uriOf("work", "", w)
		}
	}

	body, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	n.JSON = body
	if err := s.PutNode(n); err != nil {
		return "", err
	}

	s.indexRefs(uri, m)
	s.indexCards(uri, m)
	return uri, nil
}

// indexRefs writes the edges a record carries about itself.
//
// Directions are the spec's, not the ones that read most naturally in a
// sentence. `contributed_by` runs book to author even though "the author wrote
// the book" is how anybody would say it, because one canonical direction is
// what stops the same fact being written twice under two names, and Edges
// answers either way regardless.
func (s *Store) indexRefs(uri string, m map[string]any) {
	surface, at := surfaceOf(m), retrievedAt(m)
	isBook := strings.HasPrefix(uri, "gr:book/")
	isWork := strings.HasPrefix(uri, "gr:work/")

	for _, c := range keyMaps(m, "contributors") {
		au := uriOf("author", "", c)
		if au == "" {
			continue
		}
		var props any
		if role := keyStr(c, "role"); role != "" {
			props = map[string]string{"role": role}
		}
		_ = s.PutEdge(uri, EdgeContributed, au, props, surface, at)
		_ = s.PutNode(Node{URI: au, Kind: "author", ID: kcaOf(c),
			LegacyID: int64(keyNum(c, "legacy_id")), Title: keyStr(c, "name"),
			JSON:     mustJSON(map[string]any{"legacy_id": keyNum(c, "legacy_id"), "name": keyStr(c, "name")}),
			Surfaces: []string{surface}, RetrievedAt: at})
	}

	for _, e := range keyMaps(m, "series") {
		ref, _ := e["series"].(map[string]any)
		su := uriOf("series", "", ref)
		if su == "" {
			continue
		}
		// Both forms of the position. The raw string is the only one that can
		// hold 1-3 on an omnibus, and the parsed number is the only one that
		// sorts, so an edge that kept one of them would lose either the
		// omnibuses or the ordering.
		props := map[string]any{}
		if pos := keyStr(e, "position"); pos != "" {
			props["position"] = pos
		}
		if n, ok := e["number"].(float64); ok {
			props["number"] = n
		}
		_ = s.PutEdge(uri, EdgeInSeries, su, propsOrNil(props), surface, at)
		if name := keyStr(ref, "title"); name != "" {
			_ = s.PutNode(Node{URI: su, Kind: "series", Title: name,
				LegacyID: int64(keyNum(ref, "legacy_id")),
				JSON:     mustJSON(map[string]any{"kind": "series", "title": name}),
				Surfaces: []string{surface}, RetrievedAt: at})
		}
	}

	for _, g := range keyMaps(m, "genres") {
		if keyStr(g, "name") == "" {
			continue
		}
		_ = s.PutEdge(uri, EdgeShelvedAs, NodeURI("genre", genreURISlug(g)), nil, surface, at)
	}

	if w, ok := m["work"].(map[string]any); ok && isBook {
		if wu := uriOf("work", "", w); wu != "" {
			_ = s.PutEdge(uri, EdgeEditionOf, wu, nil, surface, at)
		}
	}

	if isWork {
		if b, ok := m["best_book"].(map[string]any); ok {
			if bu := uriOf("book", "", b); bu != "" {
				_ = s.PutEdge(uri, EdgeBestEdition, bu, nil, surface, at)
			}
		}
		// Places, characters and awards have no id of their own on Goodreads.
		// They are curated strings hanging off a work, so the slug is the id,
		// and two places with the same name in different countries collide.
		// That limit is in 04_graph.md rather than papered over with a
		// synthetic id nothing else in the world shares.
		s.indexNamed(uri, EdgeSetIn, "place", keyMaps(m, "places"), surface, at)
		s.indexNamed(uri, EdgeFeatures, "character", keyMaps(m, "characters"), surface, at)
		for _, a := range keyMaps(m, "awards_won") {
			name := keyStr(a, "name")
			if name == "" {
				continue
			}
			props := map[string]any{}
			if c := keyStr(a, "category"); c != "" {
				props["category"] = c
			}
			if y := keyNum(a, "year"); y > 0 {
				props["year"] = int64(y)
			}
			au := NodeURI("award", slug(name))
			_ = s.PutEdge(uri, EdgeWon, au, propsOrNil(props), surface, at)
			_ = s.PutNode(Node{URI: au, Kind: "award", Title: name,
				JSON:     mustJSON(map[string]any{"kind": "award", "name": name}),
				Surfaces: []string{surface}, RetrievedAt: at})
		}
	}

	// A quotes record points at whatever it was quoted from. The work quotes
	// page names a work and the author quotes page names an author, and only
	// the first of those is an edge the spec has.
	if subj, ok := m["subject"].(map[string]any); ok && strings.HasPrefix(uri, "gr:quotes/") {
		if wu := uriOf("work", "", subj); wu != "" && keyStr(subj, "type") == "Work" {
			_ = s.PutEdge(uri, EdgeQuotedFrom, wu, nil, surface, at)
		}
	}
}

// indexNamed writes the edges to nodes whose id is their own name.
func (s *Store) indexNamed(src, predicate, kind string, items []map[string]any, surface string, at time.Time) {
	for _, it := range items {
		name := keyStr(it, "name")
		if name == "" {
			continue
		}
		uri := NodeURI(kind, slug(name))
		_ = s.PutEdge(src, predicate, uri, nil, surface, at)
		_ = s.PutNode(Node{URI: uri, Kind: kind, Title: name,
			JSON:     mustJSON(map[string]any{"kind": kind, "name": name}),
			Surfaces: []string{surface}, RetrievedAt: at})
	}
}

func propsOrNil(m map[string]any) any {
	if len(m) == 0 {
		return nil
	}
	return m
}

// indexCards indexes the book rows a listing record carries.
//
// A list of 100 books read once puts 100 books in the store, thin ones, with a
// title and an author and nothing else. That is the point: they are findable,
// and the merge rule means the day somebody reads one properly the thin record
// gains the rest rather than being overwritten by it.
func (s *Store) indexCards(uri string, m map[string]any) {
	surface, at := surfaceOf(m), retrievedAt(m)
	for _, key := range []string{"books", "editions"} {
		for i, c := range keyMaps(m, key) {
			ref, _ := c["book"].(map[string]any)
			bu := uriOf("book", "", ref)
			if bu == "" {
				bu = uriOf("book", "", c)
			}
			if bu == "" {
				continue
			}
			// Only what the row actually said. No "this came from a card" flag,
			// because the merge rule would keep it set forever and a book that
			// has since been read properly would still be claiming to be a stub.
			body := map[string]any{
				"kind":      "book",
				"legacy_id": legacyOf(ref, c),
				"title":     firstKey(c, "title", "title_bare"),
				"surfaces":  []string{surface},
			}
			if d := keyStr(c, "description"); d != "" {
				body["description"] = d
			}
			_ = s.PutNode(Node{
				URI: bu, Kind: "book", LegacyID: legacyOf(ref, c),
				Title: firstKey(c, "title", "title_bare"), JSON: mustJSON(body),
				Surfaces: []string{surface}, RetrievedAt: at,
				ISBN13:      keyStr(c, "isbn13"),
				Description: keyStr(c, "description"),
				AuthorName:  cardAuthor(c),
			})
			// The edge runs from the row to the container, which is the
			// direction the spec fixes, and which container it is decides the
			// predicate: a list holds books and a work has editions, and those
			// are two different facts even though the row looks the same.
			switch {
			case strings.HasPrefix(uri, "gr:list/"):
				_ = s.PutEdge(bu, EdgeListedIn, uri, map[string]int{"position": i + 1}, surface, at)
			case strings.HasPrefix(uri, "gr:work/"):
				_ = s.PutEdge(bu, EdgeEditionOf, uri, nil, surface, at)
			}
			for _, con := range keyMaps(c, "contributors") {
				if au := uriOf("author", "", con); au != "" {
					var props any
					if role := keyStr(con, "role"); role != "" {
						props = map[string]string{"role": role}
					}
					_ = s.PutEdge(bu, EdgeContributed, au, props, surface, at)
				}
			}
		}
	}
}

// uriOf works out which id a record is keyed by.
//
// Genre by slug, shelf by the user and shelf name together, everything else by
// its legacy id. The fallback is the id the store was already keyed under,
// which is what carries the v0.2.0 records that never had a legacy_id field.
func uriOf(kind, fallbackID string, m map[string]any) string {
	if m == nil {
		return NodeURI(kind, fallbackID)
	}
	switch kind {
	case "genre":
		if sl := genreURISlug(m); sl != "" {
			return NodeURI("genre", sl)
		}
	case "shelf", "list", "user":
		if id := keyStr(m, "id"); id != "" {
			return NodeURI(kind, id)
		}
	}
	if n := int64(keyNum(m, "legacy_id")); n > 0 {
		return NodeURI(kind, strconv.FormatInt(n, 10))
	}
	// A Ref carries the legacy id in its id field when the surface it came from
	// was Rails rather than Apollo, so a numeric id is a legacy id.
	if id := keyStr(m, "id"); id != "" && isDigits(id) {
		return NodeURI(kind, id)
	}
	// The link, which is where the legacy id hides on an Apollo record. A
	// contributor in the cache is keyed by a kca id and carries
	// /author/show/153394.Suzanne_Collins, and dropping the author because the
	// cache spelled the id the modern way would cost the graph its main join.
	if u := keyStr(m, "web_url"); u != "" {
		if ent, id := Classify(u); id != "" && (ent == kind || kind == "") {
			return NodeURI(ent, id)
		}
	}
	if fallbackID != "" && isDigits(fallbackID) {
		return NodeURI(kind, fallbackID)
	}
	return ""
}

func genreURISlug(m map[string]any) string {
	if sl := keyStr(m, "slug"); sl != "" {
		return sl
	}
	if u := keyStr(m, "web_url"); u != "" {
		if i := strings.LastIndex(u, "/"); i >= 0 && i+1 < len(u) {
			return slug(u[i+1:])
		}
	}
	return slug(keyStr(m, "name"))
}

func legacyOf(ms ...map[string]any) int64 {
	for _, m := range ms {
		if n := int64(keyNum(m, "legacy_id")); n > 0 {
			return n
		}
		if id := keyStr(m, "id"); isDigits(id) {
			n, _ := strconv.ParseInt(id, 10, 64)
			return n
		}
	}
	return 0
}

func cardAuthor(c map[string]any) string {
	for _, con := range keyMaps(c, "contributors") {
		if n := keyStr(con, "name"); n != "" {
			return n
		}
	}
	return keyStr(c, "author_name")
}

func authorNameOf(m map[string]any) string {
	for _, con := range keyMaps(m, "contributors") {
		if n := keyStr(con, "name"); n != "" {
			return n
		}
	}
	if k, _ := m["kind"].(string); k == "author" {
		return keyStr(m, "name")
	}
	return ""
}

func titleOf(m map[string]any) string {
	if t := firstKey(m, "title", "name", "title_complete"); t != "" {
		return t
	}
	if ref, ok := m["subject"].(map[string]any); ok {
		return keyStr(ref, "title")
	}
	return ""
}

func kcaOf(m map[string]any) string {
	if id := keyStr(m, "id"); strings.HasPrefix(id, "kca://") {
		return id
	}
	return ""
}

func surfaceOf(m map[string]any) string {
	if ss := keyStrings(m, "surfaces"); len(ss) > 0 {
		return ss[0]
	}
	return "unknown"
}

func retrievedAt(m map[string]any) time.Time {
	if t := keyTime(m, "retrieved_at"); !t.IsZero() {
		return t
	}
	return time.Now().UTC()
}

func keyStr(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	s, _ := m[key].(string)
	return s
}

func firstKey(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v := keyStr(m, k); v != "" {
			return v
		}
	}
	return ""
}

func keyNum(m map[string]any, key string) float64 {
	if m == nil {
		return 0
	}
	switch v := m[key].(type) {
	case float64:
		return v
	case json.Number:
		f, _ := v.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(v, 64)
		return f
	}
	return 0
}

func keyTime(m map[string]any, key string) time.Time {
	s := keyStr(m, key)
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

func keyStrings(m map[string]any, key string) []string {
	raw, _ := m[key].([]any)
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

func keyMaps(m map[string]any, key string) []map[string]any {
	if m == nil {
		return nil
	}
	raw, _ := m[key].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, v := range raw {
		if mm, ok := v.(map[string]any); ok {
			out = append(out, mm)
		}
	}
	return out
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte(fmt.Sprintf(`{"error":%q}`, err.Error()))
	}
	return b
}
