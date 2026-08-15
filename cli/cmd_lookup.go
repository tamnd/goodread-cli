package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/tamnd/goodread-cli/goodread"
)

// lookup is the allowed answer to "I have an identifier, what book is it".
//
// The site search page is disallowed, and this does not need it. Autocomplete
// answers an ISBN13 query with the book id directly, which is one allowed
// request, and the book page then carries everything.
func (a *App) lookupCmd() *cobra.Command {
	var idsOnly bool
	cmd := &cobra.Command{
		Use:   "lookup <isbn|isbn13|asin|title>",
		Short: "Resolve an identifier to a book",
		Long: "lookup resolves an ISBN, an ISBN13, an ASIN or a title to a book, through\n" +
			"the open autocomplete endpoint. It needs no flag and no key.\n\n" +
			"This is the allowed route. The search page is disallowed and lookup does\n" +
			"not use it, so a book that autocomplete cannot find is not found here.",
		Args:    cobra.MinimumNArgs(1),
		Example: "  goodread lookup 9780439023481\n  goodread lookup B002MQYOFW --ids",
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.lookupRun(cmd.Context(), joinArgs(args), idsOnly)
		},
	}
	cmd.Flags().BoolVar(&idsOnly, "ids", false, "print the matching ids and stop, without reading the book pages")
	return cmd
}

func (a *App) lookupRun(ctx context.Context, q string, idsOnly bool) error {
	// The store first, and no request at all when it answers. An ISBN13 that is
	// already in the local graph is a fact that does not change, so spending a
	// request on it is spending somebody's rate limit to learn what we know.
	if hits := a.lookupLocally(q); len(hits) > 0 {
		// The legacy id and not the URI, because everything downstream of this,
		// the --ids output included, is what somebody pastes into another
		// command or another tool, and gr:book/2767052 is not that.
		rows := make([]idRow, 0, len(hits))
		ids := make([]string, 0, len(hits))
		for _, h := range hits {
			_, id := goodread.Classify(h.URL)
			if id == "" {
				continue
			}
			rows = append(rows, idRow{BookID: id, URL: h.URL})
			ids = append(ids, id)
		}
		if len(ids) > 0 {
			a.verbosef(1, "resolved from the local store, no autocomplete request made")
			if idsOnly {
				return a.renderOrEmpty(rows, len(rows))
			}
			return a.bookRun(ctx, ids)
		}
	}

	a.verbosef(1, "resolving %q through /book/auto_complete", q)
	hits, err := a.client.SearchBooks(ctx, q, a.limit)
	if err != nil {
		return mapFetchErr(err)
	}
	if len(hits) == 0 {
		// Naming what was tried matters here, because "not found" from a tool
		// that searched one endpoint means something narrower than it sounds.
		fmt.Fprintf(os.Stderr,
			"nothing matched %q on /book/auto_complete. a title works better than a bare ISBN10, and --no-robots with `goodread search --deep` reads the search page.\n", q)
		return codeError(exitNotFound, nil)
	}

	if idsOnly {
		rows := make([]idRow, 0, len(hits))
		for _, h := range hits {
			rows = append(rows, idRow{BookID: h.BookID, URL: h.URL})
		}
		return a.renderOrEmpty(rows, len(rows))
	}

	ids := make([]string, 0, len(hits))
	for _, h := range hits {
		if h.BookID != "" {
			ids = append(ids, h.BookID)
		}
	}
	a.verbosef(1, "reading %d book page(s)", len(ids))
	return a.bookRun(ctx, ids)
}

// lookupLocally answers an identifier from the graph, or returns nothing.
//
// Nothing on any error, including a store that will not open. This is an
// optimisation on the way to the network path, and a broken store should cost
// one request rather than a failed command.
func (a *App) lookupLocally(q string) []goodread.Hit {
	st, err := a.openStore()
	if err != nil {
		return nil
	}
	hits, err := st.LookupIdentifier(q)
	if err != nil {
		return nil
	}
	return hits
}

