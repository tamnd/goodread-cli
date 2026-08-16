package goodread

import (
	"context"
	"strconv"
	"strings"
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
	toWork := false
	byAuthor := map[string]string{}
	for _, e := range out {
		switch e.Predicate {
		case EdgeEditionOf:
			toWork = true
		case EdgeContributed:
			byAuthor[e.Dst] = string(e.Props)
		}
	}
	if !toWork && w != nil {
		t.Errorf("no edge from the edition to its work: %+v", out)
	}
	if len(byAuthor) == 0 {
		t.Errorf("nobody wrote it: %+v", out)
	}
	// The role rides on the edge, which is the point of the edge carrying
	// properties at all: an illustrator recorded as the author is a wrong fact
	// that spreads everywhere the author does.
	for _, c := range b.Contributors {
		if c.Role == "" || c.LegacyID == 0 {
			continue
		}
		dst := NodeURI("author", strconv.FormatInt(c.LegacyID, 10))
		if props, ok := byAuthor[dst]; !ok {
			t.Errorf("%s contributed and has no edge", c.Name)
		} else if !strings.Contains(props, c.Role) {
			t.Errorf("the edge to %s does not carry the role %q: %s", c.Name, c.Role, props)
		}
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

	// The edge runs book to list, so the list reads it as an incoming one.
	edges, err := s.Edges(NodeURI("list", l.ID), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) == 0 {
		t.Fatalf("the list contains %d books and no edges were written", len(l.Books))
	}
	var ranked int
	for _, e := range edges {
		if e.Predicate != EdgeListedIn {
			t.Errorf("predicate = %q", e.Predicate)
			break
		}
		// The position is on the edge and not on the book, because a book sits
		// at rank 3 on one list and rank 900 on another and the rank belongs to
		// neither of them alone.
		if strings.Contains(string(e.Props), `"position"`) {
			ranked++
		}
	}
	if ranked != len(edges) {
		t.Errorf("%d of %d rows carry their position", ranked, len(edges))
	}

	// And the rows are books in their own right, findable by title.
	title := l.Books[0].Title
	if title == "" {
		title = l.Books[0].Book.Title
	}
	if _, err := s.GetNode("book", edges[0].Src); err != nil {
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

// TestBothIDSpacesSurviveTheRoundTrip. 04_graph.md section 1: the legacy id and
// the kca id are both real and neither is derivable from the other. A store that
// kept one of them would be a store that could only be joined one way, so this
// reads them back out of the columns rather than trusting the record.
func TestBothIDSpacesSurviveTheRoundTrip(t *testing.T) {
	e, err := ExtractBook(readCapture(t, "book_show_2767052.html.gz"))
	if err != nil {
		t.Fatalf("ExtractBook: %v", err)
	}
	b, err := BookFrom(e, time.Now().UTC())
	if err != nil {
		t.Fatalf("BookFrom: %v", err)
	}
	if b.ID == "" || b.LegacyID == 0 {
		t.Fatalf("the capture parsed with id=%q legacy_id=%d, so this test proves nothing", b.ID, b.LegacyID)
	}

	s := testStore(t)
	w, _ := WorkFrom(e, time.Now().UTC())
	if err := s.Put("book", "2767052", b.WebURL, &BookRecord{Book: b, Work: w}); err != nil {
		t.Fatal(err)
	}

	rows, err := s.Query(context.Background(), `select id, legacy_id from books where uri = 'gr:book/2767052'`, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("%d rows", len(rows))
	}
	if got := rows[0].Cells()["id"]; got != b.ID {
		t.Errorf("the kca id came back as %q, want %q", got, b.ID)
	}
	if got := rows[0].Cells()["legacy_id"]; got != strconv.FormatInt(b.LegacyID, 10) {
		t.Errorf("the legacy id came back as %q, want %d", got, b.LegacyID)
	}
	// And the URI is built from the legacy one, never the kca one, because a
	// kca id is not what a link or a fifteen year old dataset carries.
	if !strings.HasSuffix(NodeURI("book", strconv.FormatInt(b.LegacyID, 10)), "/2767052") {
		t.Error("the uri is not the legacy id")
	}
}

// TestIndexSearchRecordFoldsItsRows is the acceptance line 3008 writes down: a
// search read folds into the store as books, authors and works with
// contributed_by and edition_of.
//
// The suggest endpoint is the one worth holding with a test, because it is the
// only surface that hands over a book id and its work id in the same response.
// If that stopped folding, the graph would quietly lose the cheapest edition_of
// it has and nothing else would complain.
func TestIndexSearchRecordFoldsItsRows(t *testing.T) {
	body := readCapture(t, "book_auto_complete_hunger_games.json.gz")
	e, err := ExtractSuggest(body, SearchAutocompleteURL("hunger games"), "hunger games")
	if err != nil {
		t.Fatalf("ExtractSuggest: %v", err)
	}
	rec, err := SuggestFrom(e, SearchQuery{Query: "hunger games"}, time.Now().UTC())
	if err != nil {
		t.Fatalf("SuggestFrom: %v", err)
	}
	if len(rec.Results) == 0 {
		t.Fatal("the capture parsed with no rows, so this test proves nothing")
	}

	s := testStore(t)
	if err := s.Put("search", rec.Query, rec.WebURL, rec); err != nil {
		t.Fatal(err)
	}

	for _, h := range rec.Results {
		bu := NodeURI("book", h.Book.ID)
		if _, err := s.GetNode("book", bu); err != nil {
			t.Fatalf("row %q is not a book node: %v", h.Title, err)
		}
		out, err := s.Edges(bu, false)
		if err != nil {
			t.Fatal(err)
		}
		var toWork, byAuthor bool
		for _, ed := range out {
			switch ed.Predicate {
			case EdgeEditionOf:
				if h.Work != nil && ed.Dst == NodeURI("work", h.Work.ID) {
					toWork = true
				}
			case EdgeContributed:
				byAuthor = true
			}
		}
		if h.Work != nil && !toWork {
			t.Errorf("%q names work %s and there is no edition_of edge: %+v", h.Title, h.Work.ID, out)
		}
		if len(h.Contributors) > 0 && !byAuthor {
			t.Errorf("%q was written by %s and there is no contributed_by edge", h.Title, h.Contributors[0].Name)
		}
	}

	// The work and the author land as nodes of their own, which is what makes
	// them reachable without another request.
	first := rec.Results[0]
	if first.Work != nil {
		if _, err := s.GetNode("work", NodeURI("work", first.Work.ID)); err != nil {
			t.Errorf("the work named on the row is not a node: %v", err)
		}
	}
	if len(first.Contributors) > 0 {
		au := NodeURI("author", strconv.FormatInt(first.Contributors[0].LegacyID, 10))
		if _, err := s.GetNode("author", au); err != nil {
			t.Errorf("the author named on the row is not a node: %v", err)
		}
	}
}
