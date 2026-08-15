package cli

import (
	"strings"
	"testing"

	"github.com/tamnd/goodread-cli/goodread"
)

func TestMCPHelpSaysWhatItWillNotDo(t *testing.T) {
	store := emptyStore(t)
	out, err := runCmd(t, store, "mcp", "--help")
	if err != nil {
		t.Fatalf("mcp --help: %v", err)
	}
	// The argument itself, not just the fact of the exclusion. Somebody
	// wondering why their model cannot read a shelf should find the reason
	// here rather than in a commit message.
	for _, want := range []string{
		"--no-robots has no effect",
		"not that person deciding",
		"search",
		"shelf",
		"reviews",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("mcp --help does not mention %q:\n%s", want, out)
		}
	}
	// And the tools it does have, so the help is a complete answer.
	for _, name := range goodread.MCPToolNames() {
		if !strings.Contains(out, name) {
			t.Errorf("mcp --help does not list %s", name)
		}
	}
}

func TestNoRobotsIsTakenBackOutForTheServer(t *testing.T) {
	// The flag is a person deciding it is their call, and a model calling a
	// tool is not that person, so the server's client is built from a config
	// with the override removed rather than from the one the command line set.
	a := &App{}
	a.cfg.NoRobots = true
	if a.mcpConfig().NoRobots {
		t.Error("the server would run with --no-robots on")
	}
	if !a.cfg.NoRobots {
		t.Error("mcpConfig changed the app's own config, which would leak into other commands")
	}
}

func TestWorkIsAKnownCommand(t *testing.T) {
	store := emptyStore(t)
	out, err := runCmd(t, store, "work", "--help")
	if err != nil {
		t.Fatalf("work --help: %v", err)
	}
	// It says which route it takes, because two requests for one record is a
	// thing somebody should not have to discover from a rate limit.
	for _, want := range []string{"best edition", "disallowed"} {
		if !strings.Contains(out, want) {
			t.Errorf("work --help does not mention %q:\n%s", want, out)
		}
	}
}

func TestTheRootListsTheNewCommands(t *testing.T) {
	store := emptyStore(t)
	out, err := runCmd(t, store, "--help")
	if err != nil {
		t.Fatalf("--help: %v", err)
	}
	for _, want := range []string{"work", "crawl", "mcp"} {
		if !strings.Contains(out, want) {
			t.Errorf("the root help does not list %s:\n%s", want, out)
		}
	}
}
