package goodread

import (
	"encoding/json"
	"time"
)

// The v0.3.0 record model.
//
// Two rules run through all of it.
//
// A field the read did not look at is absent, not zero. That is why so much of
// this file is pointers: a book with no page count and a book with zero pages
// are not the same fact, and a model that cannot tell them apart pushes the
// ambiguity onto every consumer downstream.
//
// Every number carries provenance. A book page and a work page publish
// different numbers for the same named thing, edition against all editions, and
// a record that does not say which population it holds is not usable for
// anything quantitative.

// Envelope is the provenance every record carries.
type Envelope struct {
	Kind        string            `json:"kind"`
	Surfaces    []string          `json:"surfaces,omitempty"`
	Sources     []string          `json:"sources,omitempty"`
	RetrievedAt time.Time         `json:"retrieved_at"`
	BuildID     string            `json:"build_id,omitempty"`
	Via         map[string]string `json:"via,omitempty"`
	Level       map[string]int    `json:"level,omitempty"`
	Missed      []string          `json:"missed,omitempty"`

	// Robots is present only when the record was built from a page the site
	// asked us not to read, which is to say only under --no-robots. A record
	// with no robots block came off allowed surfaces, so the absence carries
	// as much meaning as the presence does.
	Robots *RobotsNote `json:"robots,omitempty"`
}

// RobotsNote is the robots.txt verdict on a page that was read anyway.
//
// Allowed is never omitted, so a consumer reading `robots.allowed` gets a
// false rather than a missing key, and Rule quotes the line from the file that
// decided it rather than paraphrasing. Anything downstream of a record can
// therefore tell that this data came from a page the site declined to offer,
// without knowing which flags the run was started with.
type RobotsNote struct {
	Allowed bool   `json:"allowed"`
	Path    string `json:"path"`
	Rule    string `json:"rule"`
	Source  string `json:"source,omitempty"`
}

// Book is one edition.
//
// Not a work. A work is what somebody wrote and a book is one printing of it,
// and conflating them is the single most common error in book data. See
// ~/notes/Spec/3008/04_graph.md section 2.
type Book struct {
	Envelope

	ID       string `json:"id"` // kca://book/amzn1.gr.book.v1....
	LegacyID int64  `json:"legacy_id"`
	WebURL   string `json:"web_url,omitempty"`

	Title         string `json:"title"`
	TitleComplete string `json:"title_complete,omitempty"`

	// Both descriptions, because they are two cache fields and not one.
	// Deriving the stripped form by stripping tags is lossy on books with
	// markup in them, and Goodreads has already done it correctly.
	Description         string `json:"description,omitempty"`
	DescriptionStripped string `json:"description_stripped,omitempty"`

	ISBN   string `json:"isbn,omitempty"`
	ISBN13 string `json:"isbn13,omitempty"`
	ASIN   string `json:"asin,omitempty"`
	Format string `json:"format,omitempty"`

	// NumPages is a pointer because a book with no page count and a book with
	// zero pages are different facts, and audiobooks legitimately have neither.
	NumPages  *int   `json:"num_pages,omitempty"`
	Publisher string `json:"publisher,omitempty"`

	// PublicationTime arrives as epoch milliseconds. A pointer, because a
	// missing date and 1970 are different facts. Negative values are valid and
	// common: anything published before 1970 has one.
	PublicationTime *time.Time `json:"publication_time,omitempty"`
	Language        string     `json:"language,omitempty"`

	Work         *Ref          `json:"work,omitempty"`
	Series       []SeriesEntry `json:"series,omitempty"`
	Contributors []Contributor `json:"contributors,omitempty"`
	Genres       []GenreRef    `json:"genres,omitempty"`

	ImageURL string `json:"image_url,omitempty"`

	// Links is kept whole and untyped. It is a block of buy links and
	// affiliate URLs whose shape changes, it is not worth a struct, and
	// dropping it loses the ebook and audiobook availability that lives in it.
	Links json.RawMessage `json:"links,omitempty"`

	Stats     *Stats     `json:"stats,omitempty"`
	Reviews   []Review   `json:"reviews,omitempty"`
	Shelvings []Shelving `json:"shelvings,omitempty"`

	Extra map[string]json.RawMessage `json:"extra,omitempty"`
}

// Work is the abstract thing. One work, many editions.
type Work struct {
	Envelope

	ID       string `json:"id"`
	LegacyID int64  `json:"legacy_id"`
	WebURL   string `json:"web_url,omitempty"`

	OriginalTitle   string     `json:"original_title,omitempty"`
	PublicationTime *time.Time `json:"publication_time,omitempty"`

	Stats *Stats `json:"stats,omitempty"`

	// Curated lists nothing else in the family has. A Place carries a name and
	// often a country and a year range; a Character carries a name and
	// sometimes a role. First class node types rather than strings on the work,
	// because "which books are set in Dublin" should be a query and not a grep.
	AwardsWon  []Award     `json:"awards_won,omitempty"`
	Places     []Place     `json:"places,omitempty"`
	Characters []Character `json:"characters,omitempty"`

	BestBook     *Ref          `json:"best_book,omitempty"`
	Editions     *Conn         `json:"editions,omitempty"`
	ChoiceAwards []ChoiceAward `json:"choice_awards,omitempty"`

	ShelvesURL string `json:"shelves_url,omitempty"`

	Extra map[string]json.RawMessage `json:"extra,omitempty"`
}

