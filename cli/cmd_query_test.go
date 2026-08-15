package cli

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tamnd/goodread-cli/goodread"
)

// runCmd runs the whole command tree against a temporary store and returns what
// it printed.
//
// Through Execute rather than by calling RunE directly, because half of what
// these commands do is flag resolution and the store path is one of the things
// being resolved.
//
// os.Stdout is swapped for a pipe rather than the command's own writer, since
// every printer in this package writes to os.Stdout directly. Moving them all
// onto an injected writer is worth doing and is not this change.
func runCmd(t *testing.T, store string, args ...string) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	real := os.Stdout
	os.Stdout = w

	// Drained in the background, because a command that prints more than the
	// pipe buffer holds would otherwise block forever on its own output.
	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	root := NewRootCmd()
	root.SetOut(w)
	root.SetErr(w)
	root.SetArgs(append([]string{"--store", store, "--format", "json"}, args...))
	runErr := root.Execute()

	os.Stdout = real
	_ = w.Close()
	out := <-done
	_ = r.Close()
	return out, runErr
}

// seedStore puts one book in a store the commands will then open.
func seedStore(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "goodread.db")
	s, err := goodread.OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer func() { _ = s.Close() }()
	if err := s.PutNode(goodread.Node{
		URI: "gr:book/2767052", Kind: "book", LegacyID: 2767052,
		Title: "The Hunger Games", ISBN13: "9780439023481", NumPages: 374,
		Description: "Katniss volunteers", AuthorName: "Suzanne Collins",
		JSON:        []byte(`{"kind":"book","legacy_id":2767052,"title":"The Hunger Games"}`),
		RetrievedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("PutNode: %v", err)
	}
	return path
}

// TestQueryPrintsRows. The renderer reflects over struct tags and a SQL result
// set has none, so this is the one output path that goes through Rowed. If that
// wiring is wrong the command prints empty rows rather than failing, which is
// why the assertion is on a value and not on an error.
func TestQueryPrintsRows(t *testing.T) {
	store := seedStore(t)
	out, err := runCmd(t, store, "query", "select title, isbn13 from books")
	if err != nil {
		t.Fatalf("query: %v (%s)", err, out)
	}
	if !strings.Contains(out, "The Hunger Games") {
		t.Errorf("the title is not in the output: %s", out)
	}
	if !strings.Contains(out, "9780439023481") {
		t.Errorf("the isbn is not in the output: %s", out)
	}
}

// TestQueryRefusesToWrite. The guard is in the store and this is the command
// end of it: a write comes back as usage, which is exit 2, because it is the
// statement that is wrong and not the run.
func TestQueryRefusesToWrite(t *testing.T) {
	store := seedStore(t)
	out, err := runCmd(t, store, "query", "delete from books")
	if err == nil {
		t.Fatalf("the delete was allowed: %s", out)
	}
	if code := exitCodeOf(err); code != exitUsage {
		t.Errorf("exit %d for a write, want %d", code, exitUsage)
	}
}

// TestQueryTablesNamesTheTables. Somebody who has just been handed a SQL prompt
// over a schema they did not write needs this before anything else.
func TestQueryTablesNamesTheTables(t *testing.T) {
	store := seedStore(t)
	out, err := runCmd(t, store, "query", "--tables")
	if err != nil {
		t.Fatalf("query --tables: %v", err)
	}
	for _, want := range []string{"books", "authors", "edges", "search"} {
		if !strings.Contains(out, want) {
			t.Errorf("%s is not listed: %s", want, out)
		}
	}
}

// TestFindUsesTheFullTextIndex. The point of moving find off the substring
// match is stemming and ranking, and a stemmed match is the cheapest proof the
// new path is the one running.
func TestFindUsesTheFullTextIndex(t *testing.T) {
	store := seedStore(t)
	out, err := runCmd(t, store, "find", "volunteer")
	if err != nil {
		t.Fatalf("find: %v (%s)", err, out)
	}
	if !strings.Contains(out, "gr:book/2767052") {
		t.Errorf("a stemmed match did not find the book: %s", out)
	}
}

// TestFindSaysNothingIsThereRatherThanFetching. find is offline, and the whole
// risk of an empty result is somebody reading it as "this book does not exist"
// rather than "you have not crawled it".
func TestFindSaysNothingIsThereRatherThanFetching(t *testing.T) {
	store := seedStore(t)
	out, err := runCmd(t, store, "find", "a book nobody has crawled")
	if err == nil {
		t.Fatalf("an empty result was not an error: %s", out)
	}
	if code := exitCodeOf(err); code != exitNotFound {
		t.Errorf("exit %d, want %d", code, exitNotFound)
	}
}

// TestLookupAnswersFromTheStore. An ISBN13 already in the graph is a fact that
// does not change, and spending a request on it spends somebody's rate limit to
// learn what we already know. --ids stops before the book page, so this makes
// no request at all.
func TestLookupAnswersFromTheStore(t *testing.T) {
	store := seedStore(t)
	out, err := runCmd(t, store, "lookup", "9780439023481", "--ids")
	if err != nil {
		t.Fatalf("lookup: %v (%s)", err, out)
	}
	if !strings.Contains(out, "2767052") {
		t.Errorf("the store did not answer: %s", out)
	}
	// The legacy id and not the URI, because this is what people paste into the
	// next command.
	if strings.Contains(out, "gr:book/") {
		t.Errorf("lookup printed a URI where an id belongs: %s", out)
	}
}

func exitCodeOf(err error) int {
	var e *ExitError
	if errors.As(err, &e) {
		return e.Code
	}
	return -1
}
