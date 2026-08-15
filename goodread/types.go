// Package goodread is a library for reading public Goodreads data: books,
// authors, series, lists, quotes, users, shelves, genres, reviews, and search.
// Every page is normalized into a rich struct (JSON-LD first, HTML fallback),
// with explicit empty/zero/[] for genuinely absent fields. It carries no CLI
// knowledge and depends only on goquery and the standard library.
package goodread

import "time"

// Book is a Goodreads book page (/book/show/<id>).
type ScrapedBook struct {
	BookID             string    `json:"book_id" kit:"id"`
	WorkID             string    `json:"work_id"`
	Title              string    `json:"title"`
	TitleWithoutSeries string    `json:"title_without_series"`
	Description        string    `json:"description" kit:"body"`
	AuthorID           string    `json:"author_id" kit:"link,kind=goodreads/author"`
	AuthorName         string    `json:"author_name"`
	ISBN               string    `json:"isbn"`
	ISBN13             string    `json:"isbn13"`
	ASIN               string    `json:"asin"`
	AvgRating          float64   `json:"avg_rating"`
	RatingsCount       int64     `json:"ratings_count"`
	ReviewsCount       int64     `json:"reviews_count"`
	PublishedYear      int       `json:"published_year"`
	Publisher          string    `json:"publisher"`
	Language           string    `json:"language"`
	Pages              int       `json:"pages"`
	Format             string    `json:"format"`
	SeriesID           string    `json:"series_id" kit:"link,kind=goodreads/series,optional"`
	SeriesName         string    `json:"series_name"`
	SeriesPosition     string    `json:"series_position"`
	Genres             []string  `json:"genres"`
	CoverURL           string    `json:"cover_url"`
	URL                string    `json:"url"`
	SimilarBookIDs     []string  `json:"similar_book_ids" kit:"link,kind=goodreads/book,optional"`
	FetchedAt          time.Time `json:"fetched_at"`
}

// Author is a Goodreads author page (/author/show/<id>).
type Author struct {
	AuthorID       string    `json:"author_id" kit:"id"`
	Name           string    `json:"name"`
	Bio            string    `json:"bio" kit:"body"`
	PhotoURL       string    `json:"photo_url"`
	Website        string    `json:"website"`
	BornDate       string    `json:"born_date"`
	DiedDate       string    `json:"died_date"`
	Hometown       string    `json:"hometown"`
	Influences     []string  `json:"influences"`
	Genres         []string  `json:"genres"`
	AvgRating      float64   `json:"avg_rating"`
	RatingsCount   int64     `json:"ratings_count"`
	ReviewsCount   int64     `json:"reviews_count"`
	BooksCount     int       `json:"books_count"`
	FollowersCount int       `json:"followers_count"`
	NotableBookIDs []string  `json:"notable_book_ids" kit:"link,kind=goodreads/book,optional"`
	URL            string    `json:"url"`
	FetchedAt      time.Time `json:"fetched_at"`
}

// Series is a Goodreads book series (/series/<id>).
type Series struct {
	SeriesID         string    `json:"series_id" kit:"id"`
	Name             string    `json:"name"`
	Description      string    `json:"description" kit:"body"`
	AuthorID         string    `json:"author_id" kit:"link,kind=goodreads/author,optional"`
	AuthorName       string    `json:"author_name"`
	TotalBooks       int       `json:"total_books"`
	PrimaryWorkCount int       `json:"primary_work_count"`
	BookIDs          []string  `json:"book_ids" kit:"link,kind=goodreads/book,optional"`
	URL              string    `json:"url"`
	FetchedAt        time.Time `json:"fetched_at"`
}

// SeriesBook links a series to a book at a position. PositionLabel keeps the
// raw Goodreads label ("Book 1", "Book 0.5") since series positions are not
// always whole numbers.
type SeriesBook struct {
	SeriesID      string `json:"series_id"`
	BookID        string `json:"book_id"`
	Position      int    `json:"position"`
	PositionLabel string `json:"position_label"`
}

// List is a Goodreads Listopia list (/list/show/<id>).
type List struct {
	ListID          string    `json:"list_id" kit:"id"`
	Name            string    `json:"name"`
	Description     string    `json:"description" kit:"body"`
	BooksCount      int       `json:"books_count"`
	VotersCount     int       `json:"voters_count"`
	LikesCount      int       `json:"likes_count"`
	Tags            []string  `json:"tags"`
	CreatedByUser   string    `json:"created_by_user"`
	CreatedByUserID string    `json:"created_by_user_id" kit:"link,kind=goodreads/user,optional"`
	CreatedDate     string    `json:"created_date"`
	URL             string    `json:"url"`
	FetchedAt       time.Time `json:"fetched_at"`
}

// ListBook links a list to a book with its rank, score, votes, and the book's
// aggregate rating as shown on the list row.
type ListBook struct {
	ListID       string  `json:"list_id"`
	BookID       string  `json:"book_id"`
	Rank         int     `json:"rank"`
	Score        int     `json:"score"`
	Votes        int     `json:"votes"`
	AvgRating    float64 `json:"avg_rating"`
	RatingsCount int64   `json:"ratings_count"`
}

