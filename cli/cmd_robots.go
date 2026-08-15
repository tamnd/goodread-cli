package cli

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/tamnd/goodread-cli/goodread"
)

// robotsCmd makes the policy inspectable.
//
// Every other command's behaviour follows from these rules, so there has to be
// a way to see them without reading the source or guessing. When somebody
// disagrees with a refusal, `goodread robots check <url>` prints the rule that
// caused it, which turns an argument into a fact.
func (a *App) robotsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "robots",
		Short: "Show the robots.txt rules and which surfaces they permit",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			r, err := a.client.Robots().Get(cmd.Context())
			if err != nil {
				return mapFetchErr(err)
			}
			return a.printRobots(r)
		},
	}
	cmd.AddCommand(a.robotsCheckCmd())
	return cmd
}

func (a *App) printRobots(r *goodread.Robots) error {
	if a.format == string(FormatJSON) || a.format == string(FormatJSONL) {
		return a.render(robotsReport(r))
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "%s\tfetched %s ago, cached until %s\n",
		r.Source, age(time.Since(r.FetchedAt)), r.FetchedAt.Add(goodread.RobotsTTL).Format("15:04"))
	fmt.Fprintf(w, "User-agent: * group\t%d rules\n\n", len(r.Rules))

	var allowed, needsFlag []goodread.Op
	for _, o := range goodread.Ops {
		if o.Name == "robots" {
			continue
		}
		if r.Allowed(goodread.SamplePath(o)) {
			allowed = append(allowed, o)
		} else {
			needsFlag = append(needsFlag, o)
		}
	}

	fmt.Fprintln(w, "allowed by default")
	for _, o := range allowed {
		note := ""
		// Name the exception explicitly. An Allow that overrides a broader
		// Disallow is the least obvious thing in the file and the thing most
		// likely to be lost by a future parser change.
		if m := r.Check(goodread.SamplePath(o)); m != nil && m.Allow {
			note = "\tby " + m.String() + ", overriding a broader Disallow"
		}
		fmt.Fprintf(w, "  %s\t%s\t%s%s\n", o.Surface, o.Name, patternOf(o), note)
	}

	if len(needsFlag) > 0 {
		fmt.Fprintln(w, "\nneeds --no-robots")
		for _, o := range needsFlag {
			rule := r.Check(goodread.SamplePath(o))
			fmt.Fprintf(w, "  %s\t%s\t%s\t%s\n", o.Surface, o.Name, patternOf(o), ruleString(rule))
		}
	}

	delay := "none set for *"
	if r.CrawlDelay > 0 {
		delay = r.CrawlDelay.String()
	}
	fmt.Fprintf(w, "\ncrawl-delay\t%s\n", delay)
	fmt.Fprintf(w, "pace\t%s, floor %s, not overridable\n", a.cfg.Delay, goodread.MinDelay)
	if len(r.Sitemaps) > 0 {
		fmt.Fprintf(w, "sitemaps\t%d advertised\n", len(r.Sitemaps))
	}
	return w.Flush()
}

func (a *App) robotsCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check <url|path>",
		Short: "Say whether one URL may be fetched, and which rule decides",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := a.client.Robots().Get(cmd.Context())
			if err != nil {
				return mapFetchErr(err)
			}
			target := pathAndQuery(args[0])
			rule := r.Check(target)

			// A refusal is returned as an error rather than printed, so that
			// `robots check` exits 7 and reads exactly like the refusal any
			// other command would give for the same URL. One message, one
			// place it is written, and a script can branch on the code.
			if rule != nil && !rule.Allow && !a.cfg.NoRobots {
				return codeError(exitDisallowed, &goodread.DisallowedError{
					Path: target, Rule: *rule, Source: r.Source,
				})
			}

			fmt.Printf("%s\n", target)
			switch {
			case rule == nil:
				fmt.Println("allowed   no rule matches")
			case rule.Allow:
				fmt.Printf("allowed   %s\n", rule.String())
			default:
				fmt.Printf("allowed   by --no-robots, over %s\n", rule.String())
			}
			return nil
		},
	}
}

// pathAndQuery normalises whatever the user pasted into the form the checker
// wants. The query has to survive: "Disallow: /*reviewFilters" only ever
// matches a query string, so dropping it here would make check() disagree with
// the client that actually fetches.
func pathAndQuery(s string) string {
	s = strings.TrimSpace(s)
	if u, err := url.Parse(s); err == nil && u.Host != "" {
		if u.RawQuery != "" {
			return u.Path + "?" + u.RawQuery
		}
		return u.Path
	}
	if !strings.HasPrefix(s, "/") {
		s = "/" + s
	}
	return s
}

func patternOf(o goodread.Op) string { return goodread.SamplePath(o) }

func ruleString(r *goodread.Rule) string {
	if r == nil {
		return ""
	}
	return r.String()
}

func age(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%d seconds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	default:
		return fmt.Sprintf("%.1f hours", d.Hours())
	}
}

type robotsRule struct {
	Allow   bool   `json:"allow"`
	Pattern string `json:"pattern"`
}

type robotsSurface struct {
	Surface string `json:"surface"`
	Op      string `json:"op"`
	Path    string `json:"path"`
	Allowed bool   `json:"allowed"`
	Rule    string `json:"rule,omitempty"`
}

type robotsJSON struct {
	Source     string          `json:"source"`
	FetchedAt  time.Time       `json:"fetched_at"`
	CrawlDelay string          `json:"crawl_delay,omitempty"`
	Rules      []robotsRule    `json:"rules"`
	Surfaces   []robotsSurface `json:"surfaces"`
	Sitemaps   []string        `json:"sitemaps,omitempty"`
}

func robotsReport(r *goodread.Robots) robotsJSON {
	out := robotsJSON{Source: r.Source, FetchedAt: r.FetchedAt, Sitemaps: r.Sitemaps}
	if r.CrawlDelay > 0 {
		out.CrawlDelay = r.CrawlDelay.String()
	}
	for _, rule := range r.Rules {
		out.Rules = append(out.Rules, robotsRule{Allow: rule.Allow, Pattern: rule.Pattern})
	}
	for _, o := range goodread.Ops {
		if o.Name == "robots" {
			continue
		}
		p := goodread.SamplePath(o)
		s := robotsSurface{Surface: o.Surface, Op: o.Name, Path: p, Allowed: r.Allowed(p)}
		if m := r.Check(p); m != nil {
			s.Rule = m.String()
		}
		out.Surfaces = append(out.Surfaces, s)
	}
	return out
}