// Stats is the reason the rewrite is worth doing.
type Stats struct {
	AverageRating *float64 `json:"average_rating,omitempty"`
	RatingsCount  *int64   `json:"ratings_count,omitempty"`

	// RatingsCountDist is one star through five stars, in that order.
	//
	// Said explicitly because a five element array of integers with no labels
	// is exactly the kind of thing somebody reverses six months from now, and
	// nothing about the values themselves would give it away. StatsCheck
	// derives the mean from these buckets and compares it against the published
	// average, which is the check that catches a reversal.
	RatingsCountDist []int64 `json:"ratings_count_dist,omitempty"`

	TextReviewsCount *int64 `json:"text_reviews_count,omitempty"`

	// TextReviewsByLanguage is not rendered anywhere on the page.
	TextReviewsByLanguage []LanguageCount `json:"text_reviews_by_language,omitempty"`

	// Via names the surface these numbers came from, and it is not omitempty.
	//
	// The book page publishes this edition's numbers and a work read publishes
	// every edition's, and they are different populations with the same field
	// names. A consumer that averages across records without reading Via is
	// mixing the two and will not find out.
	Via string `json:"via"`
}

// LanguageCount is one language's share of the text reviews.
type LanguageCount struct {
	ISOLanguageCode string `json:"iso_language_code"`
	Count           int64  `json:"count"`
}

// Contributor is a person attached to a book, with the role they played.
type Contributor struct {
	ID       string `json:"id"`
	LegacyID int64  `json:"legacy_id,omitempty"`
	Name     string `json:"name"`

	// Role comes from the edge, not the node.
	//
	// primaryContributorEdge and secondaryContributorEdges each carry it, and
	// v0.2.0 flattened both into a list of names. An illustrator recorded as an
	// author is a wrong fact that propagates into every dataset built on top,
	// and nothing downstream can detect it.
	Role string `json:"role,omitempty"`

	Description     string `json:"description,omitempty"`
	ProfileImageURL string `json:"profile_image_url,omitempty"`
	WebURL          string `json:"web_url,omitempty"`

	// WorksURL is /author/list, which is a different page from the profile and
	// is the paginated list of everything they wrote. Only the suggest endpoint
	// hands it over, and it is the entry point to an author's whole catalogue.
	WorksURL string `json:"works_url,omitempty"`

	// IsGRAuthor says the author has claimed their profile, which is the signal
	// for whether the description is theirs or somebody else's.
	IsGRAuthor     *bool  `json:"is_gr_author,omitempty"`
	FollowersCount *int64 `json:"followers_count,omitempty"`

	Works *Conn `json:"works,omitempty"`

	// Extra is what the cache carried that the model has no home for.
	// Only Book and Work populate it today; the rest gain it when M4 ports the
	// surfaces that read them.
	Extra map[string]json.RawMessage `json:"extra,omitempty"`

	Via string `json:"via"`
}

// SeriesEntry places a book in a series.
type SeriesEntry struct {
	Series Ref `json:"series"`

	// Position is kept raw. Goodreads uses 2.5 for novellas and ranges like
	// 1-3 for omnibus editions, and a float alone cannot carry either.
	Position string `json:"position,omitempty"`

	// Number is the parsed form, present only when the position is a single
	// number. A range has no number and that is not an error.
	Number *float64 `json:"number,omitempty"`
}

