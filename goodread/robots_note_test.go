package goodread

import (
	"bytes"
	"context"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The acceptance line these tests hold, from the v0.3.0 checklist:
//
//	a record fetched from a disallowed path carries robots.allowed: false and
//	the rule that matched it
//
// Both directions are checked. A note that appears on everything says nothing,
// and a note that never appears says nothing either, so the disallowed read has
// to carry it and the allowed read has to not.
//
// Both use the golden captures, so what is parsed here is the bytes Goodreads
// actually served rather than a fixture written to make the test pass.

// TestDisallowedReadCarriesTheNote walks /book/reviews, which robots.txt
// disallows, and checks the rows say so.
func TestDisallowedReadCarriesTheNote(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NoRobots = true
	cfg.Delay = MinDelay

	body := readCapture(t, "book_reviews_2767052.js.gz")
	c, _ := testClient(t, cfg, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/book/reviews/") {
			t.Errorf("server saw %s, want /book/reviews/", r.URL.Path)
		}
		_, _ = w.Write(body)
	})
	c.SetWarnWriter(nil)

	reviews, err := c.GetReviewPages(context.Background(), "2767052", 1)
	if err != nil {
		t.Fatalf("GetReviewPages: %v", err)
	}
	if len(reviews) == 0 {
		t.Fatal("no reviews came back, so the note has nothing to sit on")
	}
	for i, rv := range reviews {
		if rv.Robots == nil {
			t.Fatalf("review %d carries no robots note", i)
		}
		if rv.Robots.Allowed {
			t.Errorf("review %d says allowed = true for a disallowed path", i)
		}
		if rv.Robots.Rule != "Disallow: /book/reviews/" {
			t.Errorf("review %d rule = %q, want %q", i, rv.Robots.Rule, "Disallow: /book/reviews/")
		}
		if !strings.HasPrefix(rv.Robots.Path, "/book/reviews/") {
			t.Errorf("review %d path = %q, want the path that was read", i, rv.Robots.Path)
		}
		if rv.Robots.Source == "" {
			t.Errorf("review %d does not say which robots.txt decided it", i)
		}
	}
}

// TestAllowedReadCarriesNoNote reads a book page, which is allowed, and checks
// nothing on the record claims otherwise.
func TestAllowedReadCarriesNoNote(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NoRobots = true // on, and it should still make no difference here
	cfg.Delay = MinDelay

	body := readCapture(t, "book_show_2767052.html.gz")
	c, _ := testClient(t, cfg, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	})
	c.SetWarnWriter(nil)

	rec, err := c.GetBookRecord(context.Background(), "2767052", DepthMeta)
	if err != nil {
		t.Fatalf("GetBookRecord: %v", err)
	}
	if rec.Book.Robots != nil {
		t.Errorf("an allowed book page came back marked %+v", *rec.Book.Robots)
	}
	if rec.Work != nil && rec.Work.Robots != nil {
		t.Errorf("the work off an allowed book page came back marked %+v", *rec.Work.Robots)
	}
	for i, rv := range rec.Book.Reviews {
		if rv.Robots != nil {
			t.Errorf("review %d off the book page came back marked %+v", i, *rv.Robots)
		}
	}
}

// TestEveryRecordFetcherStamps is the structural half.
//
// The note is worth little if a fetcher added later forgets to ask for it, so
// every Get*Record method has to either stamp what it built or be built out of
// other Get*Record methods that did. This reads the source rather than the
// behaviour, which is the only way to catch the fetcher that has no disallowed
// surface today and gains one when Goodreads next edits robots.txt.
func TestEveryRecordFetcherStamps(t *testing.T) {
	// Read the directory and parse each file, rather than parser.ParseDir,
	// which is deprecated because it ignores build tags. Nothing here is behind
	// a tag, so the only thing that changes is which of the two this file will
	// still compile against next year.
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the package directory: %v", err)
	}
	files := map[string]*ast.File{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		files[name] = file
	}

	var checked int
	for name, file := range files {
		for _, d := range file.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Body == nil {
				continue
			}
			if !strings.HasPrefix(fn.Name.Name, "Get") || !strings.HasSuffix(fn.Name.Name, "Record") {
				continue
			}
			checked++
			var buf bytes.Buffer
			if err := printer.Fprint(&buf, fset, fn.Body); err != nil {
				t.Fatal(err)
			}
			body := buf.String()
			if strings.Contains(body, "c.stamp(") {
				continue
			}
			if strings.Contains(body, "Record(ctx,") {
				continue // built out of other record fetchers, which stamped
			}
			t.Errorf("%s: %s builds a record without stamping the robots verdict on it",
				filepath.Base(name), fn.Name.Name)
		}
	}
	if checked == 0 {
		t.Fatal("found no record fetchers, so this test proved nothing")
	}
}
