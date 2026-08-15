package goodread

import (
	"strconv"
	"testing"
	"time"
)

// The indexer runs against the real captures rather than against hand written
// json, because the whole risk in reading a record as a map is that a key is
// spelled differently from how this file assumes. A fixture written from the
// same assumption would agree with the bug.

func TestIndexBookCaptureBuildsTheGraph(t *testing.T) {
	e, err := ExtractBook(readCapture(t, "book_show_2767052.html.gz"))
	if err != nil {
		t.Fatalf("ExtractBook: %v", err)
	}
	b, err := BookFrom(e, time.Now().UTC())
	if err != nil {
		t.Fatalf("BookFrom: %v", err)
	}
	w, _ := WorkFrom(e, time.Now().UTC())

	s := testStore(t)
	if err := s.Put("book", "2767052", b.WebURL, &BookRecord{Book: b, Work: w}); err != nil {
		t.Fatalf("Put: %v", err)
	}

	if _, err := s.GetNode("book", "gr:book/2767052"); err != nil {
		t.Fatalf("the book did not land as a node: %v", err)
	}

	// The identifier index, which is what lookup runs on.
	if b.ISBN13 != "" {
		hits, err := s.LookupIdentifier(b.ISBN13)
		if err != nil || len(hits) != 1 {
			t.Errorf("LookupIdentifier(%q) = %+v, %v", b.ISBN13, hits, err)
		}
	}

	// Full text, which is what find runs on.
	hits, err := s.FindText("book", "hunger games", 10)
	if err != nil {
		t.Fatalf("FindText: %v", err)
	}
	if len(hits) == 0 {
		t.Error("the book is in the store and the full text index does not have it")
	}

	// The edges. A book page names its work and its author, and those are the
	// two joins that make the rest of the graph reachable.
	out, err := s.Edges("gr:book/2767052", false)
	if err != nil {
		t.Fatal(err)
	}
	var toWork bool
	for _, e := range out {
		if e.Predicate == "edition_of" {
			toWork = true
		}
	}
	if !toWork && w != nil {
		t.Errorf("no edge from the edition to its work: %+v", out)
	}

	in, err := s.Edges("gr:book/2767052", true)
	if err != nil {
		t.Fatal(err)
	}
	var wrote bool
	for _, e := range in {
		if e.Predicate == "wrote" {
			wrote = true
		}
	}
	if !wrote {
		t.Errorf("nobody wrote it: %+v", in)
	}
	if len(b.Contributors) > 0 {
		au := NodeURI("author", strconv.FormatInt(b.Contributors[0].LegacyID, 10))
		if _, err := s.GetNode("author", au); err != nil {
			t.Errorf("the author named on the book page is not a node: %v", err)
		}
	}
}

// TestIndexListPutsItsRowsInTheStore. Reading one Listopia page is a hundred
// books for one request, and a store that kept only the list record would make
// somebody spend a hundred more requests to find any of them again.
func TestIndexListPutsItsRowsInTheStore(t *testing.T) {
	e, err := ExtractList(readCapture(t, "list_show_1.html.gz"), "https://www.goodreads.com/list/show/1.Best_Books_Ever")
	if err != nil {
		t.Fatalf("ExtractList: %v", err)
	}
	l, err := ListFrom(e, "1.Best_Books_Ever", time.Now().UTC())
	if err != nil {
		t.Fatalf("ListFrom: %v", err)
	}
	if len(l.Books) == 0 {
		t.Fatal("the list capture parsed with no rows, so this test proves nothing")
	}

	s := testStore(t)
	if err := s.Put("list", l.ID, l.WebURL, l); err != nil {
		t.Fatal(err)
	}

	edges, err := s.Edges(NodeURI("list", l.ID), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) == 0 {
		t.Fatalf("the list contains %d books and no edges were written", len(l.Books))
	}
	if edges[0].Predicate != "contains" {
		t.Errorf("predicate = %q", edges[0].Predicate)
	}

	// And the rows are books in their own right, findable by title.
	title := l.Books[0].Title
	if title == "" {
		title = l.Books[0].Book.Title
	}
	if _, err := s.GetNode("book", edges[0].Dst); err != nil {
		t.Errorf("row %q is an edge to a book that is not stored: %v", title, err)
	}
}

// TestSlugIsStable is the contract 04_graph.md section 3 writes down. Changing
// slug renames every genre, place and character node in every store anybody has
// built, so this is here to make that a decision rather than an accident.
func TestSlugIsStable(t *testing.T) {
	for in, want := range map[string]string{
		"Fantasy":                 "fantasy",
		"Science Fiction":         "science-fiction",
		"  Young  Adult  ":        "young-adult",
		"Katniss Everdeen":        "katniss-everdeen",
		"Dublin, Ireland":         "dublin-ireland",
		"Gabriel García Márquez":  "gabriel-garcia-marquez",
		"Stanisław Lem":           "stanislaw-lem",
		"20th Century":            "20th-century",
		"Non-Fiction":             "non-fiction",
		"---":                     "",
		"L'Étranger":              "l-etranger",
		"Хроники":                 "хроники",
		"Mystery & Thriller":      "mystery-thriller",
		"The Hitchhiker's  Guide": "the-hitchhiker-s-guide",
	} {
		if got := slug(in); got != want {
			t.Errorf("slug(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestURIComesFromTheLegacyID. A record that only carries a kca id gets no URI
// at all, because a URI built from the kca id is one a second read of the same
// book through a Rails page would not agree with, and two URIs for one book is
// the failure the whole scheme exists to avoid.
func TestURIComesFromTheLegacyID(t *testing.T) {
	kcaOnly := map[string]any{"id": "kca://book/amzn1.gr.book.v1.YaoKZD8xVx72w5T1ZgR1YQ", "title": "x"}
	if got := uriOf("book", "", kcaOnly); got != "" {
		t.Errorf("uriOf on a kca only record = %q, want nothing", got)
	}
	both := map[string]any{"id": "kca://book/x", "legacy_id": float64(2767052)}
	if got := uriOf("book", "", both); got != "gr:book/2767052" {
		t.Errorf("uriOf = %q", got)
	}
	// A Rails surface puts the legacy id in the id field of a Ref, and that is
	// still a legacy id.
	if got := uriOf("author", "", map[string]any{"id": "153394"}); got != "gr:author/153394" {
		t.Errorf("uriOf on a rails ref = %q", got)
	}
	// A genre has no numeric id anywhere and its natural key is the slug.
	if got := uriOf("genre", "", map[string]any{"name": "Science Fiction"}); got != "gr:genre/science-fiction" {
		t.Errorf("uriOf on a genre = %q", got)
	}
}