// Review is a Goodreads book review.
type ScrapedReview struct {
	ReviewID   string    `json:"review_id" kit:"id"`
	BookID     string    `json:"book_id" kit:"link,kind=goodreads/book"`
	UserID     string    `json:"user_id" kit:"link,kind=goodreads/user,optional"`
	UserName   string    `json:"user_name"`
	Rating     int       `json:"rating"`
	Text       string    `json:"text" kit:"body"`
	DateAdded  time.Time `json:"date_added"`
	LikesCount int       `json:"likes_count"`
	IsSpoiler  bool      `json:"is_spoiler"`
	URL        string    `json:"url"`
	FetchedAt  time.Time `json:"fetched_at"`
}

// Quote is a Goodreads quote.
type Quote struct {
	QuoteID    string    `json:"quote_id" kit:"id"`
	Text       string    `json:"text" kit:"body"`
	AuthorID   string    `json:"author_id" kit:"link,kind=goodreads/author,optional"`
	AuthorName string    `json:"author_name"`
	BookID     string    `json:"book_id" kit:"link,kind=goodreads/book,optional"`
	BookTitle  string    `json:"book_title"`
	WorkID     string    `json:"work_id"`
	LikesCount int       `json:"likes_count"`
	Tags       []string  `json:"tags"`
	URL        string    `json:"url"`
	FetchedAt  time.Time `json:"fetched_at"`
}

// User is a public Goodreads reader profile (/user/show/<id>).
type ScrapedUser struct {
	UserID         string    `json:"user_id" kit:"id"`
	Name           string    `json:"name"`
	Username       string    `json:"username"`
	Location       string    `json:"location"`
	JoinedDate     time.Time `json:"joined_date"`
	FriendsCount   int       `json:"friends_count"`
	BooksCount     int       `json:"books_count"`
	BooksReadCount int       `json:"books_read_count"`
	RatingsCount   int       `json:"ratings_count"`
	ReviewsCount   int       `json:"reviews_count"`
	AvgRating      float64   `json:"avg_rating"`
	Bio            string    `json:"bio" kit:"body"`
	Website        string    `json:"website"`
	AvatarURL      string    `json:"avatar_url"`

	FavoriteBookIDs         []string  `json:"favorite_book_ids" kit:"link,kind=goodreads/book,optional"`
	CurrentlyReadingBookIDs []string  `json:"currently_reading_book_ids" kit:"link,kind=goodreads/book,optional"`
	URL                     string    `json:"url"`
	FetchedAt               time.Time `json:"fetched_at"`
}

// Genre is a Goodreads genre/taxonomy node (/genres/<slug>).
type Genre struct {
	Slug          string    `json:"slug" kit:"id"`
	Name          string    `json:"name"`
	Description   string    `json:"description" kit:"body"`
	BooksCount    int       `json:"books_count"`
	BookIDs       []string  `json:"book_ids" kit:"link,kind=goodreads/book,optional"`
	RelatedGenres []string  `json:"related_genres" kit:"link,kind=goodreads/genre,optional"`
	URL           string    `json:"url"`
	FetchedAt     time.Time `json:"fetched_at"`
}

// Shelf is a user's named reading shelf.
type Shelf struct {
	ShelfID    string    `json:"shelf_id" kit:"id"` // "<user_id>/<shelf_name>"
	UserID     string    `json:"user_id" kit:"link,kind=goodreads/user"`
	Name       string    `json:"name"`
	BooksCount int       `json:"books_count"`
	URL        string    `json:"url"`
	FetchedAt  time.Time `json:"fetched_at"`
}

// ShelfBook links a shelf to a book with reading metadata. The RSS feed
// populates the richer fields (author/isbn/review/...); the HTML fallback fills
// only the basics.
type ShelfBook struct {
	ShelfID         string    `json:"shelf_id"`
	UserID          string    `json:"user_id"`
	BookID          string    `json:"book_id"`
	Title           string    `json:"title"`
	AuthorName      string    `json:"author_name,omitempty"`
	ISBN            string    `json:"isbn,omitempty"`
	NumPages        int       `json:"num_pages,omitempty"`
	AvgRating       float64   `json:"avg_rating,omitempty"`
	Rating          int       `json:"rating"`
	Shelves         []string  `json:"shelves,omitempty"`
	ReviewID        string    `json:"review_id,omitempty"`
	ReviewText      string    `json:"review_text,omitempty"`
	BookDescription string    `json:"book_description,omitempty"`
	BookPublished   int       `json:"book_published,omitempty"`
	CoverURL        string    `json:"cover_url,omitempty"`
	DateAdded       time.Time `json:"date_added"`
	DateCreated     time.Time `json:"date_created"`
	DateRead        time.Time `json:"date_read"`
}

// SearchResult is one result from autocomplete or full search.
type SearchResult struct {
	Position   int    `json:"position"`
	URL        string `json:"url"`
	EntityType string `json:"entity_type"` // book or author
	Title      string `json:"title"`
}

// QueueItem is one row of the crawl queue.
type QueueItem struct {
	ID         int64  `json:"id"`
	URL        string `json:"url"`
	EntityType string `json:"entity_type"`
	Priority   int    `json:"priority"`
	HTMLPath   string `json:"html_path"` // set when status='fetched'
}
