package goodread

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// ── Book ──────────────────────────────────────────────────────────────────────

type bookJSONLD struct {
	Type            string          `json:"@type"`
	Name            string          `json:"name"`
	Author          json.RawMessage `json:"author"` // object or array
	Image           string          `json:"image"`
	Description     string          `json:"description"`
	ISBN            string          `json:"isbn"`
	NumberOfPages   int             `json:"numberOfPages"`
	InLanguage      string          `json:"inLanguage"`
	URL             string          `json:"url"`
	AggregateRating struct {
		RatingValue json.RawMessage `json:"ratingValue"`
		RatingCount json.RawMessage `json:"ratingCount"`
		ReviewCount json.RawMessage `json:"reviewCount"`
	} `json:"aggregateRating"`
	Publisher struct {
		Name string `json:"name"`
	} `json:"publisher"`
	DatePublished string `json:"datePublished"`
	BookFormat    string `json:"bookFormat"`
	ISBN13        string `json:"isbn13"`
	ASIN          string `json:"asin"`
}

type authorObj struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

func parseAuthorField(raw json.RawMessage) authorObj {
	if len(raw) == 0 {
		return authorObj{}
	}
	var arr []authorObj
	if json.Unmarshal(raw, &arr) == nil && len(arr) > 0 {
		return arr[0]
	}
	var obj authorObj
	_ = json.Unmarshal(raw, &obj)
	return obj
}

// ParseBook parses a Goodreads book page into a Book (JSON-LD first).
func ParseBook(doc *goquery.Document, bookID, pageURL string) (*Book, error) {
	b := &Book{BookID: bookID, URL: pageURL, FetchedAt: time.Now()}

	doc.Find(`script[type="application/ld+json"]`).Each(func(_ int, sel *goquery.Selection) {
		if b.Title != "" {
			return
		}
		var ld bookJSONLD
		if err := json.Unmarshal([]byte(sel.Text()), &ld); err != nil || ld.Type != "Book" {
			return
		}
		b.Title = ld.Name
		b.CoverURL = ld.Image
		b.Description = cleanHTML(ld.Description)
		b.ISBN, b.ISBN13, b.ASIN = ld.ISBN, ld.ISBN13, ld.ASIN
		b.Pages, b.Language, b.Format = ld.NumberOfPages, ld.InLanguage, ld.BookFormat
		a := parseAuthorField(ld.Author)
		b.AuthorName = a.Name
		if a.URL != "" {
			b.AuthorID = extractIDFromPath(a.URL, "/author/show/")
		}
		b.Publisher = ld.Publisher.Name
		if len(ld.DatePublished) >= 4 {
			if y, err := strconv.Atoi(ld.DatePublished[:4]); err == nil {
				b.PublishedYear = y
			}
		}
		b.AvgRating = parseRawFloat(ld.AggregateRating.RatingValue)
		b.RatingsCount = parseRawInt64(ld.AggregateRating.RatingCount)
		b.ReviewsCount = parseRawInt64(ld.AggregateRating.ReviewCount)
	})

	if b.Title == "" {
		b.Title = strings.TrimSpace(doc.Find("h1[data-testid='bookTitle'], h1.Text__title1").First().Text())
	}
	if b.Title == "" {
		b.Title = strings.TrimSpace(doc.Find("h1").First().Text())
	}
	b.TitleWithoutSeries = b.Title
	if idx := strings.Index(b.Title, "("); idx > 0 {
		b.TitleWithoutSeries = strings.TrimSpace(b.Title[:idx])
	}
	if b.AuthorName == "" {
		b.AuthorName = strings.TrimSpace(doc.Find("[data-testid='name']").First().Text())
	}
	if b.AuthorID == "" {
		doc.Find("a[href*='/author/show/']").EachWithBreak(func(_ int, sel *goquery.Selection) bool {
			href, _ := sel.Attr("href")
			if id := extractIDFromPath(href, "/author/show/"); id != "" {
				b.AuthorID = id
				return false
			}
			return true
		})
	}
	if b.CoverURL == "" {
		b.CoverURL, _ = doc.Find("img.ResponsiveImage").First().Attr("src")
	}
	doc.Find("a[href*='/genres/']").Each(func(_ int, sel *goquery.Selection) {
		if g := strings.TrimSpace(sel.Text()); g != "" && !contains(b.Genres, g) {
			b.Genres = append(b.Genres, g)
		}
	})
	doc.Find("a[href*='/series/']").EachWithBreak(func(_ int, sel *goquery.Selection) bool {
		href, _ := sel.Attr("href")
		if id := extractIDFromPath(href, "/series/"); id != "" {
			b.SeriesID = id
			b.SeriesName = strings.TrimSpace(sel.Text())
			return false
		}
		return true
	})
	if i := strings.Index(b.Title, "#"); i > 0 {
		if end := strings.Index(b.Title[i:], ")"); end > 0 {
			b.SeriesPosition = strings.TrimSpace(b.Title[i+1 : i+end])
		}
	}
	doc.Find("a[href*='/book/show/']").Each(func(_ int, sel *goquery.Selection) {
		href, _ := sel.Attr("href")
		if id := extractIDFromPath(href, "/book/show/"); id != "" && id != bookID &&
			!contains(b.SimilarBookIDs, id) && len(b.SimilarBookIDs) < 20 {
			b.SimilarBookIDs = append(b.SimilarBookIDs, id)
		}
	})
	return b, nil
}