// find is full text over the local store.
//
// FTS5 over title, description and author name, so it ranks and it stems: a
// search for volunteer reaches volunteers, which the substring match this
// replaced could not do. It is still offline and it still finds nothing that
// has not been crawled, and that is the sentence people need when it comes back
// empty.
func (a *App) findCmd() *cobra.Command {
	var kind string
	cmd := &cobra.Command{
		Use:   "find <text>",
		Short: "Search the local store",
		Long: "find searches what you have already crawled. It is offline, it makes no\n" +
			"requests, and it finds nothing you have not read yet.\n\n" +
			"It is a full text index over the title, the description and the author\n" +
			"name, ranked by relevance. Words are stemmed, so volunteer matches\n" +
			"volunteers, and punctuation in the query is text rather than syntax.",
		Args:    cobra.MinimumNArgs(1),
		Example: "  goodread find \"hunger games\"\n  goodread find dune --kind book",
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := a.openStore()
			if err != nil {
				return codeError(exitError, err)
			}
			q := joinArgs(args)
			rows, err := st.FindText(kind, q, a.limit)
			if err != nil {
				return codeError(exitError, err)
			}
			if len(rows) == 0 {
				// The older records table is still there and still holds
				// anything crawled before the index existed, so a miss falls
				// back to the substring match rather than telling somebody
				// their own store is empty when it is not.
				if rows, err = st.Search(kind, q, a.limit); err != nil {
					return codeError(exitError, err)
				}
				if len(rows) > 0 {
					a.verbosef(1, "matched through the older records table, not the full text index")
				}
			}
			if len(rows) == 0 {
				fmt.Fprintf(os.Stderr,
					"nothing in the local store matches %q. find reads nothing from the network, so a book has to be crawled before it can be found.\n", q)
				return codeError(exitNotFound, nil)
			}
			return a.renderOrEmpty(rows, len(rows))
		},
	}
	cmd.Flags().StringVar(&kind, "kind", "", "limit to one record kind: book, author, series, list, genre, user")
	return cmd
}

// query is SQL over the store.
//
// The store is a SQLite file and this says so. Anything a person wants to ask
// that the commands do not answer is a join away, and inventing a query
// language of our own to sit in front of it would be inventing a worse SQL.
//
// It cannot write. The statement has to start with select, with, explain or
// pragma, and the connection it runs on has query_only set, so a clever spelling
// of an update is refused by SQLite rather than by a prefix check.
func (a *App) queryCmd() *cobra.Command {
	var showTables bool
	cmd := &cobra.Command{
		Use:   "query <sql>",
		Short: "Run SQL over the local store",
		Long: "query runs one read only statement against the store and prints the rows\n" +
			"in whatever format the other commands use.\n\n" +
			"The tables are one per node kind, books, works, authors, series, lists,\n" +
			"genres, quotes, shelves and users, plus edges and the search index. Every\n" +
			"one keeps the whole record in a json column, so json_extract reaches any\n" +
			"field the columns do not have.",
		Args: cobra.ArbitraryArgs,
		Example: "  goodread query --tables\n" +
			"  goodread query \"select title, isbn13 from books where num_pages > 500\"\n" +
			"  goodread query \"select dst from edges where src = 'gr:author/153394' and predicate = 'wrote'\"\n" +
			"  goodread query \"select json_extract(json,'$.stats.average_rating') as rating, title from books order by rating desc limit 10\"",
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := a.openStore()
			if err != nil {
				return codeError(exitError, err)
			}
			if showTables {
				names, err := st.Tables()
				if err != nil {
					return codeError(exitError, err)
				}
				for _, n := range names {
					fmt.Fprintln(os.Stdout, n)
				}
				return nil
			}
			if len(args) == 0 {
				return codeError(exitUsage, fmt.Errorf("query takes a statement, or --tables to see what there is to query"))
			}
			rows, err := st.Query(cmd.Context(), joinArgs(args), a.limit)
			if err != nil {
				// A statement this refuses and a statement SQLite refuses are
				// both the user's to fix, so both are usage rather than a
				// failure of the run.
				return codeError(exitUsage, err)
			}
			items := make([]any, len(rows))
			for i := range rows {
				items[i] = rows[i]
			}
			return a.renderOrEmpty(items, len(items))
		},
	}
	cmd.Flags().BoolVar(&showTables, "tables", false, "list the tables and views in the store and stop")
	return cmd
}
