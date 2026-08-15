package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/goodread-cli/goodread"
)

// emptyStore is a store with nothing in it and nothing on the frontier.
func emptyStore(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "goodread.db")
	s, err := goodread.OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	_ = s.Close()
	return path
}

func TestCrawlDryRunReadsNothing(t *testing.T) {
	store := emptyStore(t)

	out, err := runCmd(t, store, "crawl", "--seed", "gr:book/2767052", "--depth", "2", "--dry-run")
	if err != nil {
		t.Fatalf("crawl --dry-run: %v\n%s", err, out)
	}
	for _, want := range []string{"depth", "pace", "pending", "requests", "nothing was read"} {
		if !strings.Contains(out, want) {
			t.Errorf("the plan does not mention %q:\n%s", want, out)
		}
	}

	// It says "at least" rather than giving a number it cannot know. A depth 2
	// crawl reaches books whose own neighbours are not known until they have
	// been read.
	if !strings.Contains(out, "at least") {
		t.Errorf("the plan states its request count as a fact:\n%s", out)
	}

	// And the seed is on the frontier afterwards, so dropping --dry-run runs
	// the crawl that was just described rather than a different one.
	s, err := goodread.OpenStore(store)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, _, err := s.CrawledDepth("gr:book/2767052"); err != nil {
		t.Errorf("the seed from the dry run is not on the frontier: %v", err)
	}
}

func TestCrawlWithNoRobotsNeedsYes(t *testing.T) {
	store := emptyStore(t)

	// A crawl is the one place where the override multiplies, so it is refused
	// rather than warned about. Refused before anything is read, which is why
	// this can be asserted without a server anywhere near it.
	out, err := runCmd(t, store, "--no-robots", "crawl", "--seed", "gr:book/2767052", "--dry-run")
	if err == nil {
		t.Fatalf("--no-robots without --yes was allowed:\n%s", out)
	}
	if code := exitCodeOf(err); code != exitUsage {
		t.Errorf("exit code %d, want %d for a usage refusal", code, exitUsage)
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("the refusal does not say what to do about it: %v", err)
	}

	// With --yes it gets as far as planning, which is as far as --dry-run goes.
	out, err = runCmd(t, store, "--no-robots", "crawl", "--seed", "gr:book/2767052", "--dry-run", "--yes")
	if err != nil {
		t.Fatalf("--no-robots --yes was still refused: %v\n%s", err, out)
	}
}

func TestCrawlWithNothingToDoSaysSo(t *testing.T) {
	store := emptyStore(t)
	out, err := runCmd(t, store, "crawl")
	if err == nil {
		t.Fatalf("a crawl with no seeds and an empty frontier succeeded:\n%s", out)
	}
	if code := exitCodeOf(err); code != exitUsage {
		t.Errorf("exit code %d, want %d", code, exitUsage)
	}
}

func TestCrawlRefusesASeedItCannotResolve(t *testing.T) {
	store := emptyStore(t)
	out, err := runCmd(t, store, "crawl", "--seed", "nonsense", "--dry-run")
	if err == nil {
		t.Fatalf("a seed that resolves to no page was accepted:\n%s", out)
	}
	// Nothing landed on the frontier either, which is the part that matters:
	// a typo should not be discovered as a 404 an hour into a crawl.
	s, err := goodread.OpenStore(store)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	stats, err := s.FrontierStats()
	if err != nil {
		t.Fatal(err)
	}
	if stats["pending"] != 0 {
		t.Errorf("a bad seed put %d rows on the frontier", stats["pending"])
	}
}

func TestCrawlReadsASeedFile(t *testing.T) {
	store := emptyStore(t)
	path := filepath.Join(t.TempDir(), "seeds.txt")
	body := "# the ones worth having\ngr:book/2767052\n\nhttps://www.goodreads.com/author/show/153394\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := runCmd(t, store, "crawl", "--seed-file", path, "--dry-run")
	if err != nil {
		t.Fatalf("crawl --seed-file: %v\n%s", err, out)
	}

	s, err := goodread.OpenStore(store)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	for _, uri := range []string{"gr:book/2767052", "gr:author/153394"} {
		if _, _, err := s.CrawledDepth(uri); err != nil {
			t.Errorf("%s from the seed file is not on the frontier: %v", uri, err)
		}
	}
}

func TestCrawlHasNoParallelismFlag(t *testing.T) {
	store := emptyStore(t)
	// Not defaulted to one. Offered at all is the problem: a crawler that can
	// be told to open ten connections will be, and the site does not get a say.
	for _, flag := range []string{"--workers", "--concurrency", "--parallel", "--jobs"} {
		if _, err := runCmd(t, store, "crawl", flag, "4", "--dry-run"); err == nil {
			t.Errorf("crawl accepts %s", flag)
		}
	}
}

func TestCrawlResetClearsTheFrontierAndKeepsTheRecords(t *testing.T) {
	store := seedGraph(t)
	s, err := goodread.OpenStore(store)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.EnqueueURI("gr:book/2767052", goodread.BookURL("2767052"), "book", 0); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()

	// --reset with a fresh seed, so the command has something to do after the
	// clear and does not exit on an empty frontier.
	out, err := runCmd(t, store, "crawl", "--reset", "--seed", "gr:book/1885", "--dry-run")
	if err != nil {
		t.Fatalf("crawl --reset: %v\n%s", err, out)
	}

	s, err = goodread.OpenStore(store)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, _, err := s.CrawledDepth("gr:book/2767052"); err == nil {
		t.Error("--reset left the old frontier row behind")
	}
	// The records are a different thing, and hours of somebody else's
	// bandwidth, so they are untouched.
	if _, err := s.GetNodeByURI("gr:book/2767052"); err != nil {
		t.Errorf("--reset threw away a stored record: %v", err)
	}
}
