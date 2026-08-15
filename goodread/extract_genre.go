package goodread

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// ExtractGenre reads a genre page.
//
// Worth reading for exactly one thing that exists nowhere else on the site: the
// related genres block, which is the only published edge list between genres. A
// book page tells you a book is fantasy; nothing but this page tells you that
// fantasy sits next to urban fantasy, mythology and dragons.
//
// The books on it are a rotating editorial selection rather than a membership
// list, which is why they land in Books with that said out loud in the record
// rather than in something that sounds like the genre's contents.
func ExtractGenre(body []byte, pageURL string) (*Extractor, error) {
	e := NewExtractor("s5")

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return e, fmt.Errorf("parse genre page: %w", err)
	}

	meta := parseMetaNames(body)
	e.set("description", LevelMeta, meta["description"])
	if href, ok := doc.Find("link[rel='canonical']").First().Attr("href"); ok {
		e.set("web_url", LevelMeta, href)
		e.set("slug", LevelMeta, pathSegmentAfter(href, "/genres/"))
	}

	e.set("name", LevelSelector, strings.TrimSpace(doc.Find("h1.left").First().Text()))
	e.set("related", LevelSelector, relatedGenres(doc))
	e.set("books", LevelSelector, genreBooks(doc))

	if slug := pathSegmentAfter(pageURL, "/genres/"); slug != "" {
		e.set("slug", LevelSelector, slug)
	}

	if len(e.Fields) == 0 {
		return e, fmt.Errorf("no genre data on this page")
	}
	return e, nil
}

// sectionBody finds the box under a heading.
//
// The page is a stack of bigBox sections whose only label is the h2 text, so a
// section has to be found by reading its heading. Fragile and unavoidable: none
// of these boxes carries an id or a section specific class.
func sectionBody(doc *goquery.Document, heading string) *goquery.Selection {
	var found *goquery.Selection
	doc.Find("div.bigBox").EachWithBreak(func(_ int, box *goquery.Selection) bool {
		if !strings.Contains(box.Find("h2").First().Text(), heading) {
			return true
		}
		found = box.Find("div.bigBoxContent").First()
		return false
	})
	if found == nil {
		return doc.Find("nothing-matches-this")
	}
	return found
}

// relatedGenres reads the genre graph.
//
// Scoped to the Related Genres box on purpose. The page also renders the whole site
// genre nav as a.genreList__genreLink, and a query loose enough to pick both up
// would return the same twenty genres for every genre page on the site, which
// is a graph that says nothing while looking like it says something.
func relatedGenres(doc *goquery.Document) []Ref {
	var out []Ref
	seen := map[string]bool{}
	sectionBody(doc, "Related Genres").Find("a[href^='/genres/']").Each(func(_ int, a *goquery.Selection) {
		href, _ := a.Attr("href")
		slug := pathSegmentAfter(href, "/genres/")
		name := strings.TrimSpace(a.Text())
		if slug == "" || name == "" || seen[slug] {
			return
		}
		seen[slug] = true
		out = append(out, Ref{Type: "Genre", ID: slug, Key: "Genre:" + slug, Title: name, Resolved: true})
	})
	return out
}

var (
	reTipCall     = regexp.MustCompile(`new Tip\(\$\('[^']*'\),\s*"((?:[^"\\]|\\.)*)"`)
	reJSUnescape  = regexp.MustCompile(`\\(.)`)
	reGenreCoverT = regexp.MustCompile(`bookCover\d+_(\d+)`)
)

