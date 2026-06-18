package goodread

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func mustDoc(t *testing.T, html string) *goquery.Document {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return doc
}

func TestFullCoverURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://i.gr-assets.com/books/1586722975i/2767052._SX50_.jpg", "https://i.gr-assets.com/books/1586722975i/2767052.jpg"},
		{"https://i.gr-assets.com/books/1593892032i/51901147._SY180_.jpg", "https://i.gr-assets.com/books/1593892032i/51901147.jpg"},
		{"https://i.gr-assets.com/books/2767052.jpg", "https://i.gr-assets.com/books/2767052.jpg"},
		{"", ""},
	}
	for _, c := range cases {
		if got := fullCoverURL(c.in); got != c.want {
			t.Errorf("fullCoverURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCleanText(t *testing.T) {
	if got := cleanText("Fantasy   is\n  a\tgenre &amp; more"); got != "Fantasy is a genre & more" {
		t.Errorf("cleanText = %q", got)
	}
}

// Series props carry avgRating as a bare number (the autocomplete endpoint sends
// it quoted); both must decode, and the per-book author becomes the series
// author.
func TestParseSeriesReactProps(t *testing.T) {
	const html = `<html><body>
<div data-react-class="ReactComponents.SeriesHeader"
  data-react-props='{"title":"The Hunger Games Series","subtitle":"3 primary works • 5 total works","description":{"html":"A dystopian trilogy.<br>By Suzanne Collins."}}'></div>
<div data-react-class="ReactComponents.SeriesList"
  data-react-props='{"seriesHeaders":["Book 0","Book 1"],"series":[
    {"book":{"imageUrl":"x._SX50_.jpg","bookId":"51901147","workId":"71367421","bookUrl":"/book/show/51901147","title":"The Ballad of Songbirds and Snakes","avgRating":3.99,"ratingsCount":1256980,"author":{"id":153394,"name":"Suzanne Collins","profileUrl":"/author/show/153394"}}},
    {"book":{"imageUrl":"y._SX50_.jpg","bookId":"2767052","workId":"2792775","bookUrl":"/book/show/2767052","title":"The Hunger Games","avgRating":4.35,"ratingsCount":10000000,"author":{"id":153394,"name":"Suzanne Collins","profileUrl":"/author/show/153394"}}}
  ]}'></div>
</body></html>`
	s, books, err := ParseSeries(mustDoc(t, html), "73758", "https://www.goodreads.com/series/73758")
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "The Hunger Games" {
		t.Errorf("Name = %q, want The Hunger Games", s.Name)
	}
	if s.PrimaryWorkCount != 3 || s.TotalBooks != 5 {
		t.Errorf("counts = primary %d total %d, want 3/5", s.PrimaryWorkCount, s.TotalBooks)
	}
	if s.AuthorID != "153394" || s.AuthorName != "Suzanne Collins" {
		t.Errorf("author = (%q,%q), want (153394, Suzanne Collins)", s.AuthorID, s.AuthorName)
	}
	if !strings.Contains(s.Description, "dystopian trilogy") {
		t.Errorf("Description = %q", s.Description)
	}
	if len(books) != 2 {
		t.Fatalf("got %d books, want 2", len(books))
	}
	if books[0].BookID != "51901147" || books[0].PositionLabel != "Book 0" {
		t.Errorf("book0 = (%q,%q)", books[0].BookID, books[0].PositionLabel)
	}
	if books[1].BookID != "2767052" || books[1].PositionLabel != "Book 1" {
		t.Errorf("book1 = (%q,%q)", books[1].BookID, books[1].PositionLabel)
	}
}

// The list name must come from <title>, not the visible "Score" tooltip h1; the
// stacked footer carries books/voters/creator/date/tags, and each row carries
// rank, rating, score, and votes.
func TestParseListStacked(t *testing.T) {
	const html = `<html><head><title>Best Books Ever (78510 books)</title></head><body>
<div class="pageHeader"><h2><a href="/list">Listopia</a> &gt; Best Books Ever</h2></div>
<div class="u-paddingBottomMedium mediumText">The best books ever.<div class="right"><a class="flag">flag</a></div></div>
<div id="score_explanation" style="display:none"><h1>Score</h1></div>
<table class="tableList">
<tr itemscope itemtype="http://schema.org/Book">
  <td class="number">1</td>
  <td><a class="bookTitle" href="/book/show/2767052-the-hunger-games"><span itemprop="name">The Hunger Games</span></a></td>
  <td><span class="minirating">4.35 avg rating &mdash; 10,133,880 ratings</span>
    <a href="#">score: 4,475,802</a>, <a href="#">45,487 people voted</a></td>
</tr>
</table>
<div class="stacked">
  <div>78,510 books &middot; 292,601 voters &middot; list created May 14th, 2008 by <a href="/user/show/73-michael-economy">Michael Economy</a></div>
  <div>Tags: <a href="/list/show_tag/best">best</a>, <a href="/list/show_tag/fiction">fiction</a></div>
</div>
<span class="likesCount">10640 likes</span>
</body></html>`
	l, books, err := ParseList(mustDoc(t, html), "1.Best_Books_Ever", "https://www.goodreads.com/list/show/1.Best_Books_Ever")
	if err != nil {
		t.Fatal(err)
	}
	if l.Name != "Best Books Ever" {
		t.Errorf("Name = %q, want Best Books Ever", l.Name)
	}
	if l.Description != "The best books ever." {
		t.Errorf("Description = %q", l.Description)
	}
	if l.BooksCount != 78510 || l.VotersCount != 292601 || l.LikesCount != 10640 {
		t.Errorf("counts = books %d voters %d likes %d", l.BooksCount, l.VotersCount, l.LikesCount)
	}
	if l.CreatedByUser != "Michael Economy" || l.CreatedByUserID != "73" || l.CreatedDate != "May 14th, 2008" {
		t.Errorf("creator = (%q,%q,%q)", l.CreatedByUser, l.CreatedByUserID, l.CreatedDate)
	}
	if len(l.Tags) != 2 || l.Tags[0] != "best" || l.Tags[1] != "fiction" {
		t.Errorf("Tags = %v", l.Tags)
	}
	if len(books) != 1 {
		t.Fatalf("got %d books, want 1", len(books))
	}
	b := books[0]
	if b.BookID != "2767052" || b.Rank != 1 || b.Score != 4475802 || b.Votes != 45487 {
		t.Errorf("book = id %q rank %d score %d votes %d", b.BookID, b.Rank, b.Score, b.Votes)
	}
	if b.AvgRating != 4.35 || b.RatingsCount != 10133880 {
		t.Errorf("book rating = avg %v count %d", b.AvgRating, b.RatingsCount)
	}
}

// The genre description must come from the hidden freeText span (the visible
// container is truncated), and related genres must be scoped to the box, not the
// global genre nav.
func TestParseGenreFreeText(t *testing.T) {
	const html = `<html><body>
<a href="/genres/art">Art</a><a href="/genres/business">Business</a>
<h1>Fantasy</h1>
<div class="mediumText reviewText">
  <span id="freeTextContainer123">Fantasy is a genre that uses magic&hellip;<a>more</a></span>
  <span id="freeText123" style="display:none">Fantasy is a genre that uses magic and other supernatural forms.</span>
</div>
<div class="bigBox"><div class="h2Container"><h2 class="brownBackground">Related Genres</h2></div>
  <div class="bigBoxBody"><a href="/genres/paranormal">Paranormal</a><a href="/genres/magic">Magic</a></div>
</div>
<a href="/book/show/111">A</a><a href="/book/show/222">B</a>
</body></html>`
	g, err := ParseGenre(mustDoc(t, html), "fantasy", "https://www.goodreads.com/genres/fantasy")
	if err != nil {
		t.Fatal(err)
	}
	if g.Name != "Fantasy" {
		t.Errorf("Name = %q", g.Name)
	}
	if g.Description != "Fantasy is a genre that uses magic and other supernatural forms." {
		t.Errorf("Description = %q", g.Description)
	}
	if len(g.RelatedGenres) != 2 || g.RelatedGenres[0] != "Paranormal" || g.RelatedGenres[1] != "Magic" {
		t.Errorf("RelatedGenres = %v, want [Paranormal Magic] (global nav must be excluded)", g.RelatedGenres)
	}
	if g.BooksCount != 2 || len(g.BookIDs) != 2 {
		t.Errorf("books = count %d ids %v", g.BooksCount, g.BookIDs)
	}
}

// quoteText/quoteFooter are flat siblings; each quote must read its own id,
// book, work, likes, and tags rather than the first quote's.
func TestParseQuotesSiblings(t *testing.T) {
	const html = `<html><body>
<a class="authorName" href="/author/show/153394.Suzanne_Collins">Suzanne Collins</a>
<div class="quotes">
  <div class="quoteText">&ldquo;You love me. Real or not real?&rdquo;<br/>&#8213;
    <span class="authorOrTitle">Suzanne Collins,</span>
    <span id="quote_book_link_7260188"><a class="authorOrTitle" href="/work/quotes/8812783">Mockingjay</a></span>
  </div>
  <div class="quoteFooter">
    <div class="left">tags: <a href="/quotes/tag/love">love</a></div>
    <div class="right"><a class="smallText" title="View this quote" href="/quotes/290707-you-love-me">28839 likes</a></div>
  </div>
  <div class="quoteText">&ldquo;Happy Hunger Games.&rdquo;<br/>&#8213;
    <span class="authorOrTitle">Suzanne Collins,</span>
    <span id="quote_book_link_2767052"><a class="authorOrTitle" href="/work/quotes/2792775">The Hunger Games</a></span>
  </div>
  <div class="quoteFooter">
    <div class="right"><a class="smallText" title="View this quote" href="/quotes/160907-happy-hunger-games">12000 likes</a></div>
  </div>
</div>
</body></html>`
	quotes, err := ParseQuotes(mustDoc(t, html), "https://www.goodreads.com/author/quotes/153394")
	if err != nil {
		t.Fatal(err)
	}
	if len(quotes) != 2 {
		t.Fatalf("got %d quotes, want 2", len(quotes))
	}
	q0, q1 := quotes[0], quotes[1]
	if q0.QuoteID == q1.QuoteID {
		t.Fatalf("quote ids must differ, both %q", q0.QuoteID)
	}
	if q0.QuoteID != "290707" || q0.BookID != "7260188" || q0.WorkID != "8812783" {
		t.Errorf("q0 = id %q book %q work %q", q0.QuoteID, q0.BookID, q0.WorkID)
	}
	if q0.AuthorID != "153394" || q0.AuthorName != "Suzanne Collins" || q0.BookTitle != "Mockingjay" {
		t.Errorf("q0 attribution = (%q,%q,%q)", q0.AuthorID, q0.AuthorName, q0.BookTitle)
	}
	if q0.LikesCount != 28839 || len(q0.Tags) != 1 || q0.Tags[0] != "love" {
		t.Errorf("q0 footer = likes %d tags %v", q0.LikesCount, q0.Tags)
	}
	if q1.QuoteID != "160907" || q1.BookID != "2767052" || q1.WorkID != "2792775" || q1.LikesCount != 12000 {
		t.Errorf("q1 = id %q book %q work %q likes %d", q1.QuoteID, q1.BookID, q1.WorkID, q1.LikesCount)
	}
	if len(q1.Tags) != 0 {
		t.Errorf("q1 tags = %v, want none", q1.Tags)
	}
}

// The user profile parses name/username/location/books from <title>, ratings and
// reviews from the stats line, read count from the read shelf, friends from the
// header (not the friend list), and currently-reading ids from their section.
func TestParseUserTitleAndStats(t *testing.T) {
	const html = `<html><head><title>Otis  Chandler (otis) - San Francisco, CA (1,648 books)</title></head><body>
<h1>Otis  Chandler</h1>
<img class="profilePictureIcon" src="https://images.gr-assets.com/users/1.jpg"/>
<div class="profilePageUserStatsInfo">628 ratings (4.20 avg) 418 reviews</div>
<a class="userShowPageShelfListItem" href="/review/list/1?shelf=read">read&lrm; (627)</a>
<a class="userShowPageShelfListItem" href="/review/list/1?shelf=read-to-my-kids">read-to-my-kids&lrm; (12)</a>
<div>Friends (2,012)</div>
<div class="friends"><a href="/friend/user/9">Bob</a> 5,000 friends</div>
<div id="featured_shelf"><a href="/book/show/10365">Fav</a></div>
<div id="currentlyReadingReviews"><a href="/book/show/241471016">Now</a></div>
</body></html>`
	u, err := ParseUser(mustDoc(t, html), "1", "https://www.goodreads.com/user/show/1")
	if err != nil {
		t.Fatal(err)
	}
	if u.Name != "Otis Chandler" {
		t.Errorf("Name = %q, want Otis Chandler (collapsed)", u.Name)
	}
	if u.Username != "otis" || u.Location != "San Francisco, CA" || u.BooksCount != 1648 {
		t.Errorf("title fields = user %q loc %q books %d", u.Username, u.Location, u.BooksCount)
	}
	if u.RatingsCount != 628 || u.AvgRating != 4.20 || u.ReviewsCount != 418 {
		t.Errorf("stats = ratings %d avg %v reviews %d", u.RatingsCount, u.AvgRating, u.ReviewsCount)
	}
	if u.BooksReadCount != 627 {
		t.Errorf("BooksReadCount = %d, want 627", u.BooksReadCount)
	}
	if u.FriendsCount != 2012 {
		t.Errorf("FriendsCount = %d, want 2012 (header, not list)", u.FriendsCount)
	}
	if len(u.FavoriteBookIDs) != 1 || u.FavoriteBookIDs[0] != "10365" {
		t.Errorf("FavoriteBookIDs = %v", u.FavoriteBookIDs)
	}
	if len(u.CurrentlyReadingBookIDs) != 1 || u.CurrentlyReadingBookIDs[0] != "241471016" {
		t.Errorf("CurrentlyReadingBookIDs = %v", u.CurrentlyReadingBookIDs)
	}
}

// The author page scopes genres to the Genre data row (not the global nav),
// reads hometown from the bare text node by "Born", and reads the aggregate
// rating, ratings, reviews, followers, and distinct-works counts.
func TestParseAuthorDataRows(t *testing.T) {
	const html = `<html><body>
<a href="/genres/nav-junk">NavJunk</a>
<h1 class="authorName">Suzanne Collins</h1>
<div class="dataTitle">Born</div>
  in  Hartford, Connecticut, The United States
  <div class="dataItem" itemprop='birthDate'>August 11, 1962</div>
<div class="dataTitle">Website</div><div class="dataItem"><a itemprop="url" href="http://suzannecollinsbooks.com/">site</a></div>
<div class="dataTitle">Genre</div><div class="dataItem"><a href="/genres/fiction">Fiction</a>, <a href="/genres/young-adult">Young Adult</a></div>
<div class="hreview-aggregate" itemprop="aggregateRating">
  <span class="average" itemprop='ratingValue'>4.3</span>
  <span class="value-title" itemprop='ratingCount' content="21372451">21,372,451</span> ratings
  <span class="value-title" itemprop='reviewCount' content="968271">968,271</span> reviews
  <a href="/author/list/153394">68 distinct works</a>
</div>
<div>Followers (128,487)</div>
</body></html>`
	a, err := ParseAuthor(mustDoc(t, html), "153394", "https://www.goodreads.com/author/show/153394")
	if err != nil {
		t.Fatal(err)
	}
	if a.Hometown != "Hartford, Connecticut, The United States" {
		t.Errorf("Hometown = %q", a.Hometown)
	}
	if a.Website != "http://suzannecollinsbooks.com/" {
		t.Errorf("Website = %q", a.Website)
	}
	if len(a.Genres) != 2 || a.Genres[0] != "Fiction" || a.Genres[1] != "Young Adult" {
		t.Errorf("Genres = %v, want [Fiction, Young Adult] (nav must be excluded)", a.Genres)
	}
	if a.AvgRating != 4.3 || a.RatingsCount != 21372451 || a.ReviewsCount != 968271 {
		t.Errorf("rating = avg %v ratings %d reviews %d", a.AvgRating, a.RatingsCount, a.ReviewsCount)
	}
	if a.BooksCount != 68 || a.FollowersCount != 128487 {
		t.Errorf("counts = books %d followers %d", a.BooksCount, a.FollowersCount)
	}
}

func TestParseListNextPage(t *testing.T) {
	// A list page carries two "next" links: the books pager and the comments
	// thread pager. Only the /list/show/ one must be followed.
	const html = `<html><body>
<div class="pagination">
  <a class="next_page" rel="next" href="/list/show/1.Best_Books_Ever?page=2">next &raquo;</a>
</div>
<div id="comments">
  <a class="next_page" rel="next" href="/list/comments/1.Best_Books_Ever?page=2">next</a>
</div>
</body></html>`
	got := ParseListNextPage(mustDoc(t, html))
	want := "https://www.goodreads.com/list/show/1.Best_Books_Ever?page=2"
	if got != want {
		t.Errorf("ParseListNextPage = %q, want %q", got, want)
	}
}

func TestParseListNextPageLast(t *testing.T) {
	// The final list page has no books pager, only the comments one. We must
	// not mistake the comments link for more books.
	const html = `<html><body>
<div id="comments">
  <a class="next_page" rel="next" href="/list/comments/1.Best_Books_Ever?page=5">next</a>
</div>
</body></html>`
	if got := ParseListNextPage(mustDoc(t, html)); got != "" {
		t.Errorf("ParseListNextPage = %q, want \"\" (comments link is not a books pager)", got)
	}
}

func TestParseQuotesNextPage(t *testing.T) {
	const html = `<html><body>
<a class="next_page" rel="next" href="/author/quotes/153394.Suzanne_Collins?page=2">next &raquo;</a>
</body></html>`
	got := ParseQuotesNextPage(mustDoc(t, html))
	want := "https://www.goodreads.com/author/quotes/153394.Suzanne_Collins?page=2"
	if got != want {
		t.Errorf("ParseQuotesNextPage = %q, want %q", got, want)
	}
	if got := ParseQuotesNextPage(mustDoc(t, "<html><body></body></html>")); got != "" {
		t.Errorf("ParseQuotesNextPage on last page = %q, want \"\"", got)
	}
}
