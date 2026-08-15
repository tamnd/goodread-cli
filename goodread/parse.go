package goodread

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
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
func ParseBook(doc *goquery.Document, bookID, pageURL string) (*ScrapedBook, error) {
	b := &ScrapedBook{BookID: bookID, URL: pageURL, FetchedAt: time.Now()}

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
	b.CoverURL = fullCoverURL(b.CoverURL)
	if b.WorkID == "" {
		doc.Find("a[href*='/work/']").EachWithBreak(func(_ int, sel *goquery.Selection) bool {
			href, _ := sel.Attr("href")
			if m := reWorkID.FindStringSubmatch(href); len(m) > 1 {
				b.WorkID = m[1]
				return false
			}
			return true
		})
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
func ParseAuthor(doc *goquery.Document, authorID, pageURL string) (*ScrapedAuthor, error) {
	a := &ScrapedAuthor{AuthorID: authorID, URL: pageURL, FetchedAt: time.Now()}

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
		a.Name = metaContent(doc, "og:title")
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
	if a.PhotoURL == "" {
		a.PhotoURL = metaContent(doc, "og:image")
	}

	// Author data rows ("Born", "Website", "Genre", "Influences") carry the
	// structured facts. Scope each field to its own row so the global genre
	// nav and similar-author links do not leak in.
	doc.Find("div.dataTitle").Each(func(_ int, t *goquery.Selection) {
		item := t.NextFilteredUntil("div.dataItem", "div.dataTitle").First()
		switch strings.TrimSpace(t.Text()) {
		case "Born":
			a.BornDate = strings.TrimSpace(item.Text())
			// The hometown is the bare text node(s) sitting between the "Born"
			// title and the birthDate item ("in Hartford, Connecticut, ...").
			var parts []string
			for n := t.Get(0).NextSibling; n != nil; n = n.NextSibling {
				if n.Type == html.ElementNode {
					break
				}
				if n.Type == html.TextNode {
					if s := strings.TrimSpace(n.Data); s != "" {
						parts = append(parts, s)
					}
				}
			}
			if h := strings.TrimSpace(strings.TrimPrefix(cleanText(strings.Join(parts, " ")), "in ")); h != "" {
				a.Hometown = h
			}
		case "Died":
			a.DiedDate = strings.TrimSpace(item.Text())
		case "Website":
			if href, ok := item.Find("a").First().Attr("href"); ok {
				a.Website = href
			}
		case "Genre":
			item.Find("a[href*='/genres/']").Each(func(_ int, sel *goquery.Selection) {
				if g := strings.TrimSpace(sel.Text()); g != "" && !contains(a.Genres, g) {
					a.Genres = append(a.Genres, g)
				}
			})
		case "Influences":
			item.Find("a[href*='/author/show/']").Each(func(_ int, sel *goquery.Selection) {
				if t := strings.TrimSpace(sel.Text()); t != "" && !contains(a.Influences, t) {
					a.Influences = append(a.Influences, t)
				}
			})
		}
	})
	if a.Website == "" {
		a.Website = metaContent(doc, "og:url")
	}

	a.AvgRating = parseFloat(doc.Find("span[itemprop='ratingValue']").First().Text())
	a.RatingsCount = int64(atoiClean(doc.Find("[itemprop='ratingCount']").First().AttrOr("content", "")))
	a.ReviewsCount = int64(atoiClean(doc.Find("[itemprop='reviewCount']").First().AttrOr("content", "")))
	if m := reFollowers.FindStringSubmatch(doc.Text()); len(m) > 1 {
		a.FollowersCount = atoiClean(m[1])
	}
	if m := reDistinctWorks.FindStringSubmatch(doc.Text()); len(m) > 1 {
		a.BooksCount = atoiClean(m[1])
	}

	doc.Find("a[href*='/book/show/']").Each(func(_ int, sel *goquery.Selection) {
		href, _ := sel.Attr("href")
		if id := extractIDFromPath(href, "/book/show/"); id != "" && !contains(a.NotableBookIDs, id) && len(a.NotableBookIDs) < 30 {
			a.NotableBookIDs = append(a.NotableBookIDs, id)
		}
	})
	if a.BooksCount == 0 {
		a.BooksCount = len(a.NotableBookIDs)
	}
	return a, nil
}

var (
	reFollowers     = regexp.MustCompile(`Followers \(([\d,]+)\)`)
	reDistinctWorks = regexp.MustCompile(`([\d,]+)\s+distinct works?`)
)

// metaContent returns the content of <meta property="..."> or name="...".
func metaContent(doc *goquery.Document, key string) string {
	v, _ := doc.Find(`meta[property="` + key + `"], meta[name="` + key + `"]`).First().Attr("content")
	return strings.TrimSpace(v)
}

// ── Series ────────────────────────────────────────────────────────────────────

var (
	reBookShow     = regexp.MustCompile(`/book/show/(\d+)`)
	reWorkID       = regexp.MustCompile(`/work/(?:quotes/|editions/)?(\d+)`)
	reSeriesCounts = regexp.MustCompile(`([\d,]+)\s+primary works?\s*[•·]\s*([\d,]+)\s+total works?`)
)

type seriesHeaderProps struct {
	Title       string `json:"title"`
	Subtitle    string `json:"subtitle"`
	Description struct {
		HTML string `json:"html"`
	} `json:"description"`
}

type seriesListProps struct {
	SeriesHeaders []string `json:"seriesHeaders"`
	Series        []struct {
		ScrapedBook rawBook `json:"book"`
	} `json:"series"`
}

// ParseSeries parses a Goodreads series page. The page is a legacy React view
// whose data lives in data-react-props attributes (SeriesHeader for the title,
// subtitle, and description; one or more SeriesList blocks for the books), with
// a visible-text fallback for the rare page that lacks them.
func ParseSeries(doc *goquery.Document, seriesID, pageURL string) (*ScrapedSeries, []SeriesBook, error) {
	s := &ScrapedSeries{SeriesID: seriesID, URL: pageURL, FetchedAt: time.Now()}

	if props, ok := reactProps(doc, "ReactComponents.SeriesHeader"); ok {
		var h seriesHeaderProps
		if json.Unmarshal(props, &h) == nil {
			s.Name = strings.TrimSuffix(strings.TrimSpace(h.Title), " Series")
			s.Description = cleanText(h.Description.HTML)
			if m := reSeriesCounts.FindStringSubmatch(h.Subtitle); len(m) > 2 {
				s.PrimaryWorkCount = atoiClean(m[1])
				s.TotalBooks = atoiClean(m[2])
			}
		}
	}

	var books []SeriesBook
	var firstAuthorID, firstAuthorName string
	seen := map[string]bool{}
	addBook := func(id, label string) {
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		books = append(books, SeriesBook{SeriesID: seriesID, BookID: id, Position: len(books) + 1, PositionLabel: label})
		s.BookIDs = append(s.BookIDs, id)
	}
	doc.Find(`[data-react-class="ReactComponents.SeriesList"]`).Each(func(_ int, sel *goquery.Selection) {
		raw, ok := sel.Attr("data-react-props")
		if !ok {
			return
		}
		var lp seriesListProps
		if json.Unmarshal([]byte(raw), &lp) != nil {
			return
		}
		for i, entry := range lp.Series {
			label := ""
			if i < len(lp.SeriesHeaders) {
				label = lp.SeriesHeaders[i]
			}
			addBook(entry.ScrapedBook.BookID, label)
			if firstAuthorName == "" {
				firstAuthorName = entry.ScrapedBook.Author.Name
				if entry.ScrapedBook.Author.ID > 0 {
					firstAuthorID = strconv.FormatInt(entry.ScrapedBook.Author.ID, 10)
				}
			}
		}
	})
	s.AuthorID, s.AuthorName = firstAuthorID, firstAuthorName

	// Fallback for pages without React props: title/description from the DOM
	// and books from raw book links.
	if s.Name == "" {
		s.Name = strings.TrimSuffix(strings.TrimSpace(doc.Find("h1.gr-h1--serif, [data-testid='seriesTitle'], h1").First().Text()), " Series")
	}
	if s.Description == "" {
		s.Description = cleanText(doc.Find("[data-testid='description'], .seriesDesc").First().Text())
	}
	if len(books) == 0 {
		doc.Find("a[href*='/book/show/']").Each(func(_ int, sel *goquery.Selection) {
			href, _ := sel.Attr("href")
			if m := reBookShow.FindStringSubmatch(href); len(m) > 1 {
				addBook(m[1], "")
			}
		})
	}
	if s.TotalBooks == 0 {
		s.TotalBooks = len(books)
	}
	return s, books, nil
}

// ── List ──────────────────────────────────────────────────────────────────────

var (
	reListTitle  = regexp.MustCompile(`^(.+?)\s*\(([\d,]+)\s+books?\)\s*$`)
	reListVoters = regexp.MustCompile(`([\d,]+)\s+voters?`)
	reListBooks  = regexp.MustCompile(`([\d,]+)\s+books?`)
	reListMade   = regexp.MustCompile(`list created\s+(.+?)\s+by`)
	reAvgRating  = regexp.MustCompile(`([\d.]+)\s+avg rating`)
	reRatings    = regexp.MustCompile(`([\d,]+)\s+ratings?`)
	reScore      = regexp.MustCompile(`score:\s*([\d,]+)`)
	rePeopleVote = regexp.MustCompile(`([\d,]+)\s+people voted`)
)

// ParseList parses a Listopia list page. The list title, books count, voters,
// creator, and tags live in the ".stacked" footer block; each book row is a
// schema.org/Book table row carrying rank, rating, score, and votes.
func ParseList(doc *goquery.Document, listID, pageURL string) (*ScrapedList, []ListBook, error) {
	l := &ScrapedList{ListID: listID, URL: pageURL, FetchedAt: time.Now()}

	// The page <title> is "Name (N books)"; the visible h1 is the score-tooltip
	// header, so it must not be used for the name.
	if m := reListTitle.FindStringSubmatch(strings.TrimSpace(doc.Find("title").First().Text())); len(m) > 2 {
		l.Name = strings.TrimSpace(m[1])
		l.BooksCount = atoiClean(m[2])
	}
	if l.Name == "" {
		h := doc.Find("div.pageHeader h2").First().Clone()
		h.Find("a").Remove()
		l.Name = strings.TrimSpace(strings.TrimLeft(cleanText(h.Text()), "›> "))
	}

	desc := doc.Find("div.u-paddingBottomMedium.mediumText, .listDescription, [data-testid='description']").First().Clone()
	desc.Find("div.right, .right").Remove()
	l.Description = cleanText(desc.Text())

	stacked := doc.Find("div.stacked").First()
	st := stacked.Text()
	if l.BooksCount == 0 {
		if m := reListBooks.FindStringSubmatch(st); len(m) > 1 {
			l.BooksCount = atoiClean(m[1])
		}
	}
	if m := reListVoters.FindStringSubmatch(st); len(m) > 1 {
		l.VotersCount = atoiClean(m[1])
	}
	if m := reListMade.FindStringSubmatch(st); len(m) > 1 {
		l.CreatedDate = strings.TrimSpace(m[1])
	}
	stacked.Find("a[href*='/user/show/']").First().Each(func(_ int, a *goquery.Selection) {
		l.CreatedByUser = strings.TrimSpace(a.Text())
		href, _ := a.Attr("href")
		l.CreatedByUserID = numericPrefix(extractIDFromPath(href, "/user/show/"))
	})
	stacked.Find("a[href*='/list/show_tag/']").Each(func(_ int, sel *goquery.Selection) {
		if tag := strings.TrimSpace(sel.Text()); tag != "" && !contains(l.Tags, tag) {
			l.Tags = append(l.Tags, tag)
		}
	})
	l.LikesCount = atoiClean(doc.Find("span.likesCount").First().Text())

	var books []ListBook
	rank := 1
	doc.Find("tr[itemtype*='schema.org/Book'], tr.bookContainer").Each(func(_ int, row *goquery.Selection) {
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
		if n := atoiClean(row.Find("td.number").First().Text()); n > 0 {
			lb.Rank = n
		}
		mini := row.Find("span.minirating").First().Text()
		if m := reAvgRating.FindStringSubmatch(mini); len(m) > 1 {
			lb.AvgRating = parseFloat(m[1])
		}
		if m := reRatings.FindStringSubmatch(mini); len(m) > 1 {
			lb.RatingsCount = int64(atoiClean(m[1]))
		}
		rowText := row.Text()
		if m := reScore.FindStringSubmatch(rowText); len(m) > 1 {
			lb.Score = atoiClean(m[1])
		}
		if m := rePeopleVote.FindStringSubmatch(rowText); len(m) > 1 {
			lb.Votes = atoiClean(m[1])
		}
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
	if l.BooksCount == 0 {
		l.BooksCount = len(books)
	}
	return l, books, nil
}

// ── Genre ─────────────────────────────────────────────────────────────────────

// ParseGenre parses a Goodreads genre page. The full description sits in a
// hidden "freeText" span (the visible "freeTextContainer" is truncated), and
// the genuine related genres are scoped to the "Related Genres" box so the
// global genre nav does not leak in.
func ParseGenre(doc *goquery.Document, slug, pageURL string) (*ScrapedGenre, error) {
	g := &ScrapedGenre{Slug: slug, URL: pageURL, FetchedAt: time.Now()}
	g.Name = strings.TrimSpace(doc.Find("h1, [data-testid='genreTitle']").First().Text())
	if g.Name == "" {
		g.Name = strings.ReplaceAll(slug, "-", " ")
	}

	descBox := doc.Find("div.mediumText.reviewText, .genreDescription, [data-testid='description']").First()
	if full := descBox.Find("[id^='freeText']:not([id^='freeTextContainer'])").First(); full.Length() > 0 {
		g.Description = cleanText(full.Text())
	} else {
		g.Description = cleanText(descBox.Text())
	}

	// Related genres: the box whose header reads "Related Genres".
	doc.Find("div.bigBox").Each(func(_ int, box *goquery.Selection) {
		if strings.TrimSpace(box.Find("h2.brownBackground").First().Text()) != "Related Genres" {
			return
		}
		box.Find("a[href*='/genres/']").Each(func(_ int, a *goquery.Selection) {
			if name := strings.TrimSpace(a.Text()); name != "" && !contains(g.RelatedGenres, name) {
				g.RelatedGenres = append(g.RelatedGenres, name)
			}
		})
	})

	seen := map[string]bool{}
	doc.Find("a[href*='/book/show/']").Each(func(_ int, sel *goquery.Selection) {
		href, _ := sel.Attr("href")
		if id := extractIDFromPath(href, "/book/show/"); id != "" && !seen[id] {
			seen[id] = true
			g.BookIDs = append(g.BookIDs, id)
		}
	})
	g.BooksCount = len(g.BookIDs)
	return g, nil
}

// ── Quote ─────────────────────────────────────────────────────────────────────

var (
	// The permalink is "/quotes/<id>-slug"; the trailing hyphen distinguishes it
	// from the book "/work/quotes/<workId>" link, which has no slug.
	reQuotePermalink = regexp.MustCompile(`/quotes/(\d+)-`)
	reWorkQuotes     = regexp.MustCompile(`/work/quotes/(\d+)`)
	reBookLinkID     = regexp.MustCompile(`quote_book_link_(\d+)`)
)

// ParseQuotes parses one or more quotes from a quotes page. The .quoteText and
// .quoteFooter blocks are flat siblings (not nested per quote), so attribution,
// the book link, likes, and tags are read from each quote's own subtree and its
// immediately following footer, never from a shared parent.
func ParseQuotes(doc *goquery.Document, pageURL string) ([]ScrapedQuote, error) {
	// The page belongs to one author; the header carries their id.
	var pageAuthorID string
	if a := doc.Find("a[href*='/author/show/']").First(); a.Length() > 0 {
		href, _ := a.Attr("href")
		pageAuthorID = numericPrefix(extractIDFromPath(href, "/author/show/"))
	}

	var quotes []ScrapedQuote
	doc.Find(".quoteText, [data-testid='quoteText']").Each(func(_ int, sel *goquery.Selection) {
		q := ScrapedQuote{URL: pageURL, AuthorID: pageAuthorID, FetchedAt: time.Now()}
		q.Text = cleanQuote(strings.TrimSpace(sel.Clone().Children().Remove().End().Text()))
		if q.Text == "" {
			q.Text = cleanQuote(sel.Text())
		}
		if q.Text == "" {
			return
		}

		// First plain authorOrTitle span is the author; the book title is an
		// <a class="authorOrTitle"> inside the quote_book_link span.
		if a := sel.Find("span.authorOrTitle").First(); a.Length() > 0 {
			q.AuthorName = strings.TrimRight(strings.TrimSpace(a.Text()), ", ")
		}
		bookSpan := sel.Find("span[id^='quote_book_link_']").First()
		if id, ok := bookSpan.Attr("id"); ok {
			if m := reBookLinkID.FindStringSubmatch(id); len(m) > 1 {
				q.BookID = m[1]
			}
		}
		bookLink := bookSpan.Find("a").First()
		q.BookTitle = strings.TrimSpace(bookLink.Text())
		if href, ok := bookLink.Attr("href"); ok {
			if m := reWorkQuotes.FindStringSubmatch(href); len(m) > 1 {
				q.WorkID = m[1]
			}
		}

		footer := sel.NextFiltered(".quoteFooter")
		if perm := footer.Find("div.right a, a[title='View this quote']").First(); perm.Length() > 0 {
			href, _ := perm.Attr("href")
			if m := reQuotePermalink.FindStringSubmatch(href); len(m) > 1 {
				q.QuoteID = m[1]
				q.URL = BaseURL + href
			}
			q.LikesCount = atoiClean(perm.Text())
		}
		footer.Find("a[href*='/quotes/tag/']").Each(func(_ int, a *goquery.Selection) {
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

var (
	reDate          = regexp.MustCompile(`\b(Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)\s+\d{4}\b`)
	reUserTitle     = regexp.MustCompile(`^(.+?)\s*\(([^)]+)\)(?:\s*-\s*(.+?))?\s*\(([\d,]+)\s+books?\)\s*$`)
	reUserRatings   = regexp.MustCompile(`([\d,]+)\s+ratings?`)
	reUserAvg       = regexp.MustCompile(`\(([\d.]+)\s+avg\)`)
	reUserReviews   = regexp.MustCompile(`([\d,]+)\s+reviews?`)
	reFriendsHeader = regexp.MustCompile(`Friends\s+\(([\d,]+)\)`)
	reShelfParam    = regexp.MustCompile(`shelf=([^&"]+)`)
)

// ParseUser parses a Goodreads user profile. The <title> carries name,
// username, location, and total books; the stats line carries ratings, avg, and
// reviews; the read-shelf link carries the read count; and the friends header
// (not the friend list) carries the friends count.
func ParseUser(doc *goquery.Document, userID, pageURL string) (*ScrapedUser, error) {
	u := &ScrapedUser{UserID: userID, URL: pageURL, FetchedAt: time.Now()}
	u.Name = strings.Join(strings.Fields(doc.Find("h1.userProfileName, [data-testid='name']").First().Text()), " ")
	if u.Name == "" {
		u.Name = strings.Join(strings.Fields(doc.Find("h1").First().Text()), " ")
	}

	// "Name (username) - Location (N books)"
	if m := reUserTitle.FindStringSubmatch(strings.TrimSpace(doc.Find("title").First().Text())); len(m) > 4 {
		if u.Name == "" {
			u.Name = strings.Join(strings.Fields(m[1]), " ")
		}
		u.Username = strings.TrimSpace(m[2])
		u.Location = strings.TrimSpace(m[3])
		u.BooksCount = atoiClean(m[4])
	}

	u.AvatarURL, _ = doc.Find("img.profilePictureIcon, img.userPhoto, img[itemprop='image']").First().Attr("src")
	u.Bio = cleanText(doc.Find("[data-testid='aboutMe'], .aboutAuthorInfo, #aboutAuthor, span[id^='freeTextua']").First().Text())
	if u.Location == "" {
		u.Location = strings.TrimSpace(doc.Find("[data-testid='userLocation'], .userLocation").First().Text())
	}
	doc.Find("a[href^='http']").EachWithBreak(func(_ int, sel *goquery.Selection) bool {
		href, _ := sel.Attr("href")
		if !strings.Contains(href, "goodreads.com") && !strings.Contains(href, "gr-assets.com") {
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

	stats := doc.Find("div.profilePageUserStatsInfo").First().Text()
	if m := reUserRatings.FindStringSubmatch(stats); len(m) > 1 {
		u.RatingsCount = atoiClean(m[1])
	}
	if m := reUserAvg.FindStringSubmatch(stats); len(m) > 1 {
		u.AvgRating = parseFloat(m[1])
	}
	if m := reUserReviews.FindStringSubmatch(stats); len(m) > 1 {
		u.ReviewsCount = atoiClean(m[1])
	}
	if m := reFriendsHeader.FindStringSubmatch(doc.Text()); len(m) > 1 {
		u.FriendsCount = atoiClean(m[1])
	}

	// The read-shelf sidebar item reads "read‎ (627)".
	doc.Find("a.userShowPageShelfListItem").Each(func(_ int, a *goquery.Selection) {
		href, _ := a.Attr("href")
		m := reShelfParam.FindStringSubmatch(href)
		if len(m) > 1 && m[1] == "read" {
			u.BooksReadCount = atoiClean(a.Text())
		}
	})

	doc.Find("#featured_shelf a[href*='/book/show/'], .favoriteBooks a[href*='/book/show/']").Each(func(_ int, sel *goquery.Selection) {
		href, _ := sel.Attr("href")
		if id := extractIDFromPath(href, "/book/show/"); id != "" && !contains(u.FavoriteBookIDs, id) {
			u.FavoriteBookIDs = append(u.FavoriteBookIDs, id)
		}
	})
	doc.Find("#currentlyReadingReviews a[href*='/book/show/']").Each(func(_ int, sel *goquery.Selection) {
		href, _ := sel.Attr("href")
		if id := extractIDFromPath(href, "/book/show/"); id != "" && !contains(u.CurrentlyReadingBookIDs, id) {
			u.CurrentlyReadingBookIDs = append(u.CurrentlyReadingBookIDs, id)
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
func ParseReviews(doc *goquery.Document, bookID string) []ScrapedReview {
	var reviews []ScrapedReview
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
			rv := ScrapedReview{BookID: bookID, Text: cleanHTML(r.ReviewBody), FetchedAt: time.Now()}
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
			rv := ScrapedReview{BookID: bookID, FetchedAt: time.Now()}
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

// rawBook is the JSON book record Goodreads embeds in both the autocomplete
// endpoint and the series-page React props. Both carry the same shape, so one
// struct and converter serve both.
type rawBook struct {
	ImageURL  string `json:"imageUrl"`
	BookID    string `json:"bookId"`
	WorkID    string `json:"workId"`
	BookURL   string `json:"bookUrl"`
	Title     string `json:"title"`
	BookTitle string `json:"bookTitleBare"`
	NumPages  int    `json:"numPages"`
	// avgRating/ratingsCount are quoted strings in the autocomplete endpoint but
	// bare numbers in the series React props, so they are decoded leniently.
	AvgRating    json.RawMessage `json:"avgRating"`
	RatingsCount json.RawMessage `json:"ratingsCount"`
	Author       struct {
		ID         int64  `json:"id"`
		Name       string `json:"name"`
		ProfileURL string `json:"profileUrl"`
	} `json:"author"`
	Description struct {
		HTML      string `json:"html"`
		Truncated bool   `json:"truncated"`
	} `json:"description"`
}

// toBook converts a raw embedded book record into a Book.
func (it rawBook) toBook() ScrapedBook {
	b := ScrapedBook{
		BookID:             it.BookID,
		WorkID:             it.WorkID,
		Title:              it.Title,
		TitleWithoutSeries: it.BookTitle,
		Description:        cleanHTML(it.Description.HTML),
		AuthorName:         it.Author.Name,
		Pages:              it.NumPages,
		AvgRating:          parseRawFloat(it.AvgRating),
		RatingsCount:       parseRawInt64(it.RatingsCount),
		CoverURL:           fullCoverURL(it.ImageURL),
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
	return b
}

// ParseAutocompleteBooks parses the rich JSON autocomplete response into Book
// records. The autocomplete endpoint is open (not WAF-challenged), so it is the
// primary way to obtain structured book data without a lent session.
func ParseAutocompleteBooks(body []byte) []ScrapedBook {
	var items []rawBook
	if err := json.Unmarshal(body, &items); err != nil {
		return nil
	}
	out := make([]ScrapedBook, 0, len(items))
	for _, it := range items {
		out = append(out, it.toBook())
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

// reactProps returns the JSON of the first data-react-props attribute for a
// given data-react-class (goquery decodes the HTML entities for us).
func reactProps(doc *goquery.Document, class string) ([]byte, bool) {
	var out []byte
	doc.Find(`[data-react-class="` + class + `"]`).EachWithBreak(func(_ int, sel *goquery.Selection) bool {
		if raw, ok := sel.Attr("data-react-props"); ok && raw != "" {
			out = []byte(raw)
			return false
		}
		return true
	})
	return out, out != nil
}

func absURL(href string) string {
	if strings.HasPrefix(href, "/") {
		return BaseURL + href
	}
	return href
}
