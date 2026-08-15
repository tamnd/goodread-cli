package goodread

import "testing"

// TestParseRefAcceptsEveryShape is the table the one id parser exists for.
//
// v0.2.0 had a Classify call plus a numericPrefix call at each call site, so
// commands quietly accepted slightly different things. Every shape lives here
// now, which means adding one adds it everywhere.
func TestParseRefAcceptsEveryShape(t *testing.T) {
	cases := []struct {
		in     string
		entity string
		id     string
		slug   string
		extra  string
	}{
		{"2767052", "book", "2767052", "", ""},
		{"2767052-the-hunger-games", "book", "2767052", "the-hunger-games", ""},
		{"https://www.goodreads.com/book/show/2767052", "book", "2767052", "", ""},
		{"https://www.goodreads.com/book/show/2767052-the-hunger-games", "book", "2767052", "the-hunger-games", ""},
		{"https://www.goodreads.com/book/show/2767052?ref=nav", "book", "2767052", "", ""},
		{"/book/show/2767052", "book", "2767052", "", ""},
		{"gr:book/2767052", "book", "2767052", "", ""},

		{"https://www.goodreads.com/work/editions/2792775", "work", "2792775", "", ""},
		{"https://www.goodreads.com/work/quotes/2792775", "work", "2792775", "", ""},
		{"https://www.goodreads.com/work/2792775-the-hunger-games", "work", "2792775", "the-hunger-games", ""},
		{"gr:work/2792775", "work", "2792775", "", ""},

		{"https://www.goodreads.com/author/show/153394.Suzanne_Collins", "author", "153394.Suzanne_Collins", "", ""},
		{"https://www.goodreads.com/author/show/153394-suzanne-collins", "author", "153394", "suzanne-collins", ""},
		{"https://www.goodreads.com/series/73758-the-hunger-games", "series", "73758", "the-hunger-games", ""},
		{"https://www.goodreads.com/user/show/221050", "user", "221050", "", ""},
		{"https://www.goodreads.com/quotes/17362", "quote", "17362", "", ""},

		// Listopia keys its URLs on the whole "<num>.<slug>" string, so
		// truncating it gives a 404 rather than the list.
		{"https://www.goodreads.com/list/show/1.Best_Books_Ever", "list", "1.Best_Books_Ever", "Best_Books_Ever", ""},
		{"gr:list/1.Best_Books_Ever", "list", "1.Best_Books_Ever", "Best_Books_Ever", ""},

		// A genre has no numeric id at all.
		{"https://www.goodreads.com/genres/fantasy", "genre", "fantasy", "", ""},
		{"gr:genre/fantasy", "genre", "fantasy", "", ""},

		// A shelf is two things, and the second one lives in the query.
		{"https://www.goodreads.com/review/list/221050?shelf=to-read", "shelf", "221050", "", "to-read"},
		{"gr:shelf/221050/to-read", "shelf", "221050", "", "to-read"},
	}

	for _, c := range cases {
		got, err := ParseRef(c.in)
		if err != nil {
			t.Errorf("ParseRef(%q): %v", c.in, err)
			continue
		}
		if got.Entity != c.entity || got.ID != c.id || got.Slug != c.slug || got.Extra != c.extra {
			t.Errorf("ParseRef(%q) = %s/%s slug=%q extra=%q, want %s/%s slug=%q extra=%q",
				c.in, got.Entity, got.ID, got.Slug, got.Extra, c.entity, c.id, c.slug, c.extra)
		}
		if got.URL() == "" {
			t.Errorf("ParseRef(%q) has no URL, so no command could act on it", c.in)
		}
	}
}

// TestParseRefRejectsWhatItCannotRead. Guessing at a reference is worse than
// saying so: a wrong guess turns into a request for the wrong page.
func TestParseRefRejectsWhatItCannotRead(t *testing.T) {
	for _, in := range []string{
		"",
		"   ",
		"the hunger games",
		"https://example.com/book/show/1",
		"https://www.goodreads.com/topic/show/123",
		"gr:book",
		"gr:shelf/221050",
	} {
		if r, err := ParseRef(in); err == nil {
			t.Errorf("ParseRef(%q) = %+v, want an error", in, r)
		}
	}
}

// TestParseRefAsTakesTheCommandsWord holds the rule that a bare id means what
// the command says it means, and a URL means what it says it means.
func TestParseRefAsTakesTheCommandsWord(t *testing.T) {
	r, err := ParseRefAs("153394", "author")
	if err != nil {
		t.Fatalf("a bare id in the author command: %v", err)
	}
	if r.Entity != "author" || r.ID != "153394" {
		t.Errorf("got %+v, want author 153394", r)
	}

	// Pasting a book URL into the author command is a mistake worth hearing
	// about rather than silently reinterpreting.
	if _, err := ParseRefAs("https://www.goodreads.com/book/show/2767052", "author"); err == nil {
		t.Error("a book URL was accepted by the author command")
	}
	if _, err := ParseRefAs("gr:book/2767052", "author"); err == nil {
		t.Error("a gr:book reference was accepted by the author command")
	}
}

// TestRefURLsAreTheOnesTheOpsRegistryKnows keeps the parser and the surface
// registry from drifting apart, since a URL nothing can fetch is no use.
func TestRefURLsAreTheOnesTheOpsRegistryKnows(t *testing.T) {
	for _, in := range []string{
		"gr:book/2767052",
		"gr:author/153394",
		"gr:series/73758",
		"gr:list/1.Best_Books_Ever",
		"gr:genre/fantasy",
		"gr:user/221050",
		"gr:quote/17362",
	} {
		r, err := ParseRef(in)
		if err != nil {
			t.Fatalf("ParseRef(%q): %v", in, err)
		}
		u := r.URL()
		if u == "" {
			t.Errorf("%s has no URL", in)
			continue
		}
		if e, _ := Classify(u); e == "" {
			t.Errorf("%s builds %s, which Classify does not recognise", in, u)
		}
	}
}
