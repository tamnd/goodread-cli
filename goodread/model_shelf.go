package goodread

import (
	"encoding/json"
	"time"
)

// The shelf on the v0.3.0 model.
//
// A shelf is not like the other surfaces. Everything else here is a fact about
// a book that Goodreads published; a shelf is a fact about a person, and the
// rows carry their rating, their dates and their review text. 00_overview.md
// section 4 governs the review text the same way it governs a book's reviews:
// it is returned when somebody asks for the shelf, and it is not swept up by a
// crawl that was after something else.
//
// Two ways in, and they are different reads rather than two routes to the same
// data. The RSS feed is the application serialising its own rows, so it comes
// out at level 1 and carries the author, the ISBN, the page count, the shelves,
// the dates and the review. The HTML shelf is markup being read back at level 3
// and carries the basics. The record says which one it was, because a row with
// no ISBN means something different in each case.

// ShelfRecord is one user's named shelf.
type ShelfRecord struct {
	Envelope

	// ID is "<user_id>/<shelf_name>", which is the only thing that identifies a
	// shelf. Two people both have a shelf called read.
	ID     string `json:"id"`
	User   Ref    `json:"user"`
	Name   string `json:"name"`
	WebURL string `json:"web_url,omitempty"`

	// Source is "rss" or "html". Not decoration: it decides which of the fields
	// below can be there at all, so a reader comparing two shelf records needs
	// it before the comparison means anything.
	Source string `json:"source"`

	// TotalCount is what the shelf says it holds and Loaded is what was read.
	// The RSS feed returns a fixed window and never says how much it left out,
	// which is stated in Missed rather than implied by a smaller number.
	TotalCount *int64 `json:"total_count,omitempty"`
	Loaded     int    `json:"loaded"`
	Complete   bool   `json:"complete"`

	Books []ShelfCard `json:"books,omitempty"`

	Extra map[string]json.RawMessage `json:"extra,omitempty"`
}

// ShelfCard is one book on a shelf, with what the shelf owner did to it.
//
// A BookCard with the person's side attached. The book fields mean what they
// mean everywhere else; the rating, the dates and the review are this person's
// and nobody else's.
type ShelfCard struct {
	BookCard

	// Rating is the shelf owner's, out of five, and it is a pointer because
	// unrated and rated one star are different things and 0 says the first
	// while looking like a number.
	Rating *int `json:"rating,omitempty"`

	// Shelves is every shelf this person filed it on, which is where their own
	// tagging lives and the only place it exists.
	Shelves []string `json:"shelves,omitempty"`

	// ISBN is the edition the person actually shelved, which is not always the
	// edition a book page would hand you for the same title.
	ISBN string `json:"isbn,omitempty"`

	ReviewID   string `json:"review_id,omitempty"`
	ReviewText string `json:"review_text,omitempty"`

	// The three dates are kept apart because they answer different questions.
	// Added is when it appeared on the shelf, Read is when they finished it,
	// and a book added in 2019 and read in 2023 is a fact about a to-read pile.
	DateAdded   *time.Time `json:"date_added,omitempty"`
	DateCreated *time.Time `json:"date_created,omitempty"`
	DateRead    *time.Time `json:"date_read,omitempty"`
}

// ShelfFrom folds the v0.2.0 shelf shapes into the record.
//
// The parsers stay where they are. They work, they are tested against real
// feeds, and rewriting them to produce the new shape would be churn with a
// chance of losing a field. What was missing was the envelope, so that is what
// this adds.
func ShelfFrom(s *Shelf, rows []ShelfBook, source string, retrievedAt time.Time) *ShelfRecord {
	// One surface, two routes. 01_surfaces.md numbers both /review/list and
	// /review/list_rss as s11, because they are the same shelf answered two
	// ways, and the level is where the difference actually shows.
	const surface = "s11"
	level := LevelSelector
	if source == "rss" {
		// The feed is the application serialising its own rows, not markup
		// being read back, so it sits on the same rung as the Apollo cache.
		level = LevelNextData
	}

	rec := &ShelfRecord{
		Envelope: Envelope{
			Kind:        "shelf",
			RetrievedAt: retrievedAt,
			Surfaces:    []string{surface},
			Sources:     []string{SurfaceSource(surface)},
			Level:       Levels{},
			Via:         map[string]string{},
		},
		Source: source,
		Loaded: len(rows),
	}
	if s != nil {
		rec.ID = s.ShelfID
		rec.Name = s.Name
		rec.WebURL = s.URL
		rec.User = Ref{Type: "User", ID: s.UserID, Key: "User:" + s.UserID, Resolved: s.UserID != ""}
		if s.BooksCount > 0 {
			n := int64(s.BooksCount)
			rec.TotalCount = &n
		}
	}
	for _, f := range []string{"id", "name", "books", "user"} {
		rec.Level[f] = level
		rec.Via[f] = surface
	}

	for _, r := range rows {
		card := ShelfCard{
			BookCard: BookCard{
				Book:      Ref{Type: "Book", ID: r.BookID, Key: "Book:" + r.BookID, Title: r.Title, Resolved: r.BookID != ""},
				Title:     r.Title,
				TitleBare: TitleWithoutSeries(r.Title),
				ImageURL:  r.CoverURL,
				Via:       surface,
			},
			ReviewID:   r.ReviewID,
			ReviewText: r.ReviewText,
			Shelves:    r.Shelves,
		}
		if r.AuthorName != "" {
			card.Contributors = []Contributor{{Name: r.AuthorName, Role: "Author", Via: surface}}
		}
		card.ISBN = r.ISBN
		if r.NumPages > 0 {
			n := r.NumPages
			card.NumPages = &n
		}
		if r.AvgRating > 0 {
			v := r.AvgRating
			card.AverageRating = &v
		}
		if r.BookPublished > 0 {
			card.PublishedAt = itoa(r.BookPublished)
		}
		if r.BookDescription != "" {
			card.Description = r.BookDescription
		}
		if r.Rating > 0 {
			v := r.Rating
			card.Rating = &v
		}
		card.DateAdded = timeOrNil(r.DateAdded)
		card.DateCreated = timeOrNil(r.DateCreated)
		card.DateRead = timeOrNil(r.DateRead)
		rec.Books = append(rec.Books, card)
	}

	rec.Complete = rec.TotalCount != nil && int64(rec.Loaded) >= *rec.TotalCount
	if source == "rss" {
		// Said every time, because the feed returns its window and never states
		// a total. Without this a shelf of 1,400 books looks like a shelf of 100.
		rec.Missed = append(rec.Missed,
			"the rss feed returns one window of the shelf and does not say how large the shelf is. `goodread shelf <user> --html --max-pages 0` walks the whole thing, which robots.txt disallows and needs --no-robots.")
	}
	return rec
}

// timeOrNil keeps "no date" apart from the zero time.
//
// A shelf row with no read date is a book somebody has not finished, and a
// record that printed 0001-01-01 for it would be stating a date that is not a
// date.
func timeOrNil(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
