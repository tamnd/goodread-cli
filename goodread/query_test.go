package goodread

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestQueryReadsAndDoesNotWrite(t *testing.T) {
	s := testStore(t)
	now := time.Now().UTC()
	if err := s.PutNode(Node{URI: "gr:book/1", Kind: "book", Title: "Dune", NumPages: 412,
		JSON: []byte(`{"legacy_id":1,"stats":{"average_rating":4.25}}`), RetrievedAt: now}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	rows, err := s.Query(ctx, "select title, num_pages from books", 0)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("%d rows", len(rows))
	}
	if got := rows[0].Cells()["title"]; got != "Dune" {
		t.Errorf("title = %q", got)
	}
	// The column order is the statement's, since that is the only order anybody
	// asked for and alphabetising it would reorder what they typed.
	if cols := rows[0].Columns(); len(cols) != 2 || cols[0] != "title" || cols[1] != "num_pages" {
		t.Errorf("columns = %v", cols)
	}

	// The whole record is in the json column, which is what makes a field the
	// columns do not carry still reachable.
	rows, err = s.Query(ctx, `select json_extract(json,'$.stats.average_rating') as rating from books`, 0)
	if err != nil {
		t.Fatalf("json_extract: %v", err)
	}
	if got := rows[0].Cells()["rating"]; got != "4.25" {
		t.Errorf("rating = %q", got)
	}

	for _, bad := range []string{
		"delete from books",
		"update books set title='x'",
		"drop table books",
		"insert into books(uri) values('gr:book/2')",
		"select 1; delete from books",
		"",
	} {
		if _, err := s.Query(ctx, bad, 0); err == nil {
			t.Errorf("query allowed %q", bad)
		}
	}

	// And the refusal is not just the prefix check. query_only is what actually
	// stops a statement spelled a way the prefix check does not know about.
	if _, err := s.Query(ctx, "with x as (select 1) delete from books", 0); err == nil {
		t.Error("a delete behind a with was allowed")
	}

	// Nothing was written by any of that.
	rows, _ = s.Query(ctx, "select count(*) as n from books", 0)
	if got := rows[0].Cells()["n"]; got != "1" {
		t.Errorf("books = %q after the refused statements", got)
	}
}

// TestQueryNullIsEmpty. A cell that prints NULL is one that greps and sorts as
// the four letter word, and a store full of columns that are legitimately not
// known would fill a csv with it.
func TestQueryNullIsEmpty(t *testing.T) {
	s := testStore(t)
	if err := s.PutNode(Node{URI: "gr:book/1", Kind: "book", Title: "Dune",
		JSON: []byte(`{}`), RetrievedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	rows, err := s.Query(context.Background(), "select isbn13 from books", 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := rows[0].Cells()["isbn13"]; got != "" {
		t.Errorf("a missing isbn printed as %q", got)
	}
}

// TestTablesNamesWhatCanBeQueried. The first question anybody running SQL over
// somebody else's schema has.
func TestTablesNamesWhatCanBeQueried(t *testing.T) {
	s := testStore(t)
	names, err := s.Tables()
	if err != nil {
		t.Fatal(err)
	}
	all := strings.Join(names, " ")
	for _, want := range append(append([]string{}, nodeTables...), "edges", "search", "records", "queue") {
		if !strings.Contains(all, want) {
			t.Errorf("%s is not in the table list: %v", want, names)
		}
	}
}
