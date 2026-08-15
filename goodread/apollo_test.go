package goodread

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// bookCache loads the popular book capture and returns its Apollo store.
func bookCache(t *testing.T) Apollo {
	t.Helper()
	nd, err := nextData(readCapture(t, "book_show_2767052.html.gz"))
	if err != nil {
		t.Fatalf("nextData: %v", err)
	}
	st, err := nd.ApolloState()
	if err != nil {
		t.Fatalf("apolloState: %v", err)
	}
	return st
}

func TestNextDataOnTheRealPage(t *testing.T) {
	nd, err := nextData(readCapture(t, "book_show_2767052.html.gz"))
	if err != nil {
		t.Fatalf("nextData: %v", err)
	}
	if nd.BuildID == "" {
		t.Error("no buildId, and buildId is the first thing you want when extraction breaks")
	}
	st, err := nd.ApolloState()
	if err != nil {
		t.Fatalf("apolloState: %v", err)
	}
	// Measured 2026-08-15: 129 entities in eight types. The exact count moves
	// as Goodreads changes what the page requests, so this asserts the shape
	// rather than the number.
	counts := st.Counts()
	for _, typ := range []string{"Book", "Work", "Series", "Contributor", "Review", "Shelving", "User", "ROOT_QUERY"} {
		if counts[typ] == 0 {
			t.Errorf("no %s entities in the cache, got %v", typ, counts)
		}
	}
	if counts["ROOT_QUERY"] != 1 {
		t.Errorf("ROOT_QUERY count = %d, want 1", counts["ROOT_QUERY"])
	}
}

// TestNoNextDataOnRailsPages records a measured fact the spec got wrong.
//
// The spec assumed the whole site had moved to Next.js and designed the ladder
// on that basis. Only the book page has. Author, list, genre, editions and
// quotes are still Rails templates, so for those surfaces level 3 is not debt
// to be driven to zero, it is the only source there is. This test exists so
// that if Goodreads does move them, we find out from a failing test and can go
// and delete selectors, rather than never noticing.
func TestNoNextDataOnRailsPages(t *testing.T) {
	for _, c := range loadCaptures(t) {
		_, err := nextData(readCapture(t, c.File))
		has := err == nil
		if want := SurfaceHasNextData(c.Surface); has != want {
			if has {
				t.Errorf("%s (%s) now ships __NEXT_DATA__, so it can move up the ladder", c.File, c.Surface)
			} else {
				t.Errorf("%s (%s) no longer ships __NEXT_DATA__", c.File, c.Surface)
			}
		}
	}
}

func TestSplitCacheKeyOnFirstColonOnly(t *testing.T) {
	cases := []struct {
		key, typ, id string
	}{
		{"Book:kca://book/amzn1.gr.book.v1.YaoKZD8xVx72w5T1ZgR1YQ", "Book", "kca://book/amzn1.gr.book.v1.YaoKZD8xVx72w5T1ZgR1YQ"},
		{"Work:kca://work/amzn1.gr.work.v1.qDM8aVuWTQKRymQBUS02og", "Work", "kca://work/amzn1.gr.work.v1.qDM8aVuWTQKRymQBUS02og"},
		{"ROOT_QUERY", "ROOT_QUERY", ""},
	}
	for _, c := range cases {
		typ, id := SplitCacheKey(c.key)
		if typ != c.typ || id != c.id {
			t.Errorf("SplitCacheKey(%q) = (%q, %q), want (%q, %q)", c.key, typ, id, c.typ, c.id)
		}
	}
}

// TestSplitCacheKeyAgainstEveryRealKey is the one that would have caught a
// split-on-every-colon parser.
//
// The ids in the real cache come in three shapes, which is more than the spec
// assumed. Book, Work, Contributor and Series use kca:// uris, which contain
// their own colon. User uses a bare integer. Shelving uses a whole JSON object
// as its id, and that object contains kca:// uris, so it has colons several
// levels down. The invariant that holds across all three is the round trip:
// type, then a colon, then everything else, put back together unchanged.
func TestSplitCacheKeyAgainstEveryRealKey(t *testing.T) {
	shapes := map[string]int{}
	for key := range bookCache(t) {
		typ, id := SplitCacheKey(key)
		if strings.ContainsAny(typ, ":/{") {
			t.Errorf("key %q gave type %q, which is not a type name", key, typ)
		}
		if key == "ROOT_QUERY" {
			continue
		}
		if got := typ + ":" + id; got != key {
			t.Errorf("key %q did not round trip, got %q", key, got)
		}
		switch {
		case strings.HasPrefix(id, "kca://"):
			shapes["kca"]++
		case strings.HasPrefix(id, "{"):
			shapes["json"]++
		default:
			shapes["scalar"]++
		}
	}
	// All three shapes are present on the measured page, and a parser that
	// only ever saw kca ids would pass a weaker test.
	for _, want := range []string{"kca", "json", "scalar"} {
		if shapes[want] == 0 {
			t.Errorf("no %s shaped ids in the capture, got %v", want, shapes)
		}
	}
	t.Logf("id shapes: %v", shapes)
}

