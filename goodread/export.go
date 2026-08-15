package goodread

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Export, per 04_graph.md section 6.
//
// Three shapes for three consumers, and two of them live here: JSONL is what
// the rest of the family reads, RDF is for a triple store, and SQLite is the
// store file itself, which needs no exporter because it is already the thing.

// ExportJSONL writes one node per line.
//
// The record and not the columns, because the columns are an index over the
// record and a consumer that got only them would be missing every field this
// version does not have a column for. One object per line rather than one big
// array, so a reader can stream a million of them and a truncated file is still
// readable up to the last complete line.
func (s *Store) ExportJSONL(w io.Writer, kinds []string) error {
	enc := json.NewEncoder(w)
	for _, table := range tablesFor(kinds) {
		rows, err := s.db.Query(`SELECT uri, json FROM ` + table + ` ORDER BY uri`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var uri, raw string
			if err := rows.Scan(&uri, &raw); err != nil {
				_ = rows.Close()
				return err
			}
			m := map[string]any{}
			if err := json.Unmarshal([]byte(raw), &m); err != nil {
				_ = rows.Close()
				return err
			}
			// The URI goes in even though it is not part of the record, because
			// a line that did not say what it was about would leave the reader
			// rebuilding it from the id and the table it no longer has.
			m["uri"] = uri
			if err := enc.Encode(m); err != nil {
				_ = rows.Close()
				return err
			}
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return err
		}
		_ = rows.Close()
	}
	return nil
}

// ExportEdgesJSONL writes the edge table, one edge per line.
func (s *Store) ExportEdgesJSONL(w io.Writer) error {
	enc := json.NewEncoder(w)
	rows, err := s.db.Query(`SELECT src,predicate,dst,props,surface,seen_at FROM edges ORDER BY src, predicate, dst`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var e Edge
		var props *string
		var at int64
		if err := rows.Scan(&e.Src, &e.Predicate, &e.Dst, &props, &e.Surface, &at); err != nil {
			return err
		}
		if props != nil {
			e.Props = json.RawMessage(*props)
		}
		e.SeenAt = time.Unix(at, 0).UTC()
		if err := enc.Encode(e); err != nil {
			return err
		}
	}
	return rows.Err()
}

