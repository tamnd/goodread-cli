package goodread

import (
	"encoding/json"
	"time"
)

// The records for the six Rails surfaces: author, series, list, genre,
// editions and quotes.
//
// Same two rules as the book record. A field the read did not look at is
// absent rather than zero, and every number says where it came from. The
// difference is where the numbers come from. The book page hands over a typed
// Apollo cache; these pages hand over microdata, og: tags, one React island on
// the series page, and otherwise markup. So the level map on these records
// leans on 2 and 3, and that is the surface being honest about itself rather
// than the extractor being lazy.
//
// One thing worth saying up front about every count on these pages. An author's
// average rating is over their books, a series has a work count and a book
// count that differ, and a list's rating is the book's rating and not the
// list's. Each field below says which population it holds, because a number
// with no population attached is not usable for anything.

// Author is a person who wrote something, as /author/show renders them.
//
// Not a Contributor. Contributor is the person as a book page names them, with
// the role they played on that book. This is the person's own page, and it
// carries what only that page has: the biography, the birth and death dates,
// the influences, and the aggregate over everything they wrote.
type Author struct {
	Envelope

	ID       string `json:"id,omitempty"` // kca, when a surface gives one
	LegacyID int64  `json:"legacy_id"`
	WebURL   string `json:"web_url,omitempty"`

	Name string `json:"name"`

	// Both forms of the biography, for the same reason the book keeps both
	// descriptions. The HTML one has the links in it, which on an author page
	// are usually pen names and other profiles, and stripping it loses them.
	Bio     string `json:"bio,omitempty"`
	BioHTML string `json:"bio_html,omitempty"`

	ImageURL string `json:"image_url,omitempty"`

	// BornAt and DiedAt are kept as Goodreads writes them.
	//
	// "July 31, 1965" parses, and so does "1890", and so does "circa 1564".
	// Parsing the ones that parse and dropping the rest would lose the ones that
	// are only approximately known, which is most of the interesting ones.
	BornAt            string `json:"born_at,omitempty"`
	BornIn            string `json:"born_in,omitempty"`
	DiedAt            string `json:"died_at,omitempty"`
	Website           string `json:"website,omitempty"`
	Twitter           string `json:"twitter,omitempty"`
	IsGoodreadsAuthor *bool  `json:"is_goodreads_author,omitempty"`

	Genres     []GenreRef `json:"genres,omitempty"`
	Influences []Ref      `json:"influences,omitempty"`

	// Stats here is the aggregate over this author's books, which is a
	// different population from any one book's stats. The distribution is not
	// on this page, so RatingsCountDist is empty and that is row two of the
	// four states rather than a flat distribution.
	Stats *Stats `json:"stats,omitempty"`

	FollowersCount *int64 `json:"followers_count,omitempty"`

	// Works is the page's sample, and Works.TotalCount is the "665 distinct
	// works" the header prints. The two are rarely the same number and the
	// record says so rather than looking complete.
	Works *Conn      `json:"works,omitempty"`
	Books []BookCard `json:"books,omitempty"`

	Extra map[string]json.RawMessage `json:"extra,omitempty"`
}

// Series is an ordered set of works, as /series renders it.
type Series struct {
	Envelope

	ID       string `json:"id,omitempty"`
	LegacyID int64  `json:"legacy_id"`
	WebURL   string `json:"web_url,omitempty"`

	Title       string `json:"title"`
	Description string `json:"description,omitempty"`

	// Two counts, and they are different facts. Harry Potter is 7 primary works
	// and 9 total works, the difference being companion volumes and omnibus
	// editions. Collapsing them into one number is how a series ends up with
	// the wrong length in every dataset built downstream.
	PrimaryWorkCount *int64 `json:"primary_work_count,omitempty"`
	TotalWorkCount   *int64 `json:"total_work_count,omitempty"`

	Books []SeriesBookCard `json:"books,omitempty"`

	Extra map[string]json.RawMessage `json:"extra,omitempty"`
}

