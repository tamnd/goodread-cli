package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tamnd/goodread-cli/goodread"
)

// graph walks the local store and prints what is around a node.
//
// Nothing here fetches. The graph is whatever has already been crawled, and a
// node on the far end of an edge that nobody has read yet is still printed,
// because "there is an edge to this and no record for it" is the most useful
// thing this command can say to somebody deciding what to crawl next.
func (a *App) graphCmd() *cobra.Command {
	var depth int
	var yes bool
	var edgesOnly bool
	cmd := &cobra.Command{
		Use:   "graph <uri|id>",
		Short: "Walk the local graph around a node",
		Long: "graph prints the nodes and edges within --depth hops of a starting node,\n" +
			"reading only the local store.\n\n" +
			"The start is a gr: URI, the form `goodread id` prints. Cycles are normal\n" +
			"here, Book to Work to BestBook closes in three hops on most popular titles,\n" +
			"so the walk carries a visited set and each node is reported at the shortest\n" +
			"depth it was reached at.",
		Args: cobra.ExactArgs(1),
		Example: "  goodread graph gr:book/2767052\n" +
			"  goodread graph gr:author/153394 --depth 2\n" +
			"  goodread graph gr:work/2792775 --depth 3 --yes --edges",
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := a.openStore()
			if err != nil {
				return codeError(exitError, err)
			}
			uri := args[0]
			if !strings.HasPrefix(uri, "gr:") {
				return codeError(exitUsage, fmt.Errorf(
					"%q is not a gr: uri. it looks like gr:book/2767052, and `goodread id %s` will tell you which one a url is", uri, uri))
			}

			// The count before the walk, per 04_graph.md section 5. A depth 3
			// walk from a popular author is tens of thousands of nodes and
			// printing them is not the expensive part, reading a record for
			// each of them is.
			n, err := st.WalkSize(uri, depth)
			if err != nil {
				return codeError(exitError, err)
			}
			if n > graphGate && !yes {
				fmt.Fprintf(os.Stderr, "%s at depth %d is %d nodes.\n", uri, depth, n)
				return codeError(exitUsage, fmt.Errorf("pass --yes to walk all %d, or lower --depth", n))
			}
			a.verbosef(1, "walking %d nodes", n)

			g, err := st.Walk(uri, depth)
			if err != nil {
				return codeError(exitError, err)
			}
			if edgesOnly {
				return a.renderOrEmpty(g.Edges, len(g.Edges))
			}
			return a.renderOrEmpty(g.Nodes, len(g.Nodes))
		},
	}
	cmd.Flags().IntVar(&depth, "depth", 1, "how many hops out from the starting node")
	cmd.Flags().BoolVar(&yes, "yes", false, fmt.Sprintf("go ahead with a walk of more than %d nodes", graphGate))
	cmd.Flags().BoolVar(&edgesOnly, "edges", false, "print the edges rather than the nodes")
	return cmd
}

// graphGate is where a walk stops being something to run without thinking.
const graphGate = 500

// export writes the store out in one of the shapes 04_graph.md section 6 names.
//
// SQLite is not one of the choices here, because the store already is a SQLite
// file and the way to export it is to copy it.
func (a *App) exportCmd() *cobra.Command {
	var format string
	var kinds []string
	var edges bool
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Write the local store out as JSONL or RDF",
		Long: "export writes what has been crawled, reading nothing from the network.\n\n" +
			"jsonl is one node per line and is the default, which is what the rest of\n" +
			"the family reads. rdf is Turtle, mapped onto schema.org where it fits and\n" +
			"onto a local grv: vocabulary where it does not, because a wrong predicate\n" +
			"is worse than a local one.\n\n" +
			"The store itself is a SQLite file, so exporting to SQLite is copying it.",
		Args: cobra.NoArgs,
		Example: "  goodread export > books.jsonl\n" +
			"  goodread export --kind book --kind author\n" +
			"  goodread export --edges\n" +
			"  goodread export --to rdf > goodreads.ttl",
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := a.openStore()
			if err != nil {
				return codeError(exitError, err)
			}
			for _, k := range kinds {
				if !goodread.IsNodeKind(k) {
					return codeError(exitUsage, fmt.Errorf(
						"%q is not a node kind. it takes one of: %s", k, strings.Join(goodread.ExportKinds(), ", ")))
				}
			}
			switch format {
			case "jsonl", "":
				if edges {
					err = st.ExportEdgesJSONL(os.Stdout)
				} else {
					err = st.ExportJSONL(os.Stdout, kinds)
				}
			case "rdf", "ttl", "turtle":
				// The edges are always in the RDF, since a triple file with the
				// nodes and none of the relationships between them is a list
				// and not a graph.
				err = st.ExportRDF(os.Stdout, kinds)
			default:
				return codeError(exitUsage, fmt.Errorf("export takes --to jsonl or --to rdf, not %q", format))
			}
			if err != nil {
				return codeError(exitError, err)
			}
			return nil
		},
	}
	// --to and not --format, because --format is the global one that picks how
	// records print and two flags with one name is how somebody ends up asking
	// for a table of Turtle.
	cmd.Flags().StringVar(&format, "to", "jsonl", "jsonl or rdf")
	cmd.Flags().StringSliceVar(&kinds, "kind", nil, "limit to these node kinds, repeatable (default: all)")
	cmd.Flags().BoolVar(&edges, "edges", false, "write the edge table instead of the nodes, jsonl only")
	return cmd
}
