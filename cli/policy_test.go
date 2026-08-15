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