func TestParseFieldKey(t *testing.T) {
	cases := []struct {
		in   string
		name string
		args string
	}{
		{"description", "description", ""},
		{`description({"stripped":true})`, "description", `{"stripped":true}`},
		{`quotes({"pagination":{"limit":1}})`, "quotes", `{"pagination":{"limit":1}}`},
		{`getBookByLegacyId({"legacyId":"2767052"})`, "getBookByLegacyId", `{"legacyId":"2767052"}`},
		// A paren inside a string argument is not structure.
		{`search({"q":"a (b) c"})`, "search", `{"q":"a (b) c"}`},
		// Unbalanced is treated as a plain name rather than half parsed.
		{`broken({"a":1}`, `broken({"a":1}`, ""},
	}
	for _, c := range cases {
		got := ParseFieldKey(c.in)
		if got.Name != c.name || string(got.Args) != c.args {
			t.Errorf("ParseFieldKey(%q) = (%q, %q), want (%q, %q)", c.in, got.Name, got.Args, c.name, c.args)
		}
		if c.args != "" && got.String() != c.in {
			t.Errorf("round trip of %q gave %q", c.in, got.String())
		}
	}
}

// TestParseFieldKeyOnEveryRealKey runs the parser over every field key in the
// capture and asserts the arguments it pulled out are valid JSON.
//
// This is the test a strings.Contains implementation fails: it passes on the
// keys somebody thought to write down and then mangles one nobody did.
func TestParseFieldKeyOnEveryRealKey(t *testing.T) {
	cache := bookCache(t)
	seenArgs := 0
	for key := range cache {
		entity, ok := cache.Entity(key)
		if !ok {
			t.Fatalf("entity %q would not decode", key)
		}
		for field := range entity {
			fk := ParseFieldKey(field)
			if fk.Name == "" {
				t.Errorf("field %q in %q parsed to an empty name", field, key)
			}
			if strings.ContainsAny(fk.Name, "()") {
				t.Errorf("field %q in %q left parens in the name %q", field, key, fk.Name)
			}
			if len(fk.Args) == 0 {
				continue
			}
			seenArgs++
			if !json.Valid(fk.Args) {
				t.Errorf("field %q in %q gave invalid argument JSON %q", field, key, fk.Args)
			}
			if fk.String() != field {
				t.Errorf("field %q did not round trip, got %q", field, fk.String())
			}
		}
	}
	if seenArgs == 0 {
		t.Fatal("no field key in the capture carried arguments, so this test proved nothing")
	}
	t.Logf("%d field keys with arguments", seenArgs)
}

// TestQuotesLimitIsReadFromTheKey is the case the missed sentence depends on.
//
// The book page asks for one quote. A record built from it must say so rather
// than implying it carries the work's quotes, and the number in the sentence
// comes from the key so it stays true when Goodreads changes it.
func TestQuotesLimitIsReadFromTheKey(t *testing.T) {
	cache := bookCache(t)
	keys := cache.Keys("Work")
	if len(keys) == 0 {
		t.Fatal("no Work in the capture")
	}
	work, _ := cache.Entity(keys[0])

	var found bool
	for field := range work {
		fk := ParseFieldKey(field)
		if fk.Name != "quotes" {
			continue
		}
		found = true
		n, ok := fk.Limit()
		if !ok {
			t.Fatalf("quotes key %q carried no limit", field)
		}
		if n <= 0 {
			t.Errorf("quotes limit = %d, want a positive sample size", n)
		}
		t.Logf("the book page asks for %d quote(s)", n)
	}
	if !found {
		t.Fatal("no quotes field on the Work, so the missed sentence has nothing to say")
	}
}