// ── Author ────────────────────────────────────────────────────────────────────

type authorJSONLD struct {
	Type        string `json:"@type"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Image       struct {
		URL string `json:"url"`
	} `json:"image"`
	URL       string `json:"url"`
	SameAs    string `json:"sameAs"`
	BirthDate string `json:"birthDate"`
	DeathDate string `json:"deathDate"`
}

// ParseAuthor parses a Goodreads author page.
func ParseAuthor(doc *goquery.Document, authorID, pageURL string) (*Author, error) {
	a := &Author{AuthorID: authorID, URL: pageURL, FetchedAt: time.Now()}

	doc.Find(`script[type="application/ld+json"]`).Each(func(_ int, sel *goquery.Selection) {
		if a.Name != "" {
			return
		}
		var ld authorJSONLD
		if err := json.Unmarshal([]byte(sel.Text()), &ld); err != nil || ld.Type != "Person" {
			return
		}
		a.Name = ld.Name
		a.Bio = cleanHTML(ld.Description)
		a.PhotoURL = ld.Image.URL
		a.Website = ld.SameAs
		a.BornDate = ld.BirthDate
		a.DiedDate = ld.DeathDate
	})

	if a.Name == "" {
		a.Name = strings.TrimSpace(doc.Find("h1.authorName, [data-testid='name']").First().Text())
	}
	if a.Name == "" {
		a.Name = strings.TrimSpace(doc.Find("h1").First().Text())
	}
	if a.Bio == "" {
		a.Bio = cleanBio(doc.Find("[data-testid='description'], .authorShortBio, div.aboutAuthorInfo").First().Text())
	}
	if a.PhotoURL == "" {
		a.PhotoURL, _ = doc.Find("img.authorPhoto, img[itemprop='image']").First().Attr("src")
	}
	if a.Website == "" {
		doc.Find("a[href^='http']").EachWithBreak(func(_ int, sel *goquery.Selection) bool {
			href, _ := sel.Attr("href")
			if !strings.Contains(href, "goodreads.com") {
				a.Website = href
				return false
			}
			return true
		})
	}
	if h := strings.TrimSpace(doc.Find("[class*='hometown'], [data-testid='hometown']").First().Text()); h != "" {
		a.Hometown = h
	}
	doc.Find("a[href*='/genres/']").Each(func(_ int, sel *goquery.Selection) {
		if g := strings.TrimSpace(sel.Text()); g != "" && !contains(a.Genres, g) {
			a.Genres = append(a.Genres, g)
		}
	})
	doc.Find("a[href*='/author/show/']").Each(func(_ int, sel *goquery.Selection) {
		if t := strings.TrimSpace(sel.Text()); t != "" && t != a.Name && !contains(a.Influences, t) && len(a.Influences) < 10 {
			a.Influences = append(a.Influences, t)
		}
	})
	doc.Find("a[href*='/book/show/']").Each(func(_ int, sel *goquery.Selection) {
		href, _ := sel.Attr("href")
		if id := extractIDFromPath(href, "/book/show/"); id != "" && !contains(a.NotableBookIDs, id) && len(a.NotableBookIDs) < 30 {
			a.NotableBookIDs = append(a.NotableBookIDs, id)
		}
	})
	a.BooksCount = len(a.NotableBookIDs)
	return a, nil
}

// ── Series ────────────────────────────────────────────────────────────────────

var reBookShow = regexp.MustCompile(`/book/show/(\d+)`)

// ParseSeries parses a Goodreads series page.
func ParseSeries(doc *goquery.Document, seriesID, pageURL string) (*Series, []SeriesBook, error) {
	s := &Series{SeriesID: seriesID, URL: pageURL, FetchedAt: time.Now()}
	s.Name = strings.TrimSuffix(strings.TrimSpace(doc.Find("h1.gr-h1--serif, [data-testid='seriesTitle'], h1").First().Text()), " Series")
	s.Description = cleanHTML(strings.TrimSpace(doc.Find("[data-testid='description'], .seriesDesc").First().Text()))

	doc.Find("div, span, p").EachWithBreak(func(_ int, sel *goquery.Selection) bool {
		text := strings.TrimSpace(sel.Text())
		if !strings.Contains(text, "primary work") {
			return true
		}
		parts := strings.Fields(text)
		for i, p := range parts {
			if strings.Contains(p, "primary") && i > 0 {
				s.PrimaryWorkCount = atoiClean(parts[i-1])
			}
			if strings.Contains(p, "total") && i > 0 {
				s.TotalBooks = atoiClean(parts[i-1])
			}
		}
		return false
	})

	var books []SeriesBook
	seen := map[string]bool{}
	pos := 1
	doc.Find("a[href*='/book/show/']").Each(func(_ int, sel *goquery.Selection) {
		href, _ := sel.Attr("href")
		if m := reBookShow.FindStringSubmatch(href); len(m) > 1 && !seen[m[1]] {
			seen[m[1]] = true
			books = append(books, SeriesBook{SeriesID: seriesID, BookID: m[1], Position: pos})
			s.BookIDs = append(s.BookIDs, m[1])
			pos++
		}
	})
	if s.TotalBooks == 0 {
		s.TotalBooks = len(books)
	}
	return s, books, nil
}

// ── List ──────────────────────────────────────────────────────────────────────

// ParseList parses a Listopia list page.
func ParseList(doc *goquery.Document, listID, pageURL string) (*List, []ListBook, error) {
	l := &List{ListID: listID, URL: pageURL, FetchedAt: time.Now()}
	l.Name = strings.TrimSpace(doc.Find("h1.listTitle, h1[class*='listTitle']").First().Text())
	if l.Name == "" {
		l.Name = strings.TrimSpace(doc.Find("h1").First().Text())
	}
	l.Description = cleanHTML(strings.TrimSpace(doc.Find(".listDescription, [data-testid='description']").First().Text()))
	if t := strings.TrimSpace(doc.Find("a[href*='/user/show/'], a[href*='/profile/']").First().Text()); t != "" {
		l.CreatedByUser = t
	}
	doc.Find("[class*='smallText'], .smallText").Each(func(_ int, sel *goquery.Selection) {
		text := strings.TrimSpace(sel.Text())
		if strings.Contains(text, "voter") {
			if n := atoiClean(strings.Fields(text)[0]); n > 0 {
				l.VotersCount = n
			}
		}
	})
	doc.Find("a[href*='/list/tag/']").Each(func(_ int, sel *goquery.Selection) {
		if tag := strings.TrimSpace(sel.Text()); tag != "" && !contains(l.Tags, tag) {
			l.Tags = append(l.Tags, tag)
		}
	})

	var books []ListBook
	rank := 1
	doc.Find("tr.bookContainer, tr[itemtype*='Book'], .listWithDividers__item").Each(func(_ int, row *goquery.Selection) {
		var bookID string
		row.Find("a[href*='/book/show/']").EachWithBreak(func(_ int, a *goquery.Selection) bool {
			href, _ := a.Attr("href")
			if m := reBookShow.FindStringSubmatch(href); len(m) > 1 {
				bookID = m[1]
				return false
			}
			return true
		})
		if bookID == "" {
			return
		}
		lb := ListBook{ListID: listID, BookID: bookID, Rank: rank}
		row.Find("[class*='smallText']").Each(func(_ int, sel *goquery.Selection) {
			text := strings.TrimSpace(sel.Text())
			if strings.Contains(text, "vote") {
				lb.Votes = atoiClean(strings.Fields(text)[0])
			}
		})
		books = append(books, lb)
		rank++
	})
	if len(books) == 0 {
		seen := map[string]bool{}
		doc.Find("a[href*='/book/show/']").Each(func(_ int, sel *goquery.Selection) {
			href, _ := sel.Attr("href")
			if m := reBookShow.FindStringSubmatch(href); len(m) > 1 && !seen[m[1]] {
				seen[m[1]] = true
				books = append(books, ListBook{ListID: listID, BookID: m[1], Rank: rank})
				rank++
			}
		})
	}
	l.BooksCount = len(books)
	return l, books, nil
}

// ── Genre ─────────────────────────────────────────────────────────────────────

var reBooksCount = regexp.MustCompile(`([\d,]+)\s+book`)

// ParseGenre parses a Goodreads genre page.
func ParseGenre(doc *goquery.Document, slug, pageURL string) (*Genre, error) {
	g := &Genre{Slug: slug, URL: pageURL, FetchedAt: time.Now()}
	g.Name = strings.TrimSpace(doc.Find("h1, [data-testid='genreTitle']").First().Text())
	if g.Name == "" {
		g.Name = strings.ReplaceAll(slug, "-", " ")
	}
	g.Description = cleanHTML(strings.TrimSpace(doc.Find("[data-testid='description'], .genreDescription, .mediumText").First().Text()))
	doc.Find("div, span, p").EachWithBreak(func(_ int, sel *goquery.Selection) bool {
		if m := reBooksCount.FindStringSubmatch(strings.TrimSpace(sel.Text())); len(m) > 1 {
			if n := atoiClean(m[1]); n > 0 {
				g.BooksCount = n
				return false
			}
		}
		return true
	})
	seen := map[string]bool{}
	doc.Find("a[href*='/book/show/']").Each(func(_ int, sel *goquery.Selection) {
		href, _ := sel.Attr("href")
		if id := extractIDFromPath(href, "/book/show/"); id != "" && !seen[id] {
			seen[id] = true
			g.BookIDs = append(g.BookIDs, id)
		}
	})
	if g.BooksCount == 0 {
		g.BooksCount = len(g.BookIDs)
	}
	return g, nil
}

// ── Quote ─────────────────────────────────────────────────────────────────────

var (
	reQuoteID    = regexp.MustCompile(`/quotes/(\d+)`)
	reQuoteLikes = regexp.MustCompile(`(\d[\d,]*)\s+like`)
)

// ParseQuotes parses one or more quotes from a quotes page.
func ParseQuotes(doc *goquery.Document, pageURL string) ([]Quote, error) {
	var quotes []Quote
	doc.Find(".quoteText, [class*='quoteText'], [data-testid='quoteText']").Each(func(_ int, sel *goquery.Selection) {
		q := Quote{URL: pageURL, FetchedAt: time.Now()}
		q.Text = strings.TrimSpace(sel.Clone().Children().Remove().End().Text())
		if q.Text == "" {
			q.Text = strings.TrimSpace(sel.Text())
		}
		q.Text = cleanQuote(q.Text)
		if q.Text == "" {
			return
		}
		parent := sel.Parent()
		parent.Find("a[href*='/author/show/']").First().Each(func(_ int, a *goquery.Selection) {
			href, _ := a.Attr("href")
			q.AuthorID = extractIDFromPath(href, "/author/show/")
			q.AuthorName = strings.TrimSpace(a.Text())
		})
		parent.Find("a[href*='/book/show/']").First().Each(func(_ int, a *goquery.Selection) {
			href, _ := a.Attr("href")
			q.BookID = extractIDFromPath(href, "/book/show/")
			if q.BookTitle == "" {
				q.BookTitle = strings.TrimSpace(a.Text())
			}
		})
		parent.Find("a[href*='/quotes/']").EachWithBreak(func(_ int, a *goquery.Selection) bool {
			href, _ := a.Attr("href")
			if m := reQuoteID.FindStringSubmatch(href); len(m) > 1 {
				q.QuoteID = m[1]
				q.URL = BaseURL + href
				return false
			}
			return true
		})
		parent.Find("[class*='likes']").Each(func(_ int, s *goquery.Selection) {
			if m := reQuoteLikes.FindStringSubmatch(strings.TrimSpace(s.Text())); len(m) > 1 {
				q.LikesCount = atoiClean(m[1])
			}
		})
		parent.Find("a[href*='/quotes/tag/'], a[href*='/quotes?tag=']").Each(func(_ int, a *goquery.Selection) {
			if tag := strings.TrimSpace(a.Text()); tag != "" && !contains(q.Tags, tag) {
				q.Tags = append(q.Tags, tag)
			}
		})
		if q.QuoteID == "" {
			q.QuoteID = "q" + hashStr(q.Text)
		}
		quotes = append(quotes, q)
	})
	return quotes, nil
}

func hashStr(s string) string {
	h := uint64(14695981039346656037)
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return strconv.FormatUint(h, 16)
}

// ── User ──────────────────────────────────────────────────────────────────────

var reDate = regexp.MustCompile(`\b(Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)\s+\d{4}\b`)

// ParseUser parses a Goodreads user profile.
func ParseUser(doc *goquery.Document, userID, pageURL string) (*User, error) {
	u := &User{UserID: userID, URL: pageURL, FetchedAt: time.Now()}
	u.Name = strings.TrimSpace(doc.Find("h1.userProfileName, [data-testid='name']").First().Text())
	if u.Name == "" {
		u.Name = strings.TrimSpace(doc.Find("h1").First().Text())
	}
	u.AvatarURL, _ = doc.Find("img.userPhoto, img[itemprop='image']").First().Attr("src")
	u.Bio = cleanHTML(strings.TrimSpace(doc.Find("[data-testid='aboutMe'], .aboutAuthorInfo, #aboutAuthor").First().Text()))
	u.Location = strings.TrimSpace(doc.Find("[data-testid='userLocation'], .userLocation").First().Text())
	doc.Find("a[href^='http']").EachWithBreak(func(_ int, sel *goquery.Selection) bool {
		href, _ := sel.Attr("href")
		if !strings.Contains(href, "goodreads.com") {
			u.Website = href
			return false
		}
		return true
	})
	doc.Find("[class*='joined'], .memberSince").Each(func(_ int, sel *goquery.Selection) {
		if m := reDate.FindString(strings.TrimSpace(sel.Text())); m != "" {
			u.JoinedDate, _ = time.Parse("Jan 2006", m)
		}
	})
	doc.Find("[class*='statsCount'], .statsPanelCount, [data-testid='statsCount']").Each(func(_ int, sel *goquery.Selection) {
		n := atoiClean(strings.TrimSpace(sel.Text()))
		parent := strings.ToLower(strings.TrimSpace(sel.Parent().Text()))
		switch {
		case strings.Contains(parent, "friend"):
			u.FriendsCount = n
		case strings.Contains(parent, "book"):
			u.BooksReadCount = n
		case strings.Contains(parent, "rating"):
			u.RatingsCount = n
		case strings.Contains(parent, "review"):
			u.ReviewsCount = n
		}
	})
	doc.Find(".favoriteBooks a[href*='/book/show/']").Each(func(_ int, sel *goquery.Selection) {
		href, _ := sel.Attr("href")
		if id := extractIDFromPath(href, "/book/show/"); id != "" && !contains(u.FavoriteBookIDs, id) {
			u.FavoriteBookIDs = append(u.FavoriteBookIDs, id)
		}
	})
	return u, nil
}

// ── Shelf ─────────────────────────────────────────────────────────────────────

var reDateRead = regexp.MustCompile(`(\d{4})`)

// ParseShelf parses a user's shelf page into ShelfBook rows.
func ParseShelf(doc *goquery.Document, userID, shelfName, pageURL string) (*Shelf, []ShelfBook, error) {
	shelfID := userID + "/" + shelfName
	s := &Shelf{ShelfID: shelfID, UserID: userID, Name: shelfName, URL: pageURL, FetchedAt: time.Now()}
	doc.Find("[class*='headerCount'], .headerBooksCount").Each(func(_ int, sel *goquery.Selection) {
		if n := atoiClean(sel.Text()); n > 0 {
			s.BooksCount = n
		}
	})

	var books []ShelfBook
	doc.Find("tr.bookalike, tr[id^='review_']").Each(func(_ int, row *goquery.Selection) {
		var bookID, title string
		row.Find("a[href*='/book/show/']").EachWithBreak(func(_ int, a *goquery.Selection) bool {
			href, _ := a.Attr("href")
			if m := reBookShow.FindStringSubmatch(href); len(m) > 1 {
				bookID = m[1]
				title = strings.TrimSpace(a.Text())
				return false
			}
			return true
		})
		if bookID == "" {
			return
		}
		sb := ShelfBook{ShelfID: shelfID, UserID: userID, BookID: bookID, Title: title}
		sb.Rating = row.Find("[class*='staticStar'][class*='p10']").Length()
		row.Find("[class*='date_added'] .date_added_value, td.field.date_added .value").Each(func(_ int, sel *goquery.Selection) {
			if t, err := time.Parse("Jan 02, 2006", strings.TrimSpace(sel.Text())); err == nil {
				sb.DateAdded = t
			}
		})
		row.Find("[class*='date_read'] .date_read_value, td.field.date_read .value").Each(func(_ int, sel *goquery.Selection) {
			text := strings.TrimSpace(sel.Text())
			if t, err := time.Parse("Jan 02, 2006", text); err == nil {
				sb.DateRead = t
			} else if m := reDateRead.FindString(text); m != "" {
				if t, err := time.Parse("2006", m); err == nil {
					sb.DateRead = t
				}
			}
		})
		books = append(books, sb)
	})
	if s.BooksCount == 0 {
		s.BooksCount = len(books)
	}
	return s, books, nil
}

// ParseShelfNextPage returns the next shelf page URL, or "".
func ParseShelfNextPage(doc *goquery.Document) string {
	var next string
	doc.Find("a[href*='page=']").EachWithBreak(func(_ int, sel *goquery.Selection) bool {
		text := strings.ToLower(strings.TrimSpace(sel.Text()))
		if text == "next" || text == "next »" || text == "next page" {
			if href, _ := sel.Attr("href"); href != "" {
				next = absURL(href)
				return false
			}
		}
		return true
	})
	return next
}

// ── Review ────────────────────────────────────────────────────────────────────

var reReviewID = regexp.MustCompile(`/review/show/(\d+)`)

type reviewJSONLD struct {
	Type         string `json:"@type"`
	ReviewRating struct {
		RatingValue string `json:"ratingValue"`
	} `json:"reviewRating"`
	Author struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	} `json:"author"`
	ReviewBody  string `json:"reviewBody"`
	DateCreated string `json:"dateCreated"`
	URL         string `json:"url"`
}

// ParseReviews extracts embedded reviews from a book page.
func ParseReviews(doc *goquery.Document, bookID string) []Review {
	var reviews []Review
	doc.Find(`script[type="application/ld+json"]`).Each(func(_ int, sel *goquery.Selection) {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal([]byte(sel.Text()), &raw); err != nil {
			return
		}
		reviewsRaw, ok := raw["review"]
		if !ok {
			return
		}
		var jr []reviewJSONLD
		if err := json.Unmarshal(reviewsRaw, &jr); err != nil {
			var single reviewJSONLD
			if json.Unmarshal(reviewsRaw, &single) == nil {
				jr = []reviewJSONLD{single}
			}
		}
		for _, r := range jr {
			rv := Review{BookID: bookID, Text: cleanHTML(r.ReviewBody), FetchedAt: time.Now()}
			rv.Rating, _ = strconv.Atoi(r.ReviewRating.RatingValue)
			rv.UserName = r.Author.Name
			rv.UserID = extractIDFromPath(r.Author.URL, "/user/show/")
			rv.URL = r.URL
			if m := reReviewID.FindStringSubmatch(rv.URL); len(m) > 1 {
				rv.ReviewID = m[1]
			}
			if r.DateCreated != "" {
				rv.DateAdded, _ = time.Parse("2006-01-02", r.DateCreated)
			}
			if rv.ReviewID != "" {
				reviews = append(reviews, rv)
			}
		}
	})
	if len(reviews) == 0 {
		doc.Find("[data-testid='review'], .ReviewCard, .review").Each(func(_ int, sel *goquery.Selection) {
			rv := Review{BookID: bookID, FetchedAt: time.Now()}
			sel.Find("a[href*='/review/show/']").EachWithBreak(func(_ int, a *goquery.Selection) bool {
				href, _ := a.Attr("href")
				if m := reReviewID.FindStringSubmatch(href); len(m) > 1 {
					rv.ReviewID = m[1]
					rv.URL = BaseURL + href
					return false
				}
				return true
			})
			rv.UserName = strings.TrimSpace(sel.Find("[data-testid='name'], .ReviewerProfile__name").First().Text())
			sel.Find("a[href*='/user/show/']").EachWithBreak(func(_ int, a *goquery.Selection) bool {
				href, _ := a.Attr("href")
				rv.UserID = extractIDFromPath(href, "/user/show/")
				return rv.UserID == ""
			})
			sel.Find("[aria-label]").EachWithBreak(func(_ int, s *goquery.Selection) bool {
				label, _ := s.Attr("aria-label")
				if strings.Contains(label, "star") {
					rv.Rating = atoiClean(strings.Fields(label + " ")[0])
					return false
				}
				return true
			})
			rv.Text = cleanHTML(strings.TrimSpace(sel.Find("[data-testid='reviewText'], .ReviewText__content, .reviewText").Text()))
			if rv.ReviewID != "" && rv.Text != "" {
				reviews = append(reviews, rv)
			}
		})
	}
	return reviews
}

// ── Search ────────────────────────────────────────────────────────────────────

// ParseSearchAutocomplete parses the JSON autocomplete response.
func ParseSearchAutocomplete(body []byte) []SearchResult {
	var items []struct {
		BookID  string `json:"bookId"`
		BookURL string `json:"bookUrl"`
		Title   string `json:"title"`
		Author  struct {
			Name       string `json:"name"`
			ProfileURL string `json:"profileUrl"`
		} `json:"author"`
	}
	if err := json.Unmarshal(body, &items); err != nil {
		return nil
	}
	seen := map[string]bool{}
	var out []SearchResult
	for _, it := range items {
		if it.BookURL != "" && !seen[it.BookURL] {
			seen[it.BookURL] = true
			out = append(out, SearchResult{URL: absURL(it.BookURL), EntityType: "book", Title: it.Title})
		}
		if it.Author.ProfileURL != "" && !seen[it.Author.ProfileURL] {
			seen[it.Author.ProfileURL] = true
			out = append(out, SearchResult{URL: absURL(it.Author.ProfileURL), EntityType: "author", Title: it.Author.Name})
		}
	}
	return out
}

// ParseAutocompleteBooks parses the rich JSON autocomplete response into Book
// records. The autocomplete endpoint is open (not WAF-challenged), so it is the
// primary way to obtain structured book data without a lent session.
func ParseAutocompleteBooks(body []byte) []Book {
	var items []struct {
		ImageURL     string `json:"imageUrl"`
		BookID       string `json:"bookId"`
		BookURL      string `json:"bookUrl"`
		Title        string `json:"title"`
		BookTitle    string `json:"bookTitleBare"`
		NumPages     int    `json:"numPages"`
		AvgRating    string `json:"avgRating"`
		RatingsCount int64  `json:"ratingsCount"`
		Author       struct {
			ID         int64  `json:"id"`
			Name       string `json:"name"`
			ProfileURL string `json:"profileUrl"`
		} `json:"author"`
		Description struct {
			HTML string `json:"html"`
		} `json:"description"`
	}
	if err := json.Unmarshal(body, &items); err != nil {
		return nil
	}
	out := make([]Book, 0, len(items))
	for _, it := range items {
		b := Book{
			BookID:             it.BookID,
			Title:              it.Title,
			TitleWithoutSeries: it.BookTitle,
			Description:        cleanHTML(it.Description.HTML),
			AuthorName:         it.Author.Name,
			Pages:              it.NumPages,
			AvgRating:          parseFloat(it.AvgRating),
			RatingsCount:       it.RatingsCount,
			CoverURL:           it.ImageURL,
			URL:                absURL(it.BookURL),
			FetchedAt:          time.Now(),
		}
		if it.Author.ID > 0 {
			b.AuthorID = strconv.FormatInt(it.Author.ID, 10)
		}
		// "Title (Series, #N)" -> series name + position.
		if open := strings.LastIndex(it.Title, "("); open > 0 && strings.HasSuffix(strings.TrimSpace(it.Title), ")") {
			inner := strings.TrimSuffix(strings.TrimSpace(it.Title[open+1:]), ")")
			if hash := strings.LastIndex(inner, "#"); hash >= 0 {
				b.SeriesName = strings.TrimRight(strings.TrimSpace(inner[:hash]), ",")
				b.SeriesPosition = strings.TrimSpace(inner[hash+1:])
			}
		}
		out = append(out, b)
	}
	return out
}

// ParseSearchHTML parses book/author result rows from a full-search page.
func ParseSearchHTML(doc *goquery.Document) []SearchResult {
	seen := map[string]bool{}
	var out []SearchResult
	doc.Find(`tr[itemtype="http://schema.org/Book"]`).Each(func(_ int, s *goquery.Selection) {
		a := s.Find("a.bookTitle")
		href, _ := a.Attr("href")
		if href != "" && !seen[href] {
			seen[href] = true
			out = append(out, SearchResult{URL: absURL(href), EntityType: "book", Title: strings.TrimSpace(a.Text())})
		}
		au := s.Find("a.authorName")
		ahref, _ := au.Attr("href")
		if ahref != "" && !seen[ahref] {
			seen[ahref] = true
			out = append(out, SearchResult{URL: absURL(ahref), EntityType: "author", Title: strings.TrimSpace(au.Text())})
		}
	})
	return out
}

// ParseSearchHTMLNextPage returns the next search page URL, or "".
func ParseSearchHTMLNextPage(doc *goquery.Document) string {
	href, ok := doc.Find("a.next_page").Attr("href")
	if !ok || href == "" {
		return ""
	}
	return absURL(href)
}

func absURL(href string) string {
	if strings.HasPrefix(href, "/") {
		return BaseURL + href
	}
	return href
}
