package goodread

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Walking the graph.
//
// 04_graph.md section 5 is the whole design here: cycles are normal, not an
// error case. Book -> Work -> BestBook -> Book closes in three hops on almost
// every popular title and Series -> Book -> Series closes in two, so the walk
// carries a visited set from the first hop rather than growing one after
// something goes wrong.

// GraphNode is one node as a traversal reports it.
//
// Depth is how many hops from the root it was first reached at, and first is
// the word that matters: the same node is reachable by several paths and the
// shortest one is the honest answer for how close it is.
type GraphNode struct {
	URI    string          `json:"uri"`
	Kind   string          `json:"kind"`
	Title  string          `json:"title,omitempty"`
	Depth  int             `json:"depth"`
	Record json.RawMessage `json:"record,omitempty"`
}

// Neighborhood is a root and everything within n hops of it.
type Neighborhood struct {
	Root  string      `json:"root"`
	Depth int         `json:"depth"`
	Nodes []GraphNode `json:"nodes"`
	Edges []Edge      `json:"edges"`
}

// KindOf reads the node type back out of a URI.
//
// The URI is gr:kind/id and the kind in it is the only kind the node has, so
// this is a parse and not a lookup. A caller holding a URI never needs a
// request to know what it points at, which is what makes an edge to a node
// nothing has fetched yet still useful.
func KindOf(uri string) string {
	rest, ok := strings.CutPrefix(uri, "gr:")
	if !ok {
		return ""
	}
	kind, _, ok := strings.Cut(rest, "/")
	if !ok {
		return ""
	}
	return kind
}

// GetNodeByURI reads a node without being told its kind.
func (s *Store) GetNodeByURI(uri string) ([]byte, error) {
	kind := KindOf(uri)
	if kind == "" {
		return nil, fmt.Errorf("%q is not a gr: uri", uri)
	}
	return s.GetNode(kind, uri)
}

// Walk returns everything within depth hops of uri.
//
// Both directions at every hop, because half the useful questions run backwards
// along an edge: what did this author write is the reverse of contributed_by,
// and a walk that only followed edges forwards would answer one of those and
// not the other.
//
// Depth 0 is the root alone, which is a legitimate thing to ask for and is what
// makes this the same call `goodread graph` makes with no flags to show one node.
func (s *Store) Walk(uri string, depth int) (*Neighborhood, error) {
	if uri == "" {
		return nil, fmt.Errorf("walk needs a uri to start from")
	}
	if depth < 0 {
		depth = 0
	}
	out := &Neighborhood{Root: uri, Depth: depth}

	seen := map[string]int{uri: 0}
	frontier := []string{uri}
	edges := map[string]Edge{}

	for hop := 1; hop <= depth && len(frontier) > 0; hop++ {
		var next []string
		for _, from := range frontier {
			for _, incoming := range []bool{false, true} {
				es, err := s.Edges(from, incoming)
				if err != nil {
					return nil, err
				}
				for _, e := range es {
					// Keyed the same way the table is, so an edge reached from
					// both ends is reported once rather than twice.
					edges[e.Src+"\x00"+e.Predicate+"\x00"+e.Dst] = e
					to := e.Dst
					if incoming {
						to = e.Src
					}
					if _, ok := seen[to]; ok {
						continue
					}
					seen[to] = hop
					next = append(next, to)
				}
			}
		}
		frontier = next
	}

	for u, d := range seen {
		n := GraphNode{URI: u, Kind: KindOf(u), Depth: d}
		if raw, err := s.GetNodeByURI(u); err == nil {
			n.Record = raw
			var m map[string]any
			if json.Unmarshal(raw, &m) == nil {
				n.Title = titleOf(m)
			}
		}
		// A node with no record is still reported. It is an edge to something
		// nobody has fetched yet, and that is the most useful thing a walk can
		// tell somebody deciding what to crawl next.
		out.Nodes = append(out.Nodes, n)
	}
	// Sorted, and by depth before URI, because the order a map ranges in is not
	// one and a walk that printed its nodes in a different order every run would
	// be useless to diff.
	sort.Slice(out.Nodes, func(i, j int) bool {
		if out.Nodes[i].Depth != out.Nodes[j].Depth {
			return out.Nodes[i].Depth < out.Nodes[j].Depth
		}
		return out.Nodes[i].URI < out.Nodes[j].URI
	})

	for _, e := range edges {
		out.Edges = append(out.Edges, e)
	}
	sort.Slice(out.Edges, func(i, j int) bool {
		a, b := out.Edges[i], out.Edges[j]
		if a.Src != b.Src {
			return a.Src < b.Src
		}
		if a.Predicate != b.Predicate {
			return a.Predicate < b.Predicate
		}
		return a.Dst < b.Dst
	})
	return out, nil
}

// WalkSize counts what a walk would return without building it.
//
// The 500 node gate in 04_graph.md section 5 needs the number before the work,
// so this repeats the traversal and keeps only the visited set. Repeating it is
// cheaper than it looks, since the expensive part of Walk is reading a record
// per node and this reads none.
func (s *Store) WalkSize(uri string, depth int) (int, error) {
	if depth < 0 {
		depth = 0
	}
	seen := map[string]bool{uri: true}
	frontier := []string{uri}
	for hop := 0; hop < depth && len(frontier) > 0; hop++ {
		var next []string
		for _, from := range frontier {
			for _, incoming := range []bool{false, true} {
				es, err := s.Edges(from, incoming)
				if err != nil {
					return 0, err
				}
				for _, e := range es {
					to := e.Dst
					if incoming {
						to = e.Src
					}
					if !seen[to] {
						seen[to] = true
						next = append(next, to)
					}
				}
			}
		}
		frontier = next
	}
	return len(seen), nil
}
