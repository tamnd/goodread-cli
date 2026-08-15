package goodread

import (
	"testing"
	"time"
)

// seedWalk builds the cycle 04_graph.md section 5 warns about: the book is an
// edition of the work and the work's best edition is the book. Three of these
// tests exist because a walk that did not carry a visited set would not finish
// on this.
func seedWalk(t *testing.T) *Store {
	t.Helper()
	s := testStore(t)
	now := time.Now().UTC()
	for _, n := range []Node{
		{URI: "gr:book/2767052", Kind: "book", Title: "The Hunger Games", JSON: []byte(`{"title":"The Hunger Games"}`), RetrievedAt: now},
		{URI: "gr:work/2792775", Kind: "work", Title: "The Hunger Games", JSON: []byte(`{"title":"The Hunger Games"}`), RetrievedAt: now},
		{URI: "gr:author/153394", Kind: "author", Title: "Suzanne Collins", JSON: []byte(`{"name":"Suzanne Collins"}`), RetrievedAt: now},
		{URI: "gr:series/73758", Kind: "series", Title: "The Hunger Games", JSON: []byte(`{"title":"The Hunger Games"}`), RetrievedAt: now},
	} {
		if err := s.PutNode(n); err != nil {
			t.Fatal(err)
		}
	}
	for _, e := range [][3]string{
		{"gr:book/2767052", EdgeEditionOf, "gr:work/2792775"},
		{"gr:work/2792775", EdgeBestEdition, "gr:book/2767052"},
		{"gr:book/2767052", EdgeContributed, "gr:author/153394"},
		{"gr:book/2767052", EdgeInSeries, "gr:series/73758"},
		{"gr:book/2767052", EdgeShelvedAs, "gr:genre/young-adult"},
	} {
		if err := s.PutEdge(e[0], e[1], e[2], nil, "s1", now); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func TestWalkStopsAtTheDepthItWasGiven(t *testing.T) {
	s := seedWalk(t)

	// Depth 0 is the root alone, which is what makes this the same call the
	// command makes to show one node.
	g, err := s.Walk("gr:book/2767052", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Nodes) != 1 || len(g.Edges) != 0 {
		t.Errorf("depth 0 = %d nodes, %d edges", len(g.Nodes), len(g.Edges))
	}

	// Depth 1 from the book is the work, the author, the series and the genre.
	g, err = s.Walk("gr:book/2767052", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Nodes) != 5 {
		t.Errorf("depth 1 = %d nodes, want the book and its four neighbours: %+v", len(g.Nodes), g.Nodes)
	}
	for _, n := range g.Nodes {
		if n.URI == "gr:book/2767052" && n.Depth != 0 {
			t.Errorf("the root is at depth %d", n.Depth)
		}
		if n.URI == "gr:author/153394" && n.Depth != 1 {
			t.Errorf("the author is at depth %d", n.Depth)
		}
	}

	// From the author, one hop out reaches the book even though the edge points
	// the other way. Half the useful questions run backwards along an edge.
	g, err = s.Walk("gr:author/153394", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Nodes) != 2 {
		t.Errorf("the author's neighbourhood = %+v", g.Nodes)
	}
}

// TestWalkFinishesOnACycle. Book to Work to BestBook to Book closes in three
// hops, so a walk with no visited set does not return at all.
func TestWalkFinishesOnACycle(t *testing.T) {
	s := seedWalk(t)
	done := make(chan *Neighborhood, 1)
	go func() {
		g, err := s.Walk("gr:book/2767052", 6)
		if err != nil {
			t.Error(err)
		}
		done <- g
	}()
	select {
	case g := <-done:
		if len(g.Nodes) != 5 {
			t.Errorf("a cycle walked six deep found %d nodes, and there are five: %+v", len(g.Nodes), g.Nodes)
		}
		// Each edge once, however many ways round it was reached.
		if len(g.Edges) != 5 {
			t.Errorf("%d edges for 5 rows: %+v", len(g.Edges), g.Edges)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the walk did not return, which is the cycle")
	}
}

// TestWalkReportsWhatIsNotStoredYet. The genre has an edge and no record, and
// saying so is the most useful thing a walk can tell somebody deciding what to
// crawl next.
func TestWalkReportsWhatIsNotStoredYet(t *testing.T) {
	s := seedWalk(t)
	g, err := s.Walk("gr:book/2767052", 1)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, n := range g.Nodes {
		if n.URI != "gr:genre/young-adult" {
			continue
		}
		found = true
		if n.Kind != "genre" {
			t.Errorf("kind = %q, and the uri says genre", n.Kind)
		}
		if len(n.Record) != 0 {
			t.Errorf("a node nothing has fetched came back with a record: %s", n.Record)
		}
	}
	if !found {
		t.Error("the genre nothing has fetched was left out of the walk")
	}
}

// TestWalkSizeAgreesWithWalk. The gate refuses at 500 nodes based on the count,
// so a count that disagreed with the walk would refuse the wrong walks.
func TestWalkSizeAgreesWithWalk(t *testing.T) {
	s := seedWalk(t)
	for _, depth := range []int{0, 1, 2, 3} {
		n, err := s.WalkSize("gr:book/2767052", depth)
		if err != nil {
			t.Fatal(err)
		}
		g, err := s.Walk("gr:book/2767052", depth)
		if err != nil {
			t.Fatal(err)
		}
		if n != len(g.Nodes) {
			t.Errorf("depth %d: WalkSize says %d and Walk returns %d", depth, n, len(g.Nodes))
		}
	}
}

func TestKindComesOutOfTheURI(t *testing.T) {
	for uri, want := range map[string]string{
		"gr:book/2767052":       "book",
		"gr:genre/young-adult":  "genre",
		"gr:author/153394":      "author",
		"https://example.com/x": "",
		"2767052":               "",
		"gr:book":               "",
	} {
		if got := KindOf(uri); got != want {
			t.Errorf("KindOf(%q) = %q, want %q", uri, got, want)
		}
	}
}
