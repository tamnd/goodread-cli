package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// TestNoRobotsNotImplied walks every flag on the command tree, sets it, and
// asserts none of them turns the override on.
//
// The failure this guards against is not somebody wiring --no-robots to
// --force on purpose. It is a shared variable, or a flag added later that sets
// a whole config struct at once, quietly making the override reachable without
// anybody typing it. The whole justification for the flag is that a person
// decided, this invocation, that it was their call. A flag that implies it
// takes that decision away from them.
func TestNoRobotsNotImplied(t *testing.T) {
	for _, path := range commandPaths(NewRootCmd(), nil) {
		for _, name := range flagNames(NewRootCmd(), path) {
			if name == "no-robots" {
				continue
			}
			root := NewRootCmd()
			cmd := find(root, path)
			f := cmd.Flags().Lookup(name)
			if f == nil {
				continue
			}
			// A bool takes "true". Anything else gets a value it will parse,
			// and a flag whose value will not parse cannot be the mechanism.
			val := "true"
			if f.Value.Type() != "bool" {
				val = f.DefValue
				if val == "" || val == "[]" {
					continue
				}
			}
			if err := cmd.Flags().Set(name, val); err != nil {
				continue
			}
			if noRobotsOn(root) {
				where := strings.Join(append([]string{"goodread"}, path...), " ")
				t.Errorf("--%s on %q turned on --no-robots", name, where)
			}
		}
	}
}

// TestNoRobotsIsNotHidden keeps the flag visible in help.
//
// An override nobody can see in --help is one people find in a blog post
// instead, without the paragraph explaining what it means.
func TestNoRobotsIsNotHidden(t *testing.T) {
	f := NewRootCmd().PersistentFlags().Lookup("no-robots")
	if f == nil {
		t.Fatal("--no-robots is gone")
	}
	if f.Hidden {
		t.Error("--no-robots is hidden from help")
	}
	if !strings.Contains(f.Usage, "robots.txt") {
		t.Errorf("--no-robots usage does not say what it overrides: %q", f.Usage)
	}
}

// TestCookiesFlagIsGone holds the removal.
func TestCookiesFlagIsGone(t *testing.T) {
	if f := NewRootCmd().PersistentFlags().Lookup("cookies"); f != nil {
		t.Error("--cookies is back: this tool does not borrow signed-in sessions")
	}
}

func noRobotsOn(root *cobra.Command) bool {
	v, err := root.PersistentFlags().GetBool("no-robots")
	return err == nil && v
}

// commandPaths lists every command in the tree by its path below the root.
func commandPaths(cmd *cobra.Command, prefix []string) [][]string {
	out := [][]string{append([]string{}, prefix...)}
	for _, sub := range cmd.Commands() {
		out = append(out, commandPaths(sub, append(append([]string{}, prefix...), sub.Name()))...)
	}
	return out
}

func flagNames(root *cobra.Command, path []string) []string {
	cmd := find(root, path)
	if cmd == nil {
		return nil
	}
	var names []string
	cmd.Flags().VisitAll(func(f *pflag.Flag) { names = append(names, f.Name) })
	return names
}

func find(root *cobra.Command, path []string) *cobra.Command {
	cur := root
	for _, name := range path {
		var next *cobra.Command
		for _, sub := range cur.Commands() {
			if sub.Name() == name {
				next = sub
				break
			}
		}
		if next == nil {
			return nil
		}
		cur = next
	}
	return cur
}

// TestGlobalFlagsReachEveryCommand. --depth, --no-cache and -v are documented
// as global, and a command that was added without inheriting them would be a
// command where -vv silently does nothing. Persistent flags on the root make
// that true by construction, which is exactly why it is worth a test: the way
// it breaks is somebody declaring a local flag of the same name on one command,
// and nothing else would catch that.
func TestGlobalFlagsReachEveryCommand(t *testing.T) {
	root := NewRootCmd()
	for _, path := range commandPaths(root, nil) {
		cmd := find(NewRootCmd(), path)
		if cmd == nil || !cmd.Runnable() {
			continue
		}
		for _, name := range []string{"depth", "no-cache", "verbose", "quiet", "format"} {
			if cmd.InheritedFlags().Lookup(name) == nil && cmd.Flags().Lookup(name) == nil {
				t.Errorf("%s cannot see --%s", strings.Join(path, " "), name)
			}
		}
	}
}

// TestNoV020CommandWasRemoved. The promise in #12 is that no command goes away
// in v0.3.0: the records behind them change, the names do not. Somebody with a
// script written against v0.2.0 should find that it still runs, with
// --no-robots where the path is disallowed, and get better data out of it.
//
// This is the list as v0.2.0 shipped it. Adding to it is fine and removing from
// it is the breaking change this test exists to catch.
func TestNoV020CommandWasRemoved(t *testing.T) {
	v020 := []string{
		"book", "author", "series", "list", "quote", "user", "shelf", "genre",
		"search", "lookup", "find", "reviews", "similar", "id",
		"seed", "crawl", "db", "open", "cache", "robots", "info", "version",
	}
	root := NewRootCmd()
	for _, name := range v020 {
		// Found by name or by alias, since `quote` became `quotes` and kept the
		// old spelling pointing at it, which is a rename and not a removal.
		var found bool
		for _, c := range root.Commands() {
			if c.Name() == name {
				found = true
				break
			}
			for _, a := range c.Aliases {
				if a == name {
					found = true
				}
			}
		}
		if !found {
			t.Errorf("goodread %s shipped in v0.2.0 and is gone", name)
		}
	}
}

// TestDisallowedCommandsSayWhichFlag. A command that refuses on robots.txt has
// to name the rule, the flag and the allowed alternative in the same breath,
// because "disallowed" on its own reads as "this tool cannot do that" and the
// whole point is that it can, once the user has decided.
func TestDisallowedCommandsSayWhichFlag(t *testing.T) {
	root := NewRootCmd()
	for _, name := range []string{"shelf", "reviews", "search"} {
		cmd := find(root, []string{name})
		if cmd == nil {
			t.Fatalf("no %s command", name)
		}
		help := cmd.Long + cmd.Short + cmd.Example
		if !strings.Contains(help, "--no-robots") {
			t.Errorf("goodread %s can need --no-robots and its help never says so", name)
		}
	}
}