// SeriesBookCard is one book in a series, at its position.
type SeriesBookCard struct {
	BookCard

	// Position is the raw header, "Book 1" or "Book 0.5" or "Books 1-3".
	Position string `json:"position,omitempty"`

	// Number is the parsed form when there is one. An omnibus spanning 1-3 has
	// a position and no number, which is not an error.
	Number *float64 `json:"number,omitempty"`
}

// List is a Listopia list, user built and vote ranked.
type List struct {
	Envelope

	ID     string `json:"id"` // "1.Best_Books_Ever", kept whole because the URL is
	WebURL string `json:"web_url,omitempty"`

	Title       string `json:"title"`
	Description string `json:"description,omitempty"`

	BooksCount  *int64 `json:"books_count,omitempty"`
	VotersCount *int64 `json:"voters_count,omitempty"`
	LikesCount  *int64 `json:"likes_count,omitempty"`

	Tags      []string `json:"tags,omitempty"`
	CreatedBy *Ref     `json:"created_by,omitempty"`
	CreatedAt string   `json:"created_at,omitempty"`

	// Page and Books are the page that was read, not the list. A list of 60,000
	// books renders 100 at a time and a record that did not say which hundred
	// these are would be useless for anything cumulative.
	Page  int        `json:"page,omitempty"`
	Books []ListCard `json:"books,omitempty"`

	Extra map[string]json.RawMessage `json:"extra,omitempty"`
}

// ListCard is one ranked row of a Listopia list.
type ListCard struct {
	BookCard

	Rank int `json:"rank,omitempty"`

	// Score is Listopia's own weighting and Votes is the count of people. They
	// are not proportional, since a vote at rank 1 is worth more than a vote at
	// rank 50, which is exactly why both are kept.
	Score *int64 `json:"score,omitempty"`
	Votes *int64 `json:"votes,omitempty"`
}

// Genre is a taxonomy node, as /genres/<slug> renders it.
type Genre struct {
	Envelope

	// Slug is the id. Names collide and get retitled and the slug in the URL is
	// what everything else keys on.
	Slug   string `json:"slug"`
	Name   string `json:"name"`
	WebURL string `json:"web_url,omitempty"`

	Description string `json:"description,omitempty"`

	// Related is the genre graph, which is the part of this page that nothing
	// else on the site gives you.
	Related []Ref `json:"related,omitempty"`

	// Books is what the page features today, which is a rotating editorial
	// selection and not a stable membership list. Named for what it is.
	Books []BookCard `json:"books,omitempty"`

	Extra map[string]json.RawMessage `json:"extra,omitempty"`
}

// Editions is every printing of one work, from /work/editions.
//
// The single most useful traversal in book data and the reason the robots.txt
// exception matters. A work has one canonical entry and forty ISBNs, and this
// is where the forty live.
type Editions struct {
	Envelope

	Work   Ref    `json:"work"`
	WebURL string `json:"web_url,omitempty"`
	Title  string `json:"title,omitempty"`

	// TotalCount is what the page says exists and Page is what was read.
	TotalCount *int64 `json:"total_count,omitempty"`
	Page       int    `json:"page,omitempty"`
	Complete   bool   `json:"complete"`

	Editions []Edition `json:"editions,omitempty"`

	Extra map[string]json.RawMessage `json:"extra,omitempty"`
}

// Edition is one printing, as the editions page renders it.
type Edition struct {
	Book Ref `json:"book"`

	Title string `json:"title,omitempty"`

	// Name is the designation, Format is the physical thing. The page renders
	// them in one line, "Reprint Edition, Kindle Edition, 387 pages", and they
	// are not the same fact: two printings can share a format and differ only
	// in this, which is the entire reason both rows exist on the page.
	Name   string `json:"name,omitempty"`
	Format string `json:"format,omitempty"`

	ISBN      string `json:"isbn,omitempty"`
	ISBN13    string `json:"isbn13,omitempty"`
	ASIN      string `json:"asin,omitempty"`
	Language  string `json:"language,omitempty"`
	Publisher string `json:"publisher,omitempty"`

	PublishedAt string `json:"published_at,omitempty"`
	NumPages    *int   `json:"num_pages,omitempty"`

	AverageRating *float64 `json:"average_rating,omitempty"`
	RatingsCount  *int64   `json:"ratings_count,omitempty"`

	Contributors []Contributor `json:"contributors,omitempty"`

	Via string `json:"via"`
}