func tablesFor(kinds []string) []string {
	if len(kinds) == 0 {
		return nodeTables
	}
	var out []string
	seen := map[string]bool{}
	for _, k := range kinds {
		t, err := tableFor(k)
		if err != nil || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

// The RDF vocabulary.
//
// schema.org where it fits and grv: where it does not, and the line between
// them is drawn on purpose: a wrong predicate is worse than a local one,
// because a triple store will happily join on it and quietly produce nonsense.
const rdfPrefixes = `@prefix schema: <https://schema.org/> .
@prefix grv:    <https://goodread.cli/vocab#> .
@prefix gr:     <https://goodread.cli/id/> .
@prefix xsd:    <http://www.w3.org/2001/XMLSchema#> .

`

// edgePredicates maps our edges onto the vocabulary.
//
// exampleOfWork is the one that matters. It is schema.org's own edition versus
// work distinction and it lines up exactly with ours, which is the argument for
// keeping book and work apart in the first place.
var edgePredicates = map[string]string{
	EdgeEditionOf:   "schema:exampleOfWork",
	EdgeBestEdition: "schema:workExample",
	EdgeContributed: "schema:author",
	EdgeInSeries:    "schema:isPartOf",
	EdgeShelvedAs:   "schema:genre",
	EdgeListedIn:    "grv:listedIn",
	EdgeSetIn:       "grv:setIn",
	EdgeFeatures:    "grv:features",
	EdgeWon:         "schema:award",
	EdgeQuotedFrom:  "grv:quotedFrom",
	EdgeReviewed:    "grv:reviewed",
	EdgeShelved:     "grv:shelved",
}

// ExportRDF writes the graph as Turtle.
//
// Turtle rather than N-Triples, because the aggregate rating needs a blank node
// and spelling those out one triple at a time is unreadable for no gain.
func (s *Store) ExportRDF(w io.Writer, kinds []string) error {
	if _, err := io.WriteString(w, rdfPrefixes); err != nil {
		return err
	}
	for _, table := range tablesFor(kinds) {
		rows, err := s.db.Query(`SELECT uri, json FROM ` + table + ` ORDER BY uri`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var uri, raw string
			if err := rows.Scan(&uri, &raw); err != nil {
				_ = rows.Close()
				return err
			}
			m := map[string]any{}
			if err := json.Unmarshal([]byte(raw), &m); err != nil {
				continue
			}
			if _, err := io.WriteString(w, turtleNode(uri, m)); err != nil {
				_ = rows.Close()
				return err
			}
		}
		_ = rows.Close()
	}

	edges, err := s.db.Query(`SELECT src,predicate,dst FROM edges ORDER BY src, predicate, dst`)
	if err != nil {
		return err
	}
	defer func() { _ = edges.Close() }()
	for edges.Next() {
		var src, pred, dst string
		if err := edges.Scan(&src, &pred, &dst); err != nil {
			return err
		}
		p, ok := edgePredicates[pred]
		if !ok {
			continue
		}
		if _, err := fmt.Fprintf(w, "%s %s %s .\n", turtleURI(src), p, turtleURI(dst)); err != nil {
			return err
		}
	}
	return edges.Err()
}

// turtleNode writes the literal fields of one node.
func turtleNode(uri string, m map[string]any) string {
	subject := turtleURI(uri)
	var lines []string
	add := func(pred, object string) {
		if object != "" {
			lines = append(lines, "  "+pred+" "+object)
		}
	}

	add("a", classOf(KindOf(uri)))
	add("schema:name", turtleString(titleOf(m)))
	add("schema:isbn", turtleString(keyStr(m, "isbn13")))
	add("schema:description", turtleString(firstKey(m, "description_stripped", "description")))
	if n := keyNum(m, "num_pages"); n > 0 {
		add("schema:numberOfPages", strconv.Itoa(int(n)))
	}
	add("schema:publisher", turtleString(keyStr(m, "publisher")))
	if t := keyTime(m, "publication_time"); !t.IsZero() {
		add("schema:datePublished", `"`+t.UTC().Format("2006-01-02")+`"^^xsd:date`)
	}
	add("schema:url", turtleIRI(keyStr(m, "web_url")))

	// The rating goes on a blank node, because schema.org puts the value and
	// the count on an AggregateRating and not on the book. Flattening it would
	// be the forced fit this file exists to avoid.
	if st, ok := m["stats"].(map[string]any); ok {
		avg, count := keyNum(st, "average_rating"), keyNum(st, "ratings_count")
		if avg > 0 || count > 0 {
			add("schema:aggregateRating", fmt.Sprintf(
				"[ a schema:AggregateRating ; schema:ratingValue %s ; schema:ratingCount %d ]",
				strconv.FormatFloat(avg, 'f', -1, 64), int64(count)))
		}
		// The distribution is five numbers in rating order and nothing in
		// schema.org holds it, so it stays a list under grv: rather than being
		// spread over five predicates nobody could put back together.
		if dist := distOf(st); dist != "" {
			add("grv:ratingDistribution", dist)
		}
	}

	if len(lines) == 0 {
		return ""
	}
	return subject + "\n" + strings.Join(lines, " ;\n") + " .\n\n"
}

func distOf(stats map[string]any) string {
	raw, ok := stats["ratings_count_dist"].([]any)
	if !ok || len(raw) == 0 {
		return ""
	}
	parts := make([]string, 0, len(raw))
	for _, v := range raw {
		if n, ok := v.(float64); ok {
			parts = append(parts, strconv.FormatInt(int64(n), 10))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "( " + strings.Join(parts, " ") + " )"
}

func classOf(kind string) string {
	switch kind {
	case "book":
		return "schema:Book"
	case "work":
		return "schema:CreativeWork"
	case "author", "user":
		return "schema:Person"
	case "series":
		return "schema:BookSeries"
	case "genre", "list":
		return "schema:CollectionPage"
	case "quote":
		return "schema:Quotation"
	case "place":
		return "schema:Place"
	case "":
		return ""
	default:
		// grv: rather than a schema.org class that nearly fits. A Character is
		// not a Person and an Award is not an Organization.
		return "grv:" + strings.ToUpper(kind[:1]) + kind[1:]
	}
}

// turtleURI writes a gr: URI as a prefixed name.
//
// The local part is percent escaped for the characters Turtle would otherwise
// read as syntax. Legacy ids are digits and slugs are hyphenated ascii, so this
// almost never fires, and the one time it does is worth not producing a file
// that will not parse.
func turtleURI(uri string) string {
	rest, ok := strings.CutPrefix(uri, "gr:")
	if !ok {
		return turtleIRI(uri)
	}
	return "gr:" + strings.NewReplacer("/", "-", " ", "%20", ".", "%2E", "#", "%23").Replace(rest)
}

func turtleIRI(u string) string {
	if u == "" || !strings.Contains(u, "://") {
		return ""
	}
	return "<" + strings.NewReplacer("<", "%3C", ">", "%3E", `"`, "%22", " ", "%20").Replace(u) + ">"
}

func turtleString(s string) string {
	if s == "" {
		return ""
	}
	// Long strings get the triple quoted form, which is the only one that can
	// hold a description with a newline in it, and descriptions have newlines.
	if strings.ContainsAny(s, "\n\r") {
		return `"""` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"""`, `\"\"\"`) + `"""`
	}
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\t", `\t`).Replace(s) + `"`
}

// RDFPredicates lists the edge to predicate mapping, for the docs and for the
// test that says every edge has one.
func RDFPredicates() map[string]string {
	out := make(map[string]string, len(edgePredicates))
	for k, v := range edgePredicates {
		out[k] = v
	}
	return out
}

// ExportKinds is what --kind accepts, in a stable order.
//
// Singular, because that is how the model and the URIs spell a kind and the
// plural is only ever the name of the table it lands in.
func ExportKinds() []string {
	out := append([]string(nil), nodeKinds...)
	sort.Strings(out)
	return out
}

// IsNodeKind says whether a kind names a node table.
func IsNodeKind(kind string) bool {
	_, err := tableFor(kind)
	return err == nil
}
