package goodread

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// The export runs off a real capture rather than a hand written node, because
// the point of writing the whole record out is the fields nothing here thought
// to ask for, and a fixture only has the fields somebody thought to write.
func exportStore(t *testing.T) (*Store, *Book) {
	t.Helper()
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
		t.Fatal(err)
	}
	return s, b
}

func TestExportJSONLWritesTheWholeRecord(t *testing.T) {
	s, b := exportStore(t)
	var buf bytes.Buffer
	if err := s.ExportJSONL(&buf, []string{"book"}); err != nil {
		t.Fatal(err)
	}

	sc := bufio.NewScanner(&buf)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	var lines int
	var found bool
	for sc.Scan() {
		lines++
		m := map[string]any{}
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatalf("line %d is not json: %v", lines, err)
		}
		if m["uri"] != "gr:book/2767052" {
			continue
		}
		found = true
		// The description is the test that this is the record and not the
		// columns, since no column holds it and a consumer that got only the
		// columns would silently be missing it.
		if m["description_stripped"] == nil && m["description"] == nil {
			t.Error("the exported line has no description, so it is the columns and not the record")
		}
		if m["title"] != b.Title {
			t.Errorf("title = %v, want %q", m["title"], b.Title)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatalf("the book is in the store and not in the export, %d lines", lines)
	}
}

// TestExportEdgesAreOneLineEach. Separate from the nodes, because a node line
// carrying its edges would repeat every one of them twice, once at each end.
func TestExportEdgesAreOneLineEach(t *testing.T) {
	s, _ := exportStore(t)
	var buf bytes.Buffer
	if err := s.ExportEdgesJSONL(&buf); err != nil {
		t.Fatal(err)
	}
	var n int
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var e Edge
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("edge line is not json: %v", err)
		}
		if _, ok := Predicates[e.Predicate]; !ok {
			t.Errorf("exported an edge nobody can query: %q", e.Predicate)
		}
		if e.SeenAt.IsZero() {
			t.Errorf("%s edge carries no timestamp", e.Predicate)
		}
		if e.Surface == "" {
			t.Errorf("%s edge does not say what surface it came from", e.Predicate)
		}
		n++
	}
	if n == 0 {
		t.Fatal("a book page names its work and its author and no edges were exported")
	}
}

// TestExportRDFMapsOntoSchemaOrg. The mapping in 04_graph.md section 6, and the
// rule behind it: schema.org where it fits, grv: where it does not, and nothing
// forced into a predicate that nearly means the right thing.
func TestExportRDFMapsOntoSchemaOrg(t *testing.T) {
	s, b := exportStore(t)
	var buf bytes.Buffer
	if err := s.ExportRDF(&buf, nil); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	for _, want := range []string{
		"@prefix schema:",
		"@prefix grv:",
		"gr:book-2767052",
		"a schema:Book",
		"schema:name",
		"schema:exampleOfWork",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the turtle has no %s in it", want)
		}
	}
	if b.ISBN13 != "" && !strings.Contains(out, "schema:isbn") {
		t.Error("the book has an isbn13 and the turtle has no schema:isbn")
	}
	if b.Stats != nil && b.Stats.AverageRating != nil && !strings.Contains(out, "schema:AggregateRating") {
		t.Error("the rating was flattened onto the book instead of onto an AggregateRating")
	}

	// Nothing is left half quoted. Descriptions have newlines and quotes in
	// them and a file that will not parse is worse than no file.
	if strings.Count(out, `"""`)%2 != 0 {
		t.Error("an unbalanced triple quoted string, so the turtle will not parse")
	}
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "schema:") && !strings.HasPrefix(trimmed, "grv:") {
			continue
		}
		// Predicate, object and the punctuation. A line with only the first of
		// those is an empty value that got written anyway.
		if len(strings.Fields(strings.TrimRight(trimmed, " ;."))) < 2 {
			t.Errorf("a predicate with no object: %q", line)
		}
	}
}

// TestExportKindsAreSingular, because that is how the model and the URIs spell
// a kind, and --kind book reads better than --kind books.
func TestExportKindsAreSingular(t *testing.T) {
	kinds := ExportKinds()
	for _, k := range kinds {
		if !IsNodeKind(k) {
			t.Errorf("%q is offered by --kind and has no table", k)
		}
	}
	if !contains(kinds, "series") || !contains(kinds, "book") || contains(kinds, "books") {
		t.Errorf("the kind list is %v", kinds)
	}
	if IsNodeKind("nonsense") {
		t.Error("nonsense is a node kind")
	}
}