// Review is one review of a book.
type Review struct {
	ID       string `json:"id"`
	LegacyID int64  `json:"legacy_id,omitempty"`

	// Rating is a pointer. A review with text and no rating is common and it is
	// not a zero star review.
	Rating *int `json:"rating,omitempty"`

	// Text is authored content. It is returned on request and a crawl does not
	// store it unless told to, per ~/notes/Spec/3008/00_overview.md section 4.
	Text string `json:"text,omitempty"`

	CreatedAt *time.Time `json:"created_at,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
	LikeCount *int64     `json:"like_count,omitempty"`
	Spoiler   *bool      `json:"spoiler,omitempty"`
	User      *Ref       `json:"user,omitempty"`

	// Extra is what the cache carried that the model has no home for.
	// Only Book and Work populate it today; the rest gain it when M4 ports the
	// surfaces that read them.
	Extra map[string]json.RawMessage `json:"extra,omitempty"`
	Via   string                     `json:"via"`

	// Robots is set on a review that came off /book/reviews, which robots.txt
	// disallows. A review is a row rather than a record and carries no
	// envelope, and `reviews --all` returns the book page's sample and the
	// walked pages in one list, so the note has to sit on the row or the two
	// sets become indistinguishable once they are merged.
	Robots *RobotsNote `json:"robots,omitempty"`
}

// Shelving is one person putting one book on one shelf.
//
// This is what the allowed surfaces give you in place of a shelf read. It comes
// off the book page and it answers "who shelved this and how", never "what is
// on this person's shelf". The user oriented view needs --no-robots and
// `goodread shelf`, and the two are not interchangeable.
type Shelving struct {
	ID        string `json:"id"`
	ShelfName string `json:"shelf_name,omitempty"`
	Rating    *int   `json:"rating,omitempty"`
	User      *Ref   `json:"user,omitempty"`

	// Extra is what the cache carried that the model has no home for.
	// Only Book and Work populate it today; the rest gain it when M4 ports the
	// surfaces that read them.
	Extra map[string]json.RawMessage `json:"extra,omitempty"`
	Via   string                     `json:"via"`
}

// User is thin on purpose.
//
// The only user data this tool sees is what a book page renders about a
// reviewer, and it does not go looking for more.
type User struct {
	ID       string `json:"id"`
	LegacyID int64  `json:"legacy_id,omitempty"`
	Name     string `json:"name,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
	WebURL   string `json:"web_url,omitempty"`
	IsAuthor *bool  `json:"is_author,omitempty"`

	// Extra is what the cache carried that the model has no home for.
	// Only Book and Work populate it today; the rest gain it when M4 ports the
	// surfaces that read them.
	Extra map[string]json.RawMessage `json:"extra,omitempty"`
	Via   string                     `json:"via"`
}

// GenreRef is a genre as it appears attached to a book.
type GenreRef struct {
	Name   string `json:"name"`
	WebURL string `json:"web_url,omitempty"`
}

// Award is a prize a work won.
type Award struct {
	Name         string `json:"name"`
	Category     string `json:"category,omitempty"`
	AwardedAt    *int64 `json:"awarded_at,omitempty"`
	Designation  string `json:"designation,omitempty"`
	WebURL       string `json:"web_url,omitempty"`
	HasWon       *bool  `json:"has_won,omitempty"`
	AwardedBooks *Conn  `json:"awarded_books,omitempty"`
}

// ChoiceAward is the Goodreads Choice Awards, which are their own thing.
type ChoiceAward struct {
	Year     int    `json:"year,omitempty"`
	Category string `json:"category,omitempty"`
	WebURL   string `json:"web_url,omitempty"`
}

// Place is somewhere a work is set.
type Place struct {
	Name        string `json:"name"`
	CountryName string `json:"country_name,omitempty"`
	Year        *int   `json:"year,omitempty"`
	WebURL      string `json:"web_url,omitempty"`
}

// Character is somebody in a work.
type Character struct {
	Name   string `json:"name"`
	Role   string `json:"role,omitempty"`
	WebURL string `json:"web_url,omitempty"`
}

// Conn is a GraphQL connection as the cache carries it, partly filled.
type Conn struct {
	TotalCount *int64 `json:"total_count,omitempty"`
	Loaded     int    `json:"loaded"`

	// Complete is not omitempty, and that is the point.
	//
	// A consumer that reads Nodes without reading Complete is going to be
	// wrong, and false is exactly the value it most needs to see. Hiding the
	// field when it is false would hide it in every case that matters.
	Complete bool `json:"complete"`

	Nodes   []Ref  `json:"nodes,omitempty"`
	NextURL string `json:"next_url,omitempty"`
}

// Depth is how much a read is willing to spend.
type Depth string

const (
	// DepthQuick and DepthMeta are the same request. They differ in how much of
	// the cache gets decoded, which is a real saving on a crawl because
	// decoding 58 User entities on every book page is not free.
	DepthQuick Depth = "quick"
	DepthMeta  Depth = "meta"
	DepthFull  Depth = "full"
	DepthDeep  Depth = "deep"
)

// Requests is the number of requests a depth costs for one book, before
// following anything.
func (d Depth) Requests() int {
	switch d {
	case DepthQuick, DepthMeta:
		return 1
	case DepthFull:
		return 3
	case DepthDeep:
		return 3 // plus one per contributor, which the caller counts
	}
	return 1
}

// ParseDepth validates a depth string.
func ParseDepth(s string) (Depth, bool) {
	switch Depth(s) {
	case DepthQuick, DepthMeta, DepthFull, DepthDeep:
		return Depth(s), true
	case "":
		return DepthMeta, true
	}
	return "", false
}

// Depths lists them in order of cost, for help text and validation messages.
func Depths() []Depth { return []Depth{DepthQuick, DepthMeta, DepthFull, DepthDeep} }
