package goodread

import (
	"strings"
	"testing"
	"time"
)

const bestBooksURL = "https://www.goodreads.com/list/show/1.Best_Books_Ever"

func listFromCapture(t *testing.T) *List {
	t.Helper()
	e, err := ExtractList(readCapture(t, "list_show_1.html.gz"), bestBooksURL)
	if err != nil {
		t.Fatalf("ExtractList: %v", err)
	}
	l, err := ListFrom(e, "1.Best_Books_Ever", time.Now())
	if err != nil {
		t.Fatalf("ListFrom: %v", err)
	}
	return l
}

func TestListReadsItsHeader(t *testing.T) {
	l := listFromCapture(t)

	if l.Title != "Best Books Ever" {
		t.Errorf("title = %q", l.Title)
	}
	if !strings.Contains(l.Description, "voted on by the general Goodreads community") {
		t.Errorf("description = %q", l.Description)
	}
	// The description div also holds a flag link, and a description with the
	// word flag stuck on the end is the sign it was taken whole.
	if strings.Contains(l.Description, "flag") {
		t.Errorf("the flag link leaked into the description: %q", l.Description)
	}
	if l.BooksCount == nil || *l.BooksCount != 79111 {
		t.Errorf("books count = %v, want 79111", l.BooksCount)
	}
	if l.VotersCount == nil || *l.VotersCount != 295014 {
		t.Errorf("voters = %v, want 295014", l.VotersCount)
	}
	if l.ID != "1.Best_Books_Ever" {
		t.Errorf("id = %q, and the slug is part of it because the URL needs it", l.ID)
	}
}

// TestListRanksLineUpWithBooks is the bug worth guarding.
//
// The rank comes from the row and the book comes from a second pass over the
// same rows, and if those two sequences ever drift by one then every book on
// the page is filed under the rank of its neighbour. Nothing about the output
// would look wrong.
func TestListRanksLineUpWithBooks(t *testing.T) {
	l := listFromCapture(t)

	if len(l.Books) != 100 {
		t.Fatalf("%d books, and a Listopia page renders 100", len(l.Books))
	}
	for i, b := range l.Books {
		if b.Rank != i+1 {
			t.Fatalf("book %d ranked %d, so the two passes are out of step", i, b.Rank)
		}
	}
	first := l.Books[0]
	if first.Title != "The Hunger Games (The Hunger Games, #1)" {
		t.Errorf("rank 1 is %q", first.Title)
	}
	if first.TitleBare != "The Hunger Games" {
		t.Errorf("bare title = %q", first.TitleBare)
	}
	if first.Book.ID != "2767052" {
		t.Errorf("rank 1 book id = %q", first.Book.ID)
	}
	if len(first.Contributors) == 0 || first.Contributors[0].Name != "Suzanne Collins" {
		t.Errorf("rank 1 contributors = %+v", first.Contributors)
	}
	if first.Contributors[0].ID != "153394" {
		t.Errorf("author id = %q", first.Contributors[0].ID)
	}
}

// TestListKeepsScoreAndVotes. Two different measurements and not one, because a
// vote at rank 1 is worth more than a vote at rank 50 and only Listopia knows
// the weighting.
func TestListKeepsScoreAndVotes(t *testing.T) {
	l := listFromCapture(t)

	first := l.Books[0]
	if first.Score == nil || *first.Score != 4518690 {
		t.Errorf("rank 1 score = %v, want 4518690", first.Score)
	}
	if first.Votes == nil || *first.Votes != 45920 {
		t.Errorf("rank 1 votes = %v, want 45920", first.Votes)
	}
	if first.Score != nil && first.Votes != nil && *first.Score == *first.Votes {
		t.Error("score and votes are the same number, so one of them is being read off the other")
	}

	var withScore, withVotes, withRating int
	for _, b := range l.Books {
		if b.Score != nil {
			withScore++
		}
		if b.Votes != nil {
			withVotes++
		}
		if b.AverageRating != nil && b.RatingsCount != nil {
			withRating++
		}
	}
	if withScore < 95 || withVotes < 95 {
		t.Errorf("%d scores and %d vote counts out of %d rows", withScore, withVotes, len(l.Books))
	}
	if withRating < 95 {
		t.Errorf("%d of %d rows carry a rating", withRating, len(l.Books))
	}

	// Scores descend, because the page is sorted by them. A break here means
	// the score is being picked up from the wrong element on the row.
	for i := 1; i < len(l.Books); i++ {
		a, b := l.Books[i-1], l.Books[i]
		if a.Score != nil && b.Score != nil && *b.Score > *a.Score {
			t.Errorf("rank %d scores %d above rank %d at %d", b.Rank, *b.Score, a.Rank, *a.Score)
			break
		}
	}
}

// TestListSaysItIsOnePage. A hundred books of 79,111, and a record that did not
// say so is the sort of thing somebody builds a "complete" dataset out of.
func TestListSaysItIsOnePage(t *testing.T) {
	l := listFromCapture(t)

	var said bool
	for _, m := range l.Missed {
		if strings.Contains(m, "79111") || strings.Contains(m, "79,111") {
			said = true
		}
	}
	if !said {
		t.Errorf("nothing in missed names the shortfall: %v", l.Missed)
	}
	if l.Page != 1 {
		t.Errorf("page = %d, want 1", l.Page)
	}
}

func TestParseListTwitterTitle(t *testing.T) {
	for _, c := range []struct {
		in    string
		title string
		count int64
	}{
		{"Best Books Ever (79111 books)", "Best Books Ever", 79111},
		{"Books I Own (1 book)", "Books I Own", 1},
		{"A List (1,024 books)", "A List", 1024},
		{"No Count Here", "No Count Here", 0},
		{"", "", 0},
	} {
		title, count := parseListTwitterTitle(c.in)
		if title != c.title || count != c.count {
			t.Errorf("parseListTwitterTitle(%q) = %q, %d, want %q, %d", c.in, title, count, c.title, c.count)
		}
	}
}
