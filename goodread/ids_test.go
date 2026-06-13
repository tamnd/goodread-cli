package goodread

import "testing"

func TestClassify(t *testing.T) {
	cases := []struct {
		in     string
		entity string
		id     string
	}{
		{"https://www.goodreads.com/book/show/2767052", "book", "2767052"},
		{"https://www.goodreads.com/book/show/2767052-the-hunger-games", "book", "2767052"},
		{"https://www.goodreads.com/author/show/153394.Suzanne_Collins", "author", "153394"},
		{"https://www.goodreads.com/series/73758-the-hunger-games", "series", "73758"},
		{"https://www.goodreads.com/list/show/1.Best_Books_Ever", "list", "1"},
		{"https://www.goodreads.com/user/show/1-otis-chandler", "user", "1"},
		{"https://www.goodreads.com/quotes/12345", "quote", "12345"},
		{"https://www.goodreads.com/genres/fantasy", "genre", "fantasy"},
		{"2767052", "book", "2767052"},
		{"not a url", "", ""},
	}
	for _, c := range cases {
		gotEntity, gotID := Classify(c.in)
		if gotEntity != c.entity || gotID != c.id {
			t.Errorf("Classify(%q) = (%q, %q), want (%q, %q)", c.in, gotEntity, gotID, c.entity, c.id)
		}
	}
}

func TestInferEntityType(t *testing.T) {
	if got := InferEntityType("https://www.goodreads.com/author/show/153394"); got != "author" {
		t.Errorf("InferEntityType author = %q", got)
	}
	// Unknown URLs default to book.
	if got := InferEntityType("https://www.goodreads.com/something/else"); got != "book" {
		t.Errorf("InferEntityType unknown = %q, want book", got)
	}
}

func TestURLBuilders(t *testing.T) {
	if got := BookURL("2767052-the-hunger-games"); got != "https://www.goodreads.com/book/show/2767052" {
		t.Errorf("BookURL = %q", got)
	}
	if got := AuthorURL("153394.Suzanne_Collins"); got != "https://www.goodreads.com/author/show/153394" {
		t.Errorf("AuthorURL = %q", got)
	}
	if got := ListURL("1.Best_Books_Ever"); got != "https://www.goodreads.com/list/show/1.Best_Books_Ever" {
		t.Errorf("ListURL = %q", got)
	}
	if got := GenreURL("https://www.goodreads.com/genres/fantasy"); got != "https://www.goodreads.com/genres/fantasy" {
		t.Errorf("GenreURL passthrough = %q", got)
	}
}
