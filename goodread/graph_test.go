package goodread

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenStore(filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// TestQuickReadDoesNotDeleteTheEditionsList is the test 04_graph.md section 7
// asks for by name.
//
// The failure it guards against does not look like a failure. A full read
// stores forty editions, a quick read of the same book comes back with no
// editions key at all because it never asked, and a store that replaced rather
// than merged now has a book with no editions and no sign anything was lost.
// Over a long crawl that eats the whole dataset one row at a time.
func TestQuickReadDoesNotDeleteTheEditionsList(t *testing.T) {
	s := testStore(t)
	now := time.Now().UTC()

	full := []byte(`{
	  "kind":"book","legacy_id":2767052,"title":"The Hunger Games",
	  "isbn13":"9780439023481","num_pages":374,
	  "editions":[{"isbn13":"9780439023481"},{"isbn13":"9781407132082"}],
	  "stats":{"average_rating":4.35,"ratings_count":9811629,"ratings_count_dist":[1,2,3,4,5]}
	}`)
	if err := s.PutNode(Node{
		URI: "gr:book/2767052", Kind: "book", LegacyID: 2767052, Title: "The Hunger Games",
		ISBN13: "9780439023481", NumPages: 374, JSON: full, Surfaces: []string{"s1"}, RetrievedAt: now,
	}); err != nil {
		t.Fatalf("PutNode full: %v", err)
	}

	// The quick read. No editions key, a stats block with only the average in
	// it, and a newer title.
	quick := []byte(`{
	  "kind":"book","legacy_id":2767052,"title":"The Hunger Games (The Hunger Games, #1)",
	  "stats":{"average_rating":4.36}
	}`)
	if err := s.PutNode(Node{
		URI: "gr:book/2767052", Kind: "book", LegacyID: 2767052,
		Title: "The Hunger Games (The Hunger Games, #1)",
		JSON:  quick, Surfaces: []string{"s4"}, RetrievedAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("PutNode quick: %v", err)
	}

	raw, err := s.GetNode("book", "gr:book/2767052")
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}

	eds, _ := got["editions"].([]any)
	if len(eds) != 2 {
		t.Errorf("editions = %v after a quick read, and the full read had two", got["editions"])
	}
	if got["isbn13"] != "9780439023481" {
		t.Errorf("isbn13 = %v, and the quick read never looked at it", got["isbn13"])
	}
	if got["num_pages"] != float64(374) {
		t.Errorf("num_pages = %v", got["num_pages"])
	}
	// Newest non absent wins, so the newer title does replace the older one.
	if got["title"] != "The Hunger Games (The Hunger Games, #1)" {
		t.Errorf("title = %v, and the newer read should win where it said something", got["title"])
	}

	stats, _ := got["stats"].(map[string]any)
	if stats["average_rating"] != 4.36 {
		t.Errorf("average = %v, and the newer read said 4.36", stats["average_rating"])
	}
	if dist, _ := stats["ratings_count_dist"].([]any); len(dist) != 5 {
		t.Errorf("the histogram was lost inside a nested object: %v", stats["ratings_count_dist"])
	}
	if stats["ratings_count"] != float64(9811629) {
		t.Errorf("ratings_count = %v, and a partial stats block should not wipe the rest", stats["ratings_count"])
	}
}

// TestMergeKeepsAbsentAndEmptyApart. json spells "I did not look" as an absent
// key or a null, and 03_model.md section 8 is built on those not being the same
// as a field the surface published as empty.
func TestMergeKeepsAbsentAndEmptyApart(t *testing.T) {
	for _, c := range []struct {
		name, old, new, want string
	}{
		{"absent key keeps the old value", `{"a":1}`, `{}`, `1`},
		{"an explicit null is not an answer", `{"a":1}`, `{"a":null}`, `1`},
		{"an empty list loses to a full one", `{"a":[1,2]}`, `{"a":[]}`, `[1,2]`},
		{"an empty string loses too", `{"a":"x"}`, `{"a":""}`, `"x"`},
		{"a real value wins", `{"a":1}`, `{"a":2}`, `2`},
		{"false is a real value", `{"a":true}`, `{"a":false}`, `false`},
		{"zero is a real value", `{"a":9}`, `{"a":0}`, `0`},
	} {
		merged, err := MergeRecords([]byte(c.old), []byte(c.new))
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal(merged, &m); err != nil {
			t.Fatal(err)
		}
		if string(m["a"]) != c.want {
			t.Errorf("%s: a = %s, want %s", c.name, m["a"], c.want)
		}
	}
}

// TestMergeKeepsEverySurfaceItWasSeenOn. A book read from its own page and then
// seen in a Listopia row was read from two surfaces, and a record that named
// only the most recent one would understate what is behind it.
func TestMergeKeepsEverySurfaceItWasSeenOn(t *testing.T) {
	s := testStore(t)
	now := time.Now().UTC()
	body := []byte(`{"legacy_id":1,"surfaces":["s1"]}`)
	if err := s.PutNode(Node{URI: "gr:book/1", Kind: "book", JSON: body, Surfaces: []string{"s1"}, RetrievedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutNode(Node{URI: "gr:book/1", Kind: "book", JSON: []byte(`{"legacy_id":1,"surfaces":["s4"]}`), Surfaces: []string{"s4"}, RetrievedAt: now}); err != nil {
		t.Fatal(err)
	}
	raw, err := s.GetNode("book", "gr:book/1")
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Surfaces []string `json:"surfaces"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Surfaces) != 2 {
		t.Errorf("surfaces = %v, and it has been read from two", got.Surfaces)
	}
}

// TestLookupHitsTheIdentifierIndex. Site search is disallowed and this is the
// replacement, so it has to answer from the local store with no request at all.
func TestLookupHitsTheIdentifierIndex(t *testing.T) {
	s := testStore(t)
	now := time.Now().UTC()
	if err := s.PutNode(Node{
		URI: "gr:book/2767052", Kind: "book", LegacyID: 2767052, Title: "The Hunger Games",
		ISBN13: "9780439023481", ASIN: "B002MQYOFW",
		JSON: []byte(`{"legacy_id":2767052}`), RetrievedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	for _, id := range []string{"9780439023481", "B002MQYOFW"} {
		hits, err := s.LookupIdentifier(id)
		if err != nil {
			t.Fatalf("LookupIdentifier(%q): %v", id, err)
		}
		if len(hits) != 1 || hits[0].ID != "gr:book/2767052" {
			t.Errorf("LookupIdentifier(%q) = %+v", id, hits)
		}
		if hits[0].URL == "" {
			t.Errorf("LookupIdentifier(%q) returned no url, and the id alone is a lookup somebody else has to do", id)
		}
	}
	hits, err := s.LookupIdentifier("9999999999999")
	if err != nil || len(hits) != 0 {
		t.Errorf("an unknown isbn returned %+v, %v", hits, err)
	}
}

// TestFindRanksAndStems is what `goodread find` promises over what the LIKE in
// store.go does. A substring match cannot rank and cannot stem, and calling
// both of them search would set the wrong expectation about what comes back.
func TestFindRanksAndStems(t *testing.T) {
	s := testStore(t)
	now := time.Now().UTC()
	for _, n := range []Node{
		{URI: "gr:book/1", Kind: "book", Title: "The Hunger Games", Description: "Katniss volunteers for the games", AuthorName: "Suzanne Collins", JSON: []byte(`{}`), RetrievedAt: now},
		{URI: "gr:book/2", Kind: "book", Title: "Catching Fire", Description: "the second hunger games book", AuthorName: "Suzanne Collins", JSON: []byte(`{}`), RetrievedAt: now},
		{URI: "gr:author/153394", Kind: "author", Title: "Suzanne Collins", Description: "wrote the hunger games", JSON: []byte(`{}`), RetrievedAt: now},
	} {
		if err := s.PutNode(n); err != nil {
			t.Fatal(err)
		}
	}

	hits, err := s.FindText("", "hunger games", 10)
	if err != nil {
		t.Fatalf("FindText: %v", err)
	}
	if len(hits) != 3 {
		t.Errorf("%d hits for two words that appear in all three records", len(hits))
	}
	// Kind narrows it, because a graph with authors and books in it answers
	// "hunger games" with both and usually somebody meant one.
	books, err := s.FindText("book", "hunger games", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 2 {
		t.Errorf("%d book hits, want 2", len(books))
	}
	// Stemming, which is the thing a LIKE cannot do.
	if got, _ := s.FindText("", "volunteer", 10); len(got) != 1 {
		t.Errorf("a search for volunteer did not reach volunteers: %+v", got)
	}
	// And an apostrophe does not come back as an fts5 syntax error.
	if _, err := s.FindText("", "it's a \"quoted\" title-ish thing", 10); err != nil {
		t.Errorf("punctuation in the query was read as query syntax: %v", err)
	}
}

// TestReindexOnRetitle. fts5 has no upsert, so a node stored twice under
// different titles would answer to both unless the old row is deleted.
func TestReindexOnRetitle(t *testing.T) {
	s := testStore(t)
	now := time.Now().UTC()
	if err := s.PutNode(Node{URI: "gr:book/1", Kind: "book", Title: "Wrong Title", JSON: []byte(`{}`), RetrievedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutNode(Node{URI: "gr:book/1", Kind: "book", Title: "Right Title", JSON: []byte(`{}`), RetrievedAt: now}); err != nil {
		t.Fatal(err)
	}
	if hits, _ := s.FindText("", "wrong", 10); len(hits) != 0 {
		t.Errorf("the old title still answers: %+v", hits)
	}
	if hits, _ := s.FindText("", "right", 10); len(hits) != 1 {
		t.Errorf("the new title does not answer: %+v", hits)
	}
}

// TestEdgesRunBothWays. The useful questions go each direction: what did this
// author write, and who wrote this book.
func TestEdgesRunBothWays(t *testing.T) {
	s := testStore(t)
	now := time.Now().UTC()
	if err := s.PutEdge("gr:author/153394", "wrote", "gr:book/2767052", map[string]string{"role": "Author"}, "s1", now); err != nil {
		t.Fatal(err)
	}
	out, err := s.Edges("gr:author/153394", false)
	if err != nil || len(out) != 1 {
		t.Fatalf("outgoing = %+v, %v", out, err)
	}
	if string(out[0].Props) == "" {
		t.Error("the role was dropped, and it is the difference between an author and an illustrator")
	}
	in, err := s.Edges("gr:book/2767052", true)
	if err != nil || len(in) != 1 {
		t.Fatalf("incoming = %+v, %v", in, err)
	}

	// Written again with no props. The props that were there stay, same rule as
	// the record merge: a later read that did not look does not erase.
	if err := s.PutEdge("gr:author/153394", "wrote", "gr:book/2767052", nil, "s4", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	out, _ = s.Edges("gr:author/153394", false)
	if len(out) != 1 {
		t.Fatalf("the edge was duplicated: %+v", out)
	}
	if string(out[0].Props) == "" {
		t.Error("a later read with no props erased the props")
	}
}
