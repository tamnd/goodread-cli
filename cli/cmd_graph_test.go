package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tamnd/goodread-cli/goodread"
)

// seedGraph puts a book, its work and its author in a store, with the edges
// between them, so the walk has something with a shape to walk.
func seedGraph(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "goodread.db")
	s, err := goodread.OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer func() { _ = s.Close() }()
	now := time.Now().UTC()
	for _, n := range []goodread.Node{
		{URI: "gr:book/2767052", Kind: "book", LegacyID: 2767052, Title: "The Hunger Games",
			ISBN13: "9780439023481", NumPages: 374,
			JSON: []byte(`{"kind":"book","legacy_id":2767052,"title":"The Hunger Games","isbn13":"9780439023481","num_pages":374,"description":"Katniss volunteers"}`), RetrievedAt: now},
		{URI: "gr:work/2792775", Kind: "work", LegacyID: 2792775, Title: "The Hunger Games",
			JSON: []byte(`{"kind":"work","legacy_id":2792775,"title":"The Hunger Games"}`), RetrievedAt: now},
		{URI: "gr:author/153394", Kind: "author", LegacyID: 153394, Title: "Suzanne Collins",
			JSON: []byte(`{"kind":"author","legacy_id":153394,"name":"Suzanne Collins"}`), RetrievedAt: now},
	} {
		if err := s.PutNode(n); err != nil {
			t.Fatalf("PutNode: %v", err)
		}
	}
	for _, e := range [][3]string{
		{"gr:book/2767052", goodread.EdgeEditionOf, "gr:work/2792775"},
		{"gr:work/2792775", goodread.EdgeBestEdition, "gr:book/2767052"},
		{"gr:book/2767052", goodread.EdgeContributed, "gr:author/153394"},
	} {
		if err := s.PutEdge(e[0], e[1], e[2], nil, "s1", now); err != nil {
			t.Fatalf("PutEdge: %v", err)
		}
	}
	return path
}

func TestGraphWalksTheLocalStore(t *testing.T) {
	store := seedGraph(t)

	// Default depth is 1, which 04_graph.md section 5 asks for, so the bare
	// command is never the expensive one.
	out, err := runCmd(t, store, "graph", "gr:book/2767052")
	if err != nil {
		t.Fatalf("graph: %v\n%s", err, out)
	}
	var nodes []struct {
		URI   string `json:"uri"`
		Kind  string `json:"kind"`
		Depth int    `json:"depth"`
	}
	if err := json.Unmarshal([]byte(out), &nodes); err != nil {
		t.Fatalf("output is not json: %v\n%s", err, out)
	}
	if len(nodes) != 3 {
		t.Fatalf("depth 1 from the book = %d nodes, want the book, its work and its author: %s", len(nodes), out)
	}
	for _, n := range nodes {
		if n.URI == "gr:book/2767052" && n.Depth != 0 {
			t.Errorf("the root came back at depth %d", n.Depth)
		}
	}

	// --edges prints the relationships instead, and they are the spec's names.
	out, err = runCmd(t, store, "graph", "gr:book/2767052", "--edges")
	if err != nil {
		t.Fatalf("graph --edges: %v\n%s", err, out)
	}
	for _, want := range []string{goodread.EdgeEditionOf, goodread.EdgeContributed} {
		if !strings.Contains(out, want) {
			t.Errorf("the edges do not include %s: %s", want, out)
		}
	}
}

// TestGraphWantsAURI. A bare id is ambiguous between a book and an author and
// guessing wrong walks somebody else's neighbourhood.
func TestGraphWantsAURI(t *testing.T) {
	store := seedGraph(t)
	out, err := runCmd(t, store, "graph", "2767052")
	if err == nil {
		t.Fatalf("a bare id was accepted: %s", out)
	}
	if got := exitCodeOf(err); got != exitUsage {
		t.Errorf("exit code %d, want %d", got, exitUsage)
	}
}

func TestExportWritesJSONL(t *testing.T) {
	store := seedGraph(t)
	out, err := runCmd(t, store, "export", "--kind", "book")
	if err != nil {
		t.Fatalf("export: %v\n%s", err, out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 1 {
		t.Fatalf("one book in the store and %d lines out: %s", len(lines), out)
	}
	m := map[string]any{}
	if err := json.Unmarshal([]byte(lines[0]), &m); err != nil {
		t.Fatalf("the line is not json: %v", err)
	}
	if m["uri"] != "gr:book/2767052" {
		t.Errorf("uri = %v", m["uri"])
	}
	// The record and not the columns. Nothing has a column for description and
	// a consumer that got only the columns would never know it was there.
	if m["description"] != "Katniss volunteers" {
		t.Errorf("the whole record did not come through: %v", m)
	}

	// --edges is the edge table, which is the other half of the graph.
	out, err = runCmd(t, store, "export", "--edges")
	if err != nil {
		t.Fatalf("export --edges: %v\n%s", err, out)
	}
	if n := len(strings.Split(strings.TrimSpace(out), "\n")); n != 3 {
		t.Errorf("%d edge lines for 3 edges: %s", n, out)
	}
}

func TestExportWritesTurtle(t *testing.T) {
	store := seedGraph(t)
	out, err := runCmd(t, store, "export", "--to", "rdf")
	if err != nil {
		t.Fatalf("export --to rdf: %v\n%s", err, out)
	}
	for _, want := range []string{"@prefix schema:", "a schema:Book", "schema:isbn", "schema:exampleOfWork"} {
		if !strings.Contains(out, want) {
			t.Errorf("the turtle has no %s in it:\n%s", want, out)
		}
	}

	// A format nobody implements is the user's to fix, not a failed run.
	if _, err := runCmd(t, store, "export", "--to", "xml"); err == nil {
		t.Error("--to xml was accepted")
	} else if got := exitCodeOf(err); got != exitUsage {
		t.Errorf("exit code %d, want %d", got, exitUsage)
	}

	// And so is a kind that is not one.
	if _, err := runCmd(t, store, "export", "--kind", "nonsense"); err == nil {
		t.Error("--kind nonsense was accepted")
	}
}
