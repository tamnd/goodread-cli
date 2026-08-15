package goodread

import (
	"strings"
	"testing"
	"time"
)

// TestTheEdgeSetIsTheSpecsTwelve. 04_graph.md section 4 names twelve edges and
// the store is allowed to write those and nothing else. A thirteenth predicate
// is not a new fact, it is a typo or a rename, and either way it is a row that
// no query anybody has written will ever come back from.
func TestTheEdgeSetIsTheSpecsTwelve(t *testing.T) {
	want := []string{
		"best_edition", "contributed_by", "edition_of", "features", "in_series",
		"listed_in", "quoted_from", "reviewed", "set_in", "shelved", "shelved_as", "won",
	}
	got := PredicateNames()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("the edge set is\n  %v\nand the spec says\n  %v", got, want)
	}

	// Every one of them names endpoints that are node kinds, because an edge to
	// a kind with no table is one PutNode cannot land the far end of.
	for _, name := range got {
		p := Predicates[name]
		if !IsNodeKind(p.From) || !IsNodeKind(p.To) {
			t.Errorf("%s runs %s to %s and one of those is not a node kind", name, p.From, p.To)
		}
		if len(p.Surfaces) == 0 {
			t.Errorf("%s says nothing about where it comes from", name)
		}
	}

	// The two the spec calls out by name. The role is the difference between an
	// author and an illustrator and the position is the difference between book
	// two and the novella between two and three.
	if !contains(Predicates[EdgeContributed].Props, "role") {
		t.Error("contributed_by does not carry the role")
	}
	if !contains(Predicates[EdgeInSeries].Props, "position") {
		t.Error("in_series does not carry the position")
	}
}

// TestAnUnknownPredicateIsRefused. The guard is at the write, where the name is
// still in front of somebody, rather than at the read, where it looks like the
// data is missing.
func TestAnUnknownPredicateIsRefused(t *testing.T) {
	s := testStore(t)
	err := s.PutEdge("gr:author/1", "wrote", "gr:book/1", nil, "s1", time.Now())
	if err == nil {
		t.Fatal("wrote was accepted and it is not one of the twelve")
	}
	if !strings.Contains(err.Error(), "contributed_by") {
		t.Errorf("the refusal does not say what to use instead: %v", err)
	}
}

// TestEveryEdgeHasAnRDFPredicate. 04_graph.md section 6 maps the graph onto
// schema.org where it fits and grv: where it does not, and an edge with no
// mapping at all would vanish from the export without a word.
func TestEveryEdgeHasAnRDFPredicate(t *testing.T) {
	rdf := RDFPredicates()
	for _, name := range PredicateNames() {
		p, ok := rdf[name]
		if !ok {
			t.Errorf("%s has no rdf predicate, so it would be dropped from the export", name)
			continue
		}
		if !strings.HasPrefix(p, "schema:") && !strings.HasPrefix(p, "grv:") {
			t.Errorf("%s maps to %q, which is neither schema.org nor the local vocabulary", name, p)
		}
	}
	// The one the spec argues for. It is schema.org's own edition versus work
	// distinction and it lines up exactly with ours.
	if rdf[EdgeEditionOf] != "schema:exampleOfWork" {
		t.Errorf("edition_of maps to %q, want schema:exampleOfWork", rdf[EdgeEditionOf])
	}
}
