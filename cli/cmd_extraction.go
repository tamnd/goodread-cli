package cli

import (
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/tamnd/goodread-cli/goodread"
)

// extractionCmd prints the ladder report.
//
// The number that matters is level 3, the fields still read by CSS selector. A
// selector is a promise to break on a redesign, so the count is something to
// drive down, and a number nobody can see is a number nobody drives down.
func (a *App) extractionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "extraction",
		Short: "Show which rung of the ladder answers for each field",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.printExtraction()
		},
	}
}

type extractionSurface struct {
	Surface   string `json:"surface"`
	Op        string `json:"op"`
	NextData  bool   `json:"next_data"`
	Selectors int    `json:"selectors"`
	Note      string `json:"note"`
}

func (a *App) printExtraction() error {
	var rows []extractionSurface
	counts := map[string]int{}
	for _, f := range goodread.SelectorFields() {
		counts[f.Surface]++
	}
	for _, s := range goodread.Surfaces() {
		op, ok := goodread.LookupSurface(s)
		if !ok {
			continue
		}
		rows = append(rows, extractionSurface{
			Surface:   s,
			Op:        op.Name,
			NextData:  goodread.SurfaceHasNextData(s),
			Selectors: counts[s],
			Note:      goodread.SurfaceSource(s),
		})
	}

	if a.format == string(FormatJSON) || a.format == string(FormatJSONL) {
		return a.render(rows)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	if !a.noHeader {
		fmt.Fprintln(w, "SURFACE\tOP\tLEVEL 1\tSELECTORS\tNOTE")
	}
	for _, r := range rows {
		level1 := "no"
		if r.NextData {
			level1 = "yes"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\n", r.Surface, r.Op, level1, r.Selectors, r.Note)
	}
	if err := w.Flush(); err != nil {
		return err
	}

	fields := goodread.SelectorFields()
	if len(fields) > 0 {
		fmt.Println()
		sw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(sw, "SURFACE\tENTITY\tFIELD\tSINCE\tSELECTOR")
		for _, f := range fields {
			fmt.Fprintf(sw, "%s\t%s\t%s\t%s\t%s\n", f.Surface, f.Entity, f.Field, f.Since, f.Sel)
		}
		if err := sw.Flush(); err != nil {
			return err
		}
	}

	fmt.Println()
	// The honest sentence, which is not "0 fields on selectors, clean". Only
	// the book page is a Next.js route. For the Rails surfaces a selector is
	// not debt, it is the only source there is, and a report that implies
	// otherwise is setting a target nobody can hit.
	next, rails := 0, 0
	for _, r := range rows {
		if r.NextData {
			next++
		} else {
			rails++
		}
	}
	fmt.Printf("%d surface reads the Apollo cache, %d do not.\n", next, rails)
	fmt.Printf("%d fields are registered as selector reads.\n", len(fields))
	// Said plainly, because a zero here would otherwise read as "nothing is on
	// selectors" when what it means is "the surfaces still on selectors have
	// not been ported to the registry yet".
	fmt.Println("only surfaces already ported to the extractor appear in the selector list.")
	return nil
}

// verifyCmd checks the extractor against the pinned captures.
//
// Drift is a thing to find out about from a command rather than from a bug
// report. A new field appearing is Goodreads shipping something and is a chance
// to capture it. A known field disappearing is more urgent and gets its own line.
func (a *App) verifyCmd() *cobra.Command {
	var strict bool
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Check the extractor against the pinned captures",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			reports, err := goodread.VerifyCaptures()
			if err != nil {
				return err
			}
			if a.format == string(FormatJSON) || a.format == string(FormatJSONL) {
				return a.render(reports)
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			if !a.noHeader {
				fmt.Fprintln(w, "CAPTURE\tSURFACE\tFIELDS\tL1\tL2\tL3\tNOTE")
			}
			bad := 0
			for _, r := range reports {
				missing := "-"
				switch {
				case len(r.Missing) > 0:
					sort.Strings(r.Missing)
					missing = "missing " + fmt.Sprint(r.Missing)
					bad++
				case r.Err != "":
					// Not counted as bad. A surface with no extractor yet is a
					// gap in the work, not drift in the site, and conflating
					// the two would make --strict fail for the wrong reason.
					missing = r.Err
				}
				fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%d\t%d\t%s\n",
					r.Capture, r.Surface, r.Fields, r.Level1, r.Level2, r.Level3, missing)
			}
			if err := w.Flush(); err != nil {
				return err
			}
			if bad > 0 && strict {
				return codeError(exitPartial, fmt.Errorf("%d capture(s) lost a known field", bad))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&strict, "strict", false, "exit non-zero when a known field went missing")
	return cmd
}
