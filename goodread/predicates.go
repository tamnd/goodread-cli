package goodread

import "sort"

// The edge set, from 04_graph.md section 4.
//
// Twelve, and no thirteenth without a line in the spec. The names are the
// spec's rather than the ones a Go author would reach for, because an edge
// table is a published interface: somebody writes a query against
// `predicate='contributed_by'` and a rename breaks it silently, returning zero
// rows rather than an error.
//
// The direction is the spec's too, and it matters more than it looks.
// `contributed_by` runs book to author, not author to book. Both directions are
// answerable, since Edges reads either way, and having one canonical direction
// is what stops the same fact being stored twice under two names.
const (
	EdgeEditionOf   = "edition_of"     // book -> work
	EdgeBestEdition = "best_edition"   // work -> book
	EdgeContributed = "contributed_by" // book -> author, carries the role
	EdgeInSeries    = "in_series"      // book -> series, carries the position
	EdgeShelvedAs   = "shelved_as"     // book -> genre
	EdgeListedIn    = "listed_in"      // book -> list
	EdgeSetIn       = "set_in"         // work -> place
	EdgeFeatures    = "features"       // work -> character
	EdgeWon         = "won"            // work -> award
	EdgeQuotedFrom  = "quoted_from"    // quote -> work
	EdgeReviewed    = "reviewed"       // user -> book
	EdgeShelved     = "shelved"        // user -> book
)

// Predicate is one row of the edge table in the spec.
type Predicate struct {
	Name     string
	From     string
	To       string
	Surfaces []string

	// Props names what this edge carries beyond its endpoints, empty for the
	// ones that carry nothing. Two edges have properties and they are the two
	// where the fact lives on the edge rather than on either end: an
	// illustrator recorded as an author is a wrong fact that propagates
	// everywhere, and a novella at position 2.5 is not book 2.
	Props []string
}

// Predicates is the whole edge set, keyed by name.
var Predicates = map[string]Predicate{
	EdgeEditionOf:   {EdgeEditionOf, "book", "work", []string{"s1"}, nil},
	EdgeBestEdition: {EdgeBestEdition, "work", "book", []string{"s1", "s6"}, nil},
	EdgeContributed: {EdgeContributed, "book", "author", []string{"s1"}, []string{"role"}},
	EdgeInSeries:    {EdgeInSeries, "book", "series", []string{"s1", "s3"}, []string{"position", "number"}},
	EdgeShelvedAs:   {EdgeShelvedAs, "book", "genre", []string{"s1", "s5"}, nil},
	EdgeListedIn:    {EdgeListedIn, "book", "list", []string{"s4"}, []string{"position"}},
	EdgeSetIn:       {EdgeSetIn, "work", "place", []string{"s1"}, nil},
	EdgeFeatures:    {EdgeFeatures, "work", "character", []string{"s1"}, nil},
	EdgeWon:         {EdgeWon, "work", "award", []string{"s1"}, []string{"category", "year"}},
	EdgeQuotedFrom:  {EdgeQuotedFrom, "quote", "work", []string{"s7"}, nil},
	EdgeReviewed:    {EdgeReviewed, "user", "book", []string{"s1", "s12"}, []string{"rating"}},
	EdgeShelved:     {EdgeShelved, "user", "book", []string{"s1", "s11"}, []string{"shelf", "rating"}},
}

// PredicateNames lists the edge set in a stable order.
func PredicateNames() []string {
	out := make([]string, 0, len(Predicates))
	for k := range Predicates {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
