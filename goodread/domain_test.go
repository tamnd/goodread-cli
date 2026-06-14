package goodread_test

import (
	"testing"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/goodread-cli/goodread"
)

// These tests exercise the kit driver wiring without any network: the blank
// import below registers the domain, and Mint/Links/Resolve are pure string and
// reflection work over the registry.

func TestDomainInfo(t *testing.T) {
	info := goodread.Domain{}.Info()
	if info.Scheme != "goodreads" {
		t.Errorf("scheme = %q, want goodreads", info.Scheme)
	}
	if len(info.Aliases) == 0 || info.Aliases[0] != "gr" {
		t.Errorf("aliases = %v, want [gr]", info.Aliases)
	}
}

func TestClassifyLocate(t *testing.T) {
	d := goodread.Domain{}
	typ, id, err := d.Classify("https://www.goodreads.com/book/show/2767052-the-hunger-games")
	if err != nil || typ != "book" || id != "2767052" {
		t.Fatalf("Classify = %q/%q/%v", typ, id, err)
	}
	loc, err := d.Locate("book", "2767052")
	if err != nil || loc != goodread.BookURL("2767052") {
		t.Errorf("Locate = %q/%v", loc, err)
	}
	if _, _, err := d.Classify("nonsense"); err == nil {
		t.Error("Classify(nonsense) = nil error, want error")
	}
}

func TestHostMintLinksResolve(t *testing.T) {
	h, err := kit.Open()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := h.Domain("goodreads"); !ok {
		t.Fatal("goodreads not mounted on host")
	}

	b := &goodread.Book{
		BookID:         "2767052",
		AuthorID:       "153394",
		SeriesID:       "73758",
		SimilarBookIDs: []string{"7260188", "6148028"},
	}
	minted, err := h.Mint(b)
	if err != nil || minted.String() != "goodreads://book/2767052" {
		t.Errorf("Mint = %q/%v", minted.String(), err)
	}

	got := map[string]bool{}
	for _, u := range h.Links(b) {
		got[u.String()] = true
	}
	for _, want := range []string{
		"goodreads://author/153394",
		"goodreads://series/73758",
		"goodreads://book/7260188",
		"goodreads://book/6148028",
	} {
		if !got[want] {
			t.Errorf("Links missing %q (got %v)", want, got)
		}
	}

	u, err := h.ResolveOn("goodreads", "2767052")
	if err != nil || u.String() != "goodreads://book/2767052" {
		t.Errorf("ResolveOn(bare) = %q/%v", u.String(), err)
	}
	u, err = h.Resolve("https://www.goodreads.com/author/show/153394-suzanne-collins")
	if err != nil || u.String() != "goodreads://author/153394" {
		t.Errorf("Resolve(url) = %q/%v", u.String(), err)
	}
}
