package goodread

import (
	"encoding/json"
	"strconv"
	"strings"
)

// Apollo is the normalised GraphQL store a page ships inline, kept undecoded until something asks.
//
// Named Apollo rather than Cache because Cache is already the on-disk page
// cache in this package, and two things called Cache in one package is how you
// end up reading the wrong one at a call site.
type Apollo map[string]json.RawMessage

// FieldKey is an Apollo field name with its GraphQL arguments.
//
// Apollo writes the arguments into the key, so the same field requested twice
// with different arguments lands under two keys:
//
//	description
//	description({"stripped":true})
//	quotes({"pagination":{"limit":1}})
//
// The arguments are worth reading rather than discarding. quotes with a limit
// of 1 says the page asked for one quote, so a record built from that page must
// not claim to carry the work's quotes, and the honest missed sentence is
// generated from the limit rather than hardcoded.
type FieldKey struct {
	Name string
	Args json.RawMessage // nil when the key carried no arguments
}

// ParseFieldKey splits a cache field key into its name and arguments.
//
// Name runs up to the first open paren, then the argument JSON up to the
// matching close paren. Matching, not the last one: arguments nest, and
// getPageBannerInput carries an object inside an object. A strings.Contains
// check passes for a while and then matches the wrong field on a page shape
// nobody tested.
func ParseFieldKey(k string) FieldKey {
	open := strings.IndexByte(k, '(')
	if open < 0 {
		return FieldKey{Name: k}
	}
	depth := 0
	inString := false
	escaped := false
	for i := open; i < len(k); i++ {
		c := k[i]
		switch {
		case escaped:
			escaped = false
		case c == '\\' && inString:
			escaped = true
		case c == '"':
			inString = !inString
		case inString:
			// Parens inside a string argument are not structure.
		case c == '(':
			depth++
		case c == ')':
			depth--
			if depth == 0 {
				return FieldKey{Name: k[:open], Args: json.RawMessage(k[open+1 : i])}
			}
		}
	}
	// Unbalanced. Treat the whole thing as a name rather than guessing, since a
	// half-parsed key is worse than an odd-looking one.
	return FieldKey{Name: k}
}

// Limit reads a pagination limit out of a field key's arguments.
//
// Reports false when the key carried no limit, which is a different answer from
// a limit of zero. This is what turns "the page asked for one quote" into a
// missed sentence that stays true when Goodreads changes the number.
func (f FieldKey) Limit() (int, bool) {
	if len(f.Args) == 0 {
		return 0, false
	}
	var args map[string]json.RawMessage
	if err := json.Unmarshal(f.Args, &args); err != nil {
		return 0, false
	}
	if raw, ok := args["pagination"]; ok {
		var page struct {
			Limit *int `json:"limit"`
		}
		if err := json.Unmarshal(raw, &page); err == nil && page.Limit != nil {
			return *page.Limit, true
		}
	}
	if raw, ok := args["limit"]; ok {
		var n int
		if err := json.Unmarshal(raw, &n); err == nil {
			return n, true
		}
	}
	return 0, false
}

// String rebuilds the key, so a round trip through the parser is checkable.
func (f FieldKey) String() string {
	if len(f.Args) == 0 {
		return f.Name
	}
	return f.Name + "(" + string(f.Args) + ")"
}

// Ref is a pointer to another entity in the cache.
//
// Resolved false is a documented state and not an error. A page's cache holds
// references to entities the page did not include, which is the page not having
// asked for them rather than anything going wrong. The id is enough to fetch
// the thing later, and "this exists and we did not read it" is a more useful
// record than either a silent omission or a failure.
type Ref struct {
	Type     string `json:"type"`
	ID       string `json:"id"`
	Key      string `json:"key"`
	Resolved bool   `json:"resolved"`
}

// SplitCacheKey splits an Apollo cache key into its type and id.
//
// On the first colon only. The id is a kca:// URI and contains its own colon,
// so splitting on every colon gives a type of Book and an id of //book/amzn1...
// which is wrong in a way that still looks plausible in a debugger.
func SplitCacheKey(key string) (typ, id string) {
	if i := strings.IndexByte(key, ':'); i >= 0 {
		return key[:i], key[i+1:]
	}
	return key, ""
}

// MaxRefDepth bounds reference resolution.
//
// A book references a work, which references a best book, which is the book.
// Cycles are normal here and not a defect, so resolution keeps a visited set
// and a depth bound rather than trying to prove the graph is acyclic.
const MaxRefDepth = 8

// Entity returns one entity from the cache, decoded.
func (c Apollo) Entity(key string) (map[string]json.RawMessage, bool) {
	raw, ok := c[key]
	if !ok {
		return nil, false
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, false
	}
	return m, true
}

