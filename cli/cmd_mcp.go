package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tamnd/goodread-cli/goodread"
)

// work ──────────────────────────────────────────────────────────────────────

func (a *App) workCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "work <work-id|url>",
		Short: "Read a work through its best edition",
		Long: "work reads the abstract thing rather than one printing of it: the\n" +
			"original title, the awards, the places and characters, and how many\n" +
			"editions there are.\n\n" +
			"It reads the work through its best edition, which means the editions\n" +
			"page and then the first edition's own page, because /work/<id> is\n" +
			"disallowed and this is the allowed route to the same facts. That costs\n" +
			"two requests and the record says which edition it came through.",
		Args: cobra.ExactArgs(1),
		Example: "  goodread work 2792775\n" +
			"  goodread work https://www.goodreads.com/work/editions/2792775",
		RunE: func(cmd *cobra.Command, args []string) error {
			// A work id and not a book id. The two look alike and the wrong one
			// reads a different book entirely, so the URL form is resolved
			// rather than passed through.
			id := args[0]
			if _, got := goodread.Classify(args[0]); got != "" {
				id = got
			}
			a.verbosef(1, "reading work %s through its best edition", id)
			rec, err := a.client.GetWorkRecord(cmd.Context(), id)
			if err != nil {
				return mapFetchErr(err)
			}
			a.reportLevels(rec.Work.Envelope)
			if a.store != nil {
				_ = a.store.Put("work", id, goodread.EditionsURL(id, 1), rec)
			}
			return a.renderRecords([]*goodread.WorkRecord{rec}, func() { printWork(os.Stdout, rec) })
		},
	}
	return cmd
}

func printWork(w *os.File, rec *goodread.WorkRecord) {
	title := rec.Work.OriginalTitle
	if title == "" && rec.BestBook != nil {
		title = rec.BestBook.Title
	}
	fmt.Fprintf(w, "%s\n", title)
	fmt.Fprintf(w, "work      %d\n", rec.Work.LegacyID)
	if rec.EditionCount != nil {
		fmt.Fprintf(w, "editions  %d\n", *rec.EditionCount)
	}
	if rec.BestBook != nil {
		fmt.Fprintf(w, "read via  book %d, %s\n", rec.BestBook.LegacyID, rec.BestBook.Title)
	}
	if n := len(rec.Work.AwardsWon); n > 0 {
		fmt.Fprintf(w, "awards    %d\n", n)
	}
	if n := len(rec.Work.Places); n > 0 {
		fmt.Fprintf(w, "places    %d\n", n)
	}
	if n := len(rec.Work.Characters); n > 0 {
		fmt.Fprintf(w, "people    %d\n", n)
	}
}

// mcp ───────────────────────────────────────────────────────────────────────

// mcp is the server, and the tools it does not have are the point of it.
//
// No search, no shelf, no reviews, and --no-robots has no effect here. The
// reason is not that the flag is wrong. It is that the flag's whole
// justification is a person deciding it is their call, and a model calling a
// tool is not that person deciding.
func (a *App) mcpCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Run as an MCP server over stdio",
		Long: goodread.MCPNotice + "\n" + bullets(goodread.MCPExcluded) + "\n" +
			"Served:\n" + bullets(goodread.MCPToolNames()),
		Args:    cobra.NoArgs,
		Example: "  goodread mcp\n  goodread mcp --store ~/books.db",
		RunE: func(cmd *cobra.Command, _ []string) error {
			// stderr, because stdout is the transport. A notice printed onto
			// the wire would be a protocol error rather than a notice.
			fmt.Fprintln(os.Stderr, goodread.MCPNotice)
			fmt.Fprint(os.Stderr, bullets(goodread.MCPExcluded))
			fmt.Fprintf(os.Stderr, "\nserving %d tools on stdio: %s\n",
				len(goodread.MCPToolNames()), strings.Join(goodread.MCPToolNames(), ", "))

			// The store is optional. A server started before anything has been
			// crawled is a normal thing to do, and the two store tools say so
			// themselves rather than the command refusing to start.
			st, err := a.openStore()
			if err != nil {
				a.progressf("no local store (%v), so store_find and store_query will say so", err)
			}

			// A client of its own, built from a config with the override taken
			// back out. Reusing a.client would carry --no-robots in, and a
			// server that honoured it would be a server where the model gets
			// to make the decision the flag exists for a person to make.
			cfg := a.mcpConfig()
			srv := &goodread.MCPServer{
				Client: goodread.NewClient(cfg), Store: st, Cache: a.cache,
				Config: cfg, Version: Version, Limit: a.limit,
			}
			return srv.ServeMCP(cmd.Context(), os.Stdin, os.Stdout)
		},
	}
	return cmd
}

// mcpConfig is the config with the override taken back out.
//
// Belt and braces: MCPServer never reads a disallowed surface anyway, because
// no tool maps to one. This is here so that if somebody later adds a tool that
// does, it is refused by the client rather than warned about and fetched.
func (a *App) mcpConfig() goodread.Config {
	cfg := a.cfg
	cfg.NoRobots = false
	return cfg
}

func bullets(items []string) string {
	var b strings.Builder
	for _, s := range items {
		fmt.Fprintf(&b, "  %s\n", s)
	}
	return b.String()
}
