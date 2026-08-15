package goodread

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

// ExtractAuthor reads an author page.
//
// A Rails template with no Apollo cache and no React island worth reading, so
// there is no level 1 here at all. What it does have is schema.org microdata,
// and that is worth more than it looks: itemprop='ratingValue' and
// itemprop='ratingCount' are a published contract with the search engines, and
// a contract is a great deal more stable than div.dataTitle is. So microdata
// counts as level 2 alongside ld+json and og:, and the level map on an author
// record is mostly 2 with a tail of 3.
//
// The tail of 3 is the biography, the birth and death dates, the influences and
// the works list, which have no microdata and are only in the markup.
func ExtractAuthor(body []byte, pageURL string) (*Extractor, error) {
	e := NewExtractor("s2")

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return e, err
	}

	og := parseOpenGraph(body)
	e.set("name", LevelMeta, og["title"])
	e.set("web_url", LevelMeta, og["url"])
	e.set("image_url", LevelMeta, og["image"])

	// Microdata, level 2. The aggregate is over this author's books and not
	// over any one of them, which is why the model keeps it on the author.
	e.set("average_rating", LevelMeta, parseFloat(doc.Find("span[itemprop='ratingValue']").First().Text()))
	e.set("ratings_count", LevelMeta, microCount(doc, "ratingCount"))
	e.set("text_reviews_count", LevelMeta, microCount(doc, "reviewCount"))
	e.set("name", LevelMeta, strings.TrimSpace(doc.Find("h1.authorName span[itemprop='name']").First().Text()))
	e.set("image_url", LevelMeta, attr(doc.Find("meta[itemprop='image']").First(), "content"))

	// Level 3 from here down. Each of these is registered, because a selector
	// is a promise to break on a redesign and a promise that is written down is
	// one somebody can pay off.
	e.set("born_at", LevelSelector, strings.TrimSpace(doc.Find("div.dataItem[itemprop='birthDate']").First().Text()))
	e.set("died_at", LevelSelector, strings.TrimSpace(doc.Find("div.dataItem[itemprop='deathDate']").First().Text()))
	e.set("born_in", LevelSelector, bornIn(doc))
	e.set("website", LevelSelector, dataItemLink(doc, "Website"))
	e.set("twitter", LevelSelector, dataItemText(doc, "Twitter"))
	e.set("genres", LevelSelector, genreRefsOf(dataItemLinks(doc, "Genre")))
	e.set("influences", LevelSelector, authorRefsOf(dataItemLinks(doc, "Influences")))

	bio, bioHTML := freeTextOf(doc.Find("div.aboutAuthorInfo").First())
	e.set("bio", LevelSelector, bio)
	e.set("bio_html", LevelSelector, bioHTML)

	e.set("followers_count", LevelSelector, followersCount(doc))
	if n, url := worksCount(doc); n > 0 {
		e.set("works_count", LevelSelector, n)
		e.set("works_url", LevelSelector, absURL(url))
	}
	e.set("author_books", LevelSelector, authorBooks(doc))

	if id := extractIDFromPath(pageURL, "/author/show/"); id != "" {
		e.set("legacy_id", LevelSelector, commaFreeInt(numericPrefix(id)))
	}

	if len(e.Fields) == 0 {
		return e, fmt.Errorf("no author data on this page")
	}
	return e, nil
}

func init() {
	for _, f := range []SelectorField{
		{"s2", "Author", "born_at", "div.dataItem[itemprop='birthDate']", "v0.3.0", "no microdata for it anywhere else on the page"},
		{"s2", "Author", "born_in", "div.dataTitle:contains('Born') + text", "v0.3.0", "the place is a bare text node between the label and the date"},
		{"s2", "Author", "bio", "div.aboutAuthorInfo span[id^=freeText]", "v0.3.0", "the full biography, of which the visible one is truncated"},
		{"s2", "Author", "genres", "div.dataTitle:contains('Genre') + div.dataItem a", "v0.3.0", "label and value are siblings, not parent and child"},
		{"s2", "Author", "influences", "div.dataTitle:contains('Influences') + div.dataItem a", "v0.3.0", "same shape as genres"},
		{"s2", "Author", "followers_count", "h2:contains('Followers')", "v0.3.0", "rendered into a heading, with no count anywhere else"},
		{"s2", "Author", "works_count", "a[href^='/author/list/']", "v0.3.0", "the distinct works count sits in the link text"},
		{"s2", "Author", "books", "tr[itemtype$='schema.org/Book']", "v0.3.0", "the works sample, with ratings and edition counts per row"},
	} {
		RegisterSelector(f)
	}
}