// genreBooks reads the featured shelves.
//
// The visible markup for one of these boxes is a cover image and a link, and
// that is all: no title element, no author, no rating. Everything else the page
// knows is in the tooltip, which Goodreads builds as an escaped HTML string
// inside a `new Tip(...)` call in a script tag next to the cover.
//
// So it gets read. The alternative is a book card with a cover and an id and
// nothing else, when the page plainly has the author, the rating, the year and
// the description sitting right there. The string is unescaped and parsed as
// the HTML fragment it is, with nothing evaluated as JavaScript.
func genreBooks(doc *goquery.Document) []BookCard {
	var out []BookCard
	seen := map[string]bool{}
	doc.Find("div.bookBox").Each(func(_ int, box *goquery.Selection) {
		cover := box.Find("div.coverWrapper").First()
		id := ""
		if v, ok := cover.Attr("id"); ok {
			if m := reGenreCoverT.FindStringSubmatch(v); m != nil {
				id = m[1]
			}
		}
		link := box.Find("a[href*='/book/show/']").First()
		href, _ := link.Attr("href")
		if id == "" {
			id = numericPrefix(extractIDFromPath(href, "/book/show/"))
		}
		if id == "" || seen[id] {
			return
		}
		seen[id] = true

		img := box.Find("img").First()
		title := strings.TrimSpace(attr(img, "alt"))
		c := BookCard{
			Book:      Ref{Type: "Book", ID: id, Key: "Book:" + id, Title: TitleWithoutSeries(title), Resolved: true},
			Title:     title,
			TitleBare: TitleWithoutSeries(title),
			ImageURL:  attr(img, "src"),
			Via:       "s5",
		}
		fillFromTip(&c, box)
		out = append(out, c)
	})
	return out
}

// fillFromTip parses the tooltip payload sitting beside a cover.
func fillFromTip(c *BookCard, box *goquery.Selection) {
	m := reTipCall.FindStringSubmatch(box.Find("script").Text())
	if m == nil {
		return
	}
	// A JavaScript string literal, so the backslash escapes come out before it
	// is HTML at all. \n and \/ are the only two Goodreads actually emits here,
	// and the general rule handles both without guessing at the rest.
	frag := reJSUnescape.ReplaceAllStringFunc(m[1], func(s string) string {
		switch s[1] {
		case 'n':
			return "\n"
		case 't':
			return "\t"
		default:
			return string(s[1])
		}
	})
	tip, err := goquery.NewDocumentFromReader(strings.NewReader(frag))
	if err != nil {
		return
	}
	if t := strings.TrimSpace(tip.Find("a.bookTitle").First().Text()); t != "" {
		c.Title = t
		c.TitleBare = TitleWithoutSeries(t)
		c.Book.Title = c.TitleBare
	}
	tip.Find("a.authorName").Each(func(_ int, a *goquery.Selection) {
		name := strings.TrimSpace(a.Text())
		if name == "" {
			return
		}
		href, _ := a.Attr("href")
		id := numericPrefix(extractIDFromPath(href, "/author/show/"))
		c.Contributors = append(c.Contributors, Contributor{
			ID: id, LegacyID: commaFreeInt(id), Name: name, Role: "Author", WebURL: absURL(href), Via: "s5",
		})
	})
	mini := tip.Find("span.minirating").First().Text()
	if m := reMiniRating.FindStringSubmatch(mini); m != nil {
		if v := parseFloat(m[1]); v > 0 {
			c.AverageRating = &v
		}
	}
	if m := reMiniRatings.FindStringSubmatch(mini); m != nil {
		if v := commaFreeInt(m[1]); v > 0 {
			c.RatingsCount = &v
		}
	}
	if m := rePublished.FindStringSubmatch(tip.Text()); m != nil {
		c.PublishedAt = m[1]
	}
	desc, _ := freeTextOf(tip.Find("div.addBookTipDescription").First())
	c.Description = desc
}

// GenreFrom turns a genre extraction into a record.
func GenreFrom(e *Extractor, slug string, retrievedAt time.Time) (*Genre, error) {
	if e == nil {
		return nil, fmt.Errorf("no extraction")
	}
	g := &Genre{Envelope: envelopeOf(e, "genre", retrievedAt)}
	g.Slug = slug
	if v := firstString(e, "slug"); v != "" {
		g.Slug = v
	}
	g.WebURL = firstString(e, "web_url")
	if g.WebURL == "" {
		g.WebURL = GenreURL(g.Slug)
	}
	g.Name, _ = e.Fields["name"].(string)
	g.Description, _ = e.Fields["description"].(string)
	g.Related, _ = e.Fields["related"].([]Ref)
	g.Books, _ = e.Fields["books"].([]BookCard)

	// Said every time rather than only when something looks short, because
	// there is no count on this page to compare against. The absence of a total
	// is exactly why the caveat has to be unconditional.
	e.Miss("the books on a genre page are what it features today, which rotates. this is not the membership of the genre and there is no count on the page to say how large that is.")
	g.Missed = append(g.Missed, e.Missed[len(e.Missed)-1])
	return g, nil
}