func TestFieldKeyLimit(t *testing.T) {
	cases := []struct {
		in   string
		want int
		ok   bool
	}{
		{"quotes", 0, false},
		{`quotes({"pagination":{"limit":1}})`, 1, true},
		{`reviews({"limit":30})`, 30, true},
		{`reviews({"pagination":{"limit":0}})`, 0, true},
		{`getBookByLegacyId({"legacyId":"2767052"})`, 0, false},
	}
	for _, c := range cases {
		got, ok := ParseFieldKey(c.in).Limit()
		if got != c.want || ok != c.ok {
			t.Errorf("Limit(%q) = (%d, %v), want (%d, %v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

// TestResolveHandlesTheRealCycle walks the book/work/bestBook triangle, which
// is on every book page, and asserts it terminates with a stub rather than
// recursing.
func TestResolveHandlesTheRealCycle(t *testing.T) {
	cache := bookCache(t)
	key, ok := cache.RootRefKey("getBookByLegacyId")
	if !ok {
		t.Fatal("ROOT_QUERY does not name the book this page is about")
	}
	m, ok := cache.Resolve(cache[key]).(map[string]any)
	if !ok {
		t.Fatalf("resolved book is not an object")
	}
	if _, ok := m["title"]; !ok {
		t.Error("resolved book has no title")
	}
	// A cycle resolves to a Ref stub somewhere down the tree, never to an
	// endless nest. Round tripping through JSON is the cheap way to assert it
	// terminated and produced something serialisable.
	if _, err := json.Marshal(m); err != nil {
		t.Fatalf("resolved book will not marshal: %v", err)
	}
}

// TestResolveKeepsBothDescriptions holds the rule that two keys for the same
// field name are two pieces of data.
//
// description is the raw markup a renderer wants and
// description({"stripped":true}) is what a text pipeline wants, and deriving
// either from the other is lossy. A resolver that collapses them on name loses
// one silently.
func TestResolveKeepsBothDescriptions(t *testing.T) {
	cache := bookCache(t)
	key, ok := cache.RootRefKey("getBookByLegacyId")
	if !ok {
		t.Fatal("ROOT_QUERY does not name the book this page is about")
	}
	book, ok := cache.Entity(key)
	if !ok {
		t.Fatalf("%s is not in the cache", key)
	}
	var argKey string
	for f := range book {
		if fk := ParseFieldKey(f); fk.Name == "description" && len(fk.Args) > 0 {
			argKey = f
		}
	}
	if argKey == "" {
		t.Skip("this capture has only one description key")
	}

	raw, _ := json.Marshal(book)
	out, ok2 := cache.Resolve(raw).(map[string]any)
	if !ok2 {
		t.Fatal("resolve did not return an object")
	}
	if _, ok := out["description"]; !ok {
		t.Error("plain description was dropped")
	}
	if _, ok := out[argKey]; !ok {
		t.Errorf("%s was dropped, so the stripped text is gone", argKey)
	}
}

// TestResolveUnknownRefBecomesStub covers the normal case of a page holding a
// reference to something it did not include.
func TestResolveUnknownRefBecomesStub(t *testing.T) {
	cache := Apollo{}
	got := cache.Resolve(json.RawMessage(`{"__ref":"Work:kca://work/amzn1.gr.work.v1.abc"}`))
	ref, ok := got.(Ref)
	if !ok {
		t.Fatalf("got %T, want a Ref stub", got)
	}
	want := Ref{Type: "Work", ID: "kca://work/amzn1.gr.work.v1.abc", Key: "Work:kca://work/amzn1.gr.work.v1.abc"}
	if !reflect.DeepEqual(ref, want) {
		t.Errorf("stub = %+v, want %+v", ref, want)
	}
	if ref.Resolved {
		t.Error("an unresolved ref reported itself resolved")
	}
}

// TestResolveSharedEntityIsNotMistakenForACycle is the reason the visited set
// unmarks on the way out. Two fields pointing at the same author is a shared
// author, not a loop, and both should resolve.
func TestResolveSharedEntityIsNotMistakenForACycle(t *testing.T) {
	cache := Apollo{
		"Contributor:kca://author/x": json.RawMessage(`{"name":"Suzanne Collins"}`),
	}
	got := cache.Resolve(json.RawMessage(
		`{"a":{"__ref":"Contributor:kca://author/x"},"b":{"__ref":"Contributor:kca://author/x"}}`))
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("got %T, want an object", got)
	}
	for _, field := range []string{"a", "b"} {
		sub, ok := m[field].(map[string]any)
		if !ok {
			t.Fatalf("%s is %T, want the resolved author", field, m[field])
		}
		if sub["name"] != "Suzanne Collins" {
			t.Errorf("%s resolved to %v", field, sub)
		}
	}
}

func TestLegacyID(t *testing.T) {
	cache := bookCache(t)
	for _, k := range cache.Keys("Book") {
		e, _ := cache.Entity(k)
		if id, ok := LegacyID(e); ok && id > 0 {
			return
		}
	}
	t.Error("no Book in the capture yielded a legacy id, and the legacy id is what the URL uses")
}