// Quotes is the quote set of a work or an author.
type Quotes struct {
	Envelope

	// Subject is whichever of the two this page is about. /work/quotes is a
	// work and /author/quotes is an author, and a record that did not say which
	// would be ambiguous the moment two of them are in the same file.
	Subject Ref    `json:"subject"`
	WebURL  string `json:"web_url,omitempty"`

	TotalCount *int64 `json:"total_count,omitempty"`
	Page       int    `json:"page,omitempty"`
	Complete   bool   `json:"complete"`

	Quotes []Quote `json:"quotes,omitempty"`

	Extra map[string]json.RawMessage `json:"extra,omitempty"`
}

// Quote is one quotation.
type Quote struct {
	ID     string `json:"id,omitempty"`
	WebURL string `json:"web_url,omitempty"`

	// Text is authored content, and 00_overview.md section 4 governs it the
	// same way it governs review text: returned on request, not stored by a
	// crawl unless asked for.
	Text string `json:"text"`

	Author *Ref     `json:"author,omitempty"`
	Book   *Ref     `json:"book,omitempty"`
	Tags   []string `json:"tags,omitempty"`

	LikeCount *int64 `json:"like_count,omitempty"`

	Via string `json:"via"`
}

// BookCard is a book as some other page renders it in a row.
//
// Deliberately not a Book. A Book is an edition read from its own page with a
// level map and a missed list; this is the four or five fields a listing shows,
// and calling it a Book would be claiming a read that did not happen. Every
// card carries the ref, so `goodread book <id>` is the upgrade path and the
// record says so rather than implying it already has the data.
type BookCard struct {
	Book Ref `json:"book"`

	// Work is here because the listing pages give it away for free, in the
	// editions link. Getting from an edition to its work usually costs a
	// request, and these pages hand it over.
	Work *Ref `json:"work,omitempty"`

	Title string `json:"title,omitempty"`

	// TitleBare is the title without the series suffix, when the page gives
	// both. "Harry Potter and the Philosopher's Stone (Harry Potter, #1)" and
	// the bare form are both worth having, and deriving one from the other by
	// stripping parentheses is wrong on every book with parentheses in its
	// actual title.
	TitleBare string `json:"title_bare,omitempty"`

	ImageURL string `json:"image_url,omitempty"`

	Contributors []Contributor `json:"contributors,omitempty"`

	AverageRating    *float64 `json:"average_rating,omitempty"`
	RatingsCount     *int64   `json:"ratings_count,omitempty"`
	TextReviewsCount *int64   `json:"text_reviews_count,omitempty"`
	NumPages         *int     `json:"num_pages,omitempty"`

	// PublishedAt is a year as printed, because that is all a listing row gives
	// and inventing a January 1st to make it a date would be inventing a fact.
	PublishedAt string `json:"published_at,omitempty"`

	EditionsCount *int64 `json:"editions_count,omitempty"`
	EditionsURL   string `json:"editions_url,omitempty"`

	Description string `json:"description,omitempty"`

	Via string `json:"via"`
}

// envelopeOf fills the provenance every record carries.
//
// One function rather than the same fifteen lines at the top of six From
// constructors, because the day a field is added to Envelope is the day five of
// those six would have been missed.
func envelopeOf(e *Extractor, kind string, retrievedAt time.Time) Envelope {
	env := Envelope{
		Kind:        kind,
		RetrievedAt: retrievedAt,
		Surfaces:    []string{e.Surface},
		Sources:     []string{SurfaceSource(e.Surface)},
		Level:       Levels{},
		Via:         viaOf(e),
	}
	for k, v := range e.Levels {
		env.Level[k] = v
	}
	env.Missed = append(env.Missed, e.Missed...)
	env.BuildID, _ = e.Fields["build_id"].(string)
	return env
}