// Keys returns every cache key of a given type, in no particular order.
func (c Apollo) Keys(typ string) []string {
	var out []string
	for k := range c {
		if t, _ := SplitCacheKey(k); t == typ {
			out = append(out, k)
		}
	}
	return out
}

// Counts returns how many entities of each type the cache holds, which is the
// one number that tells you at a glance whether a page loaded what you expected.
func (c Apollo) Counts() map[string]int {
	out := map[string]int{}
	for k := range c {
		t, _ := SplitCacheKey(k)
		out[t]++
	}
	return out
}

// Root returns a field of ROOT_QUERY by name, ignoring its arguments.
//
// This is how you find the entity the page is actually about. Picking the first
// Book in the cache does not work: the measured page carries three, and two of
// them are stubs holding nothing but __typename and id because something on the
// page referenced them. ROOT_QUERY names the query the page ran, so
// getBookByLegacyId points at the one book the reader asked for.
func (c Apollo) Root(name string) (json.RawMessage, FieldKey, bool) {
	root, ok := c.Entity("ROOT_QUERY")
	if !ok {
		return nil, FieldKey{}, false
	}
	for field, v := range root {
		if fk := ParseFieldKey(field); fk.Name == name {
			return v, fk, true
		}
	}
	return nil, FieldKey{}, false
}

// RootRefKey returns the cache key ROOT_QUERY's named field points at.
func (c Apollo) RootRefKey(name string) (string, bool) {
	v, _, ok := c.Root(name)
	if !ok {
		return "", false
	}
	var ref struct {
		Ref string `json:"__ref"`
	}
	if err := json.Unmarshal(v, &ref); err != nil || ref.Ref == "" {
		return "", false
	}
	return ref.Ref, true
}

// Resolve walks a value, replacing every __ref with the entity it points at.
//
// Three rules, each of which comes from something the real cache does.
// Cycles resolve to a stub rather than recursing, since the book/work/bestBook
// triangle is on every page. Depth is bounded, because a deep nest costs more
// than it returns. A reference the cache does not hold becomes a stub with
// resolved false, since that is the page not having asked for it.
func (c Apollo) Resolve(v json.RawMessage) any {
	return c.resolve(v, 0, map[string]bool{})
}

func (c Apollo) resolve(v json.RawMessage, depth int, seen map[string]bool) any {
	var probe any
	if err := json.Unmarshal(v, &probe); err != nil {
		return nil
	}
	switch t := probe.(type) {
	case map[string]any:
		return c.resolveObject(v, t, depth, seen)
	case []any:
		var items []json.RawMessage
		if err := json.Unmarshal(v, &items); err != nil {
			return t
		}
		out := make([]any, 0, len(items))
		for _, it := range items {
			out = append(out, c.resolve(it, depth, seen))
		}
		return out
	default:
		return probe
	}
}

func (c Apollo) resolveObject(raw json.RawMessage, obj map[string]any, depth int, seen map[string]bool) any {
	if r, ok := obj["__ref"].(string); ok {
		typ, id := SplitCacheKey(r)
		stub := Ref{Type: typ, ID: id, Key: r}
		if depth >= MaxRefDepth || seen[r] {
			return stub
		}
		target, ok := c[r]
		if !ok {
			return stub
		}
		seen[r] = true
		// Unmarked on the way out so a sibling can resolve the same entity.
		// Only an ancestor being revisited is a cycle; two fields pointing at
		// the same author is just a shared author.
		defer delete(seen, r)
		resolved := c.resolve(target, depth+1, seen)
		if m, ok := resolved.(map[string]any); ok {
			m["__key"] = r
			return m
		}
		return resolved
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return obj
	}
	out := make(map[string]any, len(fields))
	for k, v := range fields {
		fk := ParseFieldKey(k)
		val := c.resolve(v, depth, seen)
		// Two keys for the same field name, one plain and one with arguments,
		// both get kept. description and description({"stripped":true}) are
		// different data and deriving either from the other is lossy.
		if _, clash := out[fk.Name]; clash && len(fk.Args) > 0 {
			out[k] = val
			continue
		}
		if len(fk.Args) > 0 {
			if _, plain := fields[fk.Name]; plain {
				out[k] = val
				continue
			}
		}
		out[fk.Name] = val
	}
	return out
}

// LegacyID digs the numeric Goodreads id out of an entity.
//
// Goodreads runs two id spaces, the integer in the URL and the opaque kca one,
// and neither derives from the other. Both are kept; this reads the first.
func LegacyID(m map[string]json.RawMessage) (int, bool) {
	raw, ok := m["legacyId"]
	if !ok {
		return 0, false
	}
	var n int
	if err := json.Unmarshal(raw, &n); err == nil {
		return n, true
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if n, err := strconv.Atoi(s); err == nil {
			return n, true
		}
	}
	return 0, false
}
