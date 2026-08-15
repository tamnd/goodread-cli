package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/tamnd/goodread-cli/goodread"
)

// TestRailsCommandsAreRegistered. Six surfaces were spec'd and six commands
// have to exist, with the flags the spec says each one takes. A surface with an
// extractor and no command is work nobody can reach.
func TestRailsCommandsAreRegistered(t *testing.T) {
	want := map[string][]string{
		"author":   {"books"},
		"series":   {"books"},
		"list":     {"books", "page"},
		"genre":    {"books", "related"},
		"editions": {"page", "pages"},
		"quotes":   {"page", "pages", "author"},
	}
	root := NewRootCmd()
	for name, flags := range want {
		cmd := find(root, []string{name})
		if cmd == nil {
			t.Errorf("no %s command", name)
			continue
		}
		for _, f := range flags {
			if cmd.Flags().Lookup(f) == nil {
				t.Errorf("%s has no --%s", name, f)
			}
		}
	}
}

// TestQuoteStillAnswers. v0.2.0 spelled it `quote` and the surface is plural,
// so the old spelling stays as an alias rather than breaking every script that
// already types it.
func TestQuoteStillAnswers(t *testing.T) {
	cmd := find(NewRootCmd(), []string{"quotes"})
	if cmd == nil {
		t.Fatal("no quotes command")
	}
	var found bool
	for _, a := range cmd.Aliases {
		if a == "quote" {
			found = true
		}
	}
	if !found {
		t.Errorf("aliases = %v, and the v0.2.0 spelling was quote", cmd.Aliases)
	}
}

// TestPrintersSayWhatIsMissing. The not read block is the whole point of the
// envelope, and a table printer that drops it turns "ten of 527" into "ten".
func TestPrintersSayWhatIsMissing(t *testing.T) {
	total := int64(527)
	ed := &goodread.Editions{
		Envelope: goodread.Envelope{Missed: []string{"this page carries 10 of 527 editions"}},
		Work:     goodread.Ref{Type: "Work", ID: "2792775"},
		Title:    "The Hunger Games",
		Page:     1, TotalCount: &total,
		Editions: []goodread.Edition{{
			Book:  goodread.Ref{Type: "Book", ID: "2767052"},
			Title: "The Hunger Games", Format: "Hardcover", ISBN13: "9780439023481",
		}},
	}
	var buf bytes.Buffer
	printEditions(&buf, ed)
	out := buf.String()
	for _, want := range []string{"The Hunger Games", "527", "9780439023481", "10 of 527"} {
		if !strings.Contains(out, want) {
			t.Errorf("editions output does not mention %q:\n%s", want, out)
		}
	}
}

// TestRatingSuffixKeepsTheFourStates. An unrated book and a row that did not
// publish a rating are different facts, and "0.00 (0)" says the first one about
// both.
func TestRatingSuffixKeepsTheFourStates(t *testing.T) {
	avg, count := 4.35, int64(9811629)
	if got := ratingSuffix(nil, nil); got != "" {
		t.Errorf("no rating printed as %q", got)
	}
	if got := ratingSuffix(nil, &count); got != "" {
		t.Errorf("a count with no average printed as %q", got)
	}
	if got := ratingSuffix(&avg, nil); !strings.Contains(got, "4.35") || strings.Contains(got, "(") {
		t.Errorf("an average with no count printed as %q", got)
	}
	if got := ratingSuffix(&avg, &count); !strings.Contains(got, "9,811,629") {
		t.Errorf("ratingSuffix = %q, and the count is meant to be readable", got)
	}
}

// TestListPrinterKeepsScoreAndVotes. They are two measurements and a book can
// rank differently under each, so a reader who sees one of them cannot tell
// which ranking is in front of them.
func TestListPrinterKeepsScoreAndVotes(t *testing.T) {
	score, votes := int64(4518690), int64(45920)
	l := &goodread.List{
		ID: "1.Best_Books_Ever", Title: "Best Books Ever",
		Books: []goodread.ListCard{{
			BookCard: goodread.BookCard{Title: "The Hunger Games"},
			Rank:     1, Score: &score, Votes: &votes,
		}},
	}
	var buf bytes.Buffer
	printList(&buf, l)
	out := buf.String()
	if !strings.Contains(out, "4,518,690") || !strings.Contains(out, "45,920") {
		t.Errorf("list output drops one of the two numbers:\n%s", out)
	}
}
