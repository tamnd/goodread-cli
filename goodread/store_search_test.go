package goodread

import (
	"path/filepath"
	"testing"
)

// TestStoreSearchFindsWhatWasStored covers the offline route.
//
// find makes no requests, so the only thing it can be wrong about is what is
// already in the store. The kind filter is the part worth pinning, since a
// query that matches an author and a book should be narrowable to one.
func TestStoreSearchFindsWhatWasStored(t *testing.T) {
	st, err := OpenStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer func() { _ = st.Close() }()

	type rec struct {
		Title string `json:"title"`
	}
	type person struct {
		Name string `json:"name"`
	}
	if err := st.Put("book", "2767052", "https://www.goodreads.com/book/show/2767052", rec{Title: "The Hunger Games"}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := st.Put("author", "153394", "https://www.goodreads.com/author/show/153394", person{Name: "Suzanne Collins"}); err != nil {
		t.Fatalf("Put: %v", err)
	}

	hits, err := st.Search("", "hunger", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 1 || hits[0].ID != "2767052" {
		t.Fatalf("got %+v, want the one book", hits)
	}
	if hits[0].Title != "The Hunger Games" {
		t.Errorf("title = %q, and a hit with no title is a row nobody can read", hits[0].Title)
	}

	// Case does not matter, because nobody types a title the way it is stored.
	if hits, err := st.Search("", "COLLINS", 10); err != nil || len(hits) != 1 {
		t.Errorf("got %+v (%v), want the author", hits, err)
	}
	// A name comes from a different key and still has to fill the title column.
	if hits, _ := st.Search("author", "collins", 10); len(hits) != 1 || hits[0].Title != "Suzanne Collins" {
		t.Errorf("got %+v, want the author with their name", hits)
	}
	if hits, _ := st.Search("book", "collins", 10); len(hits) != 0 {
		t.Errorf("the kind filter let %d author rows through", len(hits))
	}
	if hits, _ := st.Search("", "nothing here matches this", 10); len(hits) != 0 {
		t.Errorf("got %+v for a query that matches nothing", hits)
	}
}