// AuthorFrom turns an author extraction into a record.
func AuthorFrom(e *Extractor, id string, retrievedAt time.Time) (*Author, error) {
	if e == nil {
		return nil, fmt.Errorf("no extraction")
	}
	a := &Author{Envelope: envelopeOf(e, "author", retrievedAt)}
	a.LegacyID, _ = int64Of(e.Fields["legacy_id"])
	if a.LegacyID == 0 {
		a.LegacyID, _ = strconv.ParseInt(numericPrefix(id), 10, 64)
	}
	a.WebURL = firstString(e, "web_url")
	if a.WebURL == "" {
		a.WebURL = AuthorURL(id)
	}
	a.Name, _ = e.Fields["name"].(string)
	a.Bio, _ = e.Fields["bio"].(string)
	a.BioHTML, _ = e.Fields["bio_html"].(string)
	a.ImageURL, _ = e.Fields["image_url"].(string)
	a.BornAt, _ = e.Fields["born_at"].(string)
	a.BornIn, _ = e.Fields["born_in"].(string)
	a.DiedAt, _ = e.Fields["died_at"].(string)
	a.Website, _ = e.Fields["website"].(string)
	a.Twitter, _ = e.Fields["twitter"].(string)
	a.Genres, _ = e.Fields["genres"].([]GenreRef)
	a.Influences, _ = e.Fields["influences"].([]Ref)
	a.Stats = statsFrom(e)
	if n, ok := int64Of(e.Fields["followers_count"]); ok {
		a.FollowersCount = &n
	}

	a.Books, _ = e.Fields["author_books"].([]BookCard)
	total, hasTotal := int64Of(e.Fields["works_count"])
	if hasTotal || len(a.Books) > 0 {
		c := &Conn{Loaded: len(a.Books)}
		c.NextURL, _ = e.Fields["works_url"].(string)
		if hasTotal {
			c.TotalCount = &total
		}
		c.Complete = hasTotal && total == int64(len(a.Books))
		for _, b := range a.Books {
			c.Nodes = append(c.Nodes, b.Book)
		}
		a.Works = c
	}

	// The page renders a handful of an author's books and says how many there
	// are. Saying which is which is the difference between a sample and a
	// record that looks complete and is not.
	if hasTotal && total > int64(len(a.Books)) {
		e.Miss("this page shows %d of %s works. `goodread author %s --works` walks /author/list for the rest.",
			len(a.Books), commaInt(total), id)
		a.Missed = append(a.Missed, e.Missed[len(e.Missed)-1])
	}
	return a, nil
}

// microCount reads a schema.org count off its value-title span.
//
// The visible text is grouped with commas and the title attribute is not, so
// the attribute is the one to read. Reading the text and stripping commas works
// until a locale renders them as dots.
func microCount(doc *goquery.Document, prop string) int64 {
	sel := doc.Find("span[itemprop='" + prop + "']").First()
	if sel.Length() == 0 {
		return 0
	}
	if v, ok := sel.Attr("content"); ok {
		return commaFreeInt(v)
	}
	if v, ok := sel.Attr("title"); ok {
		return commaFreeInt(v)
	}
	return commaFreeInt(sel.Text())
}

// bornIn reads the place out from between the label and the date.
//
// The markup is a dataTitle div, then a bare text node holding "in Yate, South
// Gloucestershire, England", then the dataItem holding the date. A bare text
// node with no element around it cannot be selected, so this reads the parent's
// text and takes what is between the two.
func bornIn(doc *goquery.Document) string {
	var out string
	doc.Find("div.dataTitle").EachWithBreak(func(_ int, sel *goquery.Selection) bool {
		if strings.TrimSpace(sel.Text()) != "Born" {
			return true
		}
		for n := sel.Nodes[0].NextSibling; n != nil; n = n.NextSibling {
			// Named constant on purpose. The numeric values are not in the
			// order anyone guesses: TextNode is 1 and ElementNode is 3, so
			// `n.Type == 1` reads like "stop at the element" and actually
			// stops at the text node this whole function is here to read.
			if n.Type == html.ElementNode {
				break
			}
			if n.Type != html.TextNode {
				continue
			}
			if s := strings.TrimSpace(n.Data); s != "" {
				out = strings.TrimSpace(strings.TrimPrefix(s, "in "))
				return false
			}
		}
		return false
	})
	return out
}

