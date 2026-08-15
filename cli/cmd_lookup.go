package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
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

// find is full text over the local store.
//
// The index it wants is M5's, so today it is a substring match over what the
// store already holds. It says so rather than looking like a search engine,
// because a search that quietly matches less than you think is worse than one
// that tells you what it does.
func (a *App) findCmd() *cobra.Command {
	var kind string
	cmd := &cobra.Command{
		Use:   "find <text>",
		Short: "Search the local store",
		Long: "find searches what you have already crawled. It is offline, it makes no\n" +
			"requests, and it finds nothing you have not read yet.\n\n" +
			"Today it is a substring match over the stored records. The full text index\n" +
			"lands with the store work in v0.3.0.",
		Args:    cobra.MinimumNArgs(1),
		Example: "  goodread find \"hunger games\"\n  goodread find dune --kind book",
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := a.openStore()
			if err != nil {
				return codeError(exitError, err)
			}
			q := joinArgs(args)
			rows, err := st.Search(kind, q, a.limit)
			if err != nil {
				return codeError(exitError, err)
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