// dataItem finds the value div that follows a given label div.
func dataItem(doc *goquery.Document, label string) *goquery.Selection {
	var found *goquery.Selection
	doc.Find("div.dataTitle").EachWithBreak(func(_ int, sel *goquery.Selection) bool {
		if strings.TrimSpace(sel.Text()) != label {
			return true
		}
		found = sel.NextAllFiltered("div.dataItem").First()
		return false
	})
	if found == nil {
		return new(goquery.Selection)
	}
	return found
}

func dataItemText(doc *goquery.Document, label string) string {
	return strings.TrimSpace(dataItem(doc, label).Text())
}

func dataItemLink(doc *goquery.Document, label string) string {
	href, _ := dataItem(doc, label).Find("a").First().Attr("href")
	return href
}

// dataItemLinks returns the label and href of every link under a label.
func dataItemLinks(doc *goquery.Document, label string) [][2]string {
	var out [][2]string
	dataItem(doc, label).Find("a").Each(func(_ int, a *goquery.Selection) {
		name := strings.TrimSpace(a.Text())
		href, _ := a.Attr("href")
		if name == "" || strings.HasPrefix(name, "...") {
			return
		}
		out = append(out, [2]string{name, absURL(href)})
	})
	return out
}

func genreRefsOf(links [][2]string) []GenreRef {
	var out []GenreRef
	for _, l := range links {
		out = append(out, GenreRef{Name: l[0], WebURL: l[1]})
	}
	return out
}

// authorRefsOf turns influence links into refs.
//
// Deduplicated, because the page renders the list twice: once truncated for
// display and once in full behind the "...more" link. Both are in the markup
// and both match the selector.
func authorRefsOf(links [][2]string) []Ref {
	var out []Ref
	seen := map[string]bool{}
	for _, l := range links {
		id := extractIDFromPath(l[1], "/author/show/")
		if id == "" {
			continue
		}
		id = numericPrefix(id)
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, Ref{Type: "Contributor", ID: id, Key: "Contributor:" + id, Title: l[0], Resolved: true})
	}
	return out
}

// freeTextOf reads Goodreads' truncated-with-a-hidden-full-copy pattern.
//
// Every long text on the Rails pages is rendered twice, once truncated in
// #freeTextContainerNNN and once whole in #freeTextNNN with display:none. The
// hidden one is the real value, and a parser that takes the visible one gets a
// biography that ends in the middle of a sentence.
func freeTextOf(sel *goquery.Selection) (text, html string) {
	full := sel.Find("span[id^='freeText']:not([id^='freeTextContainer'])").First()
	if full.Length() == 0 {
		full = sel.Find("span[id^='freeTextContainer']").First()
	}
	if full.Length() == 0 {
		return "", ""
	}
	h, _ := full.Html()
	return strings.TrimSpace(full.Text()), strings.TrimSpace(h)
}

func followersCount(doc *goquery.Document) int64 {
	var n int64
	doc.Find("h2").EachWithBreak(func(_ int, sel *goquery.Selection) bool {
		m := reFollowers.FindStringSubmatch(sel.Text())
		if m == nil {
			return true
		}
		n = commaFreeInt(m[1])
		return false
	})
	return n
}

func worksCount(doc *goquery.Document) (int64, string) {
	var n int64
	var href string
	doc.Find("a[href*='/author/list/']").EachWithBreak(func(_ int, a *goquery.Selection) bool {
		m := reDistinctWorks.FindStringSubmatch(a.Text())
		if m == nil {
			return true
		}
		n = commaFreeInt(m[1])
		href, _ = a.Attr("href")
		return false
	})
	return n, href
}

// authorBooks reads the works sample off the page.
//
// The row shape is shared with the Listopia page, so the reading of it lives in
// extract_rows.go and this only names the surface it came from.
func authorBooks(doc *goquery.Document) []BookCard {
	return schemaBookRows(doc.Selection, "s2")
}

func attr(sel *goquery.Selection, name string) string {
	v, _ := sel.Attr(name)
	return v
}
