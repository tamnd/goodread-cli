package goodread

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// ExtractEditions reads a work's editions page.
//
// The surface that answers the question the book page cannot: which editions
// are the same work. A book id is one edition, and the ISBN, the format, the
// publisher and the page count all differ between them while the text does not.
// Anything that wants to talk about the book rather than about one printing has
// to come through here.
//
// The rows are the most regular markup on the whole site: a dataTitle and a
// dataValue for every field, always the same labels. No microdata, no JSON, and
// none needed.
func ExtractEditions(body []byte, pageURL string) (*Extractor, error) {
	e := NewExtractor("s6")

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return e, fmt.Errorf("parse editions page: %w", err)
	}

	// The heading renders "The Hunger Games > Editions" and the crumb comes off
	// the same way it does on a Listopia page.
	head := strings.TrimSpace(doc.Find("h1").First().Text())
	head = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(head), "> Editions"))
	e.set("title", LevelSelector, head)

	e.set("editions", LevelSelector, editionRows(doc))
	if n := showingOf(doc); n > 0 {
		e.set("total_count", LevelSelector, n)
	}
	if p := currentPage(doc); p > 0 {
		e.set("page", LevelSelector, p)
	}
	if id := numericPrefix(extractIDFromPath(pageURL, "/work/editions/")); id != "" {
		e.set("work_id", LevelSelector, id)
	}

	if len(e.Fields) == 0 {
		return e, fmt.Errorf("no editions data on this page")
	}
	return e, nil
}

var (
	rePublishedBy = regexp.MustCompile(`(?s)Published\s+(.*?)(?:\s+by\s+(.*))?$`)
	reFormatPages = regexp.MustCompile(`([\d,]+)\s+pages?$`)
	reISBN13      = regexp.MustCompile(`\b(\d{13})\b`)
	reISBN10      = regexp.MustCompile(`ISBN10:\s*([\dXx]{10})`)
	reEdRating    = regexp.MustCompile(`([\d.]+)`)
	reEdRatings   = regexp.MustCompile(`([\d,]+)\s+ratings?`)
)

// editionRows reads one edition per editionData block.
//
// The first two or three dataRows are unlabelled, positional: title, then the
// publication line, then the format line. Everything after that lives under
// moreDetails as a labelled pair. So the top is read by position and the rest
// by label, which is what the markup actually offers rather than a scheme
// imposed on it.
func editionRows(doc *goquery.Document) []Edition {
	var out []Edition
	doc.Find("div.editionData").Each(func(_ int, box *goquery.Selection) {
		link := box.Find("a.bookTitle").First()
		href, _ := link.Attr("href")
		id := numericPrefix(extractIDFromPath(href, "/book/show/"))
		if id == "" {
			return
		}
		title := strings.TrimSpace(link.Text())
		ed := Edition{
			Book:  Ref{Type: "Book", ID: id, Key: "Book:" + id, Title: TitleWithoutSeries(title), Resolved: true},
			Title: title,
			Via:   "s6",
		}

		box.ChildrenFiltered("div.dataRow").Each(func(i int, row *goquery.Selection) {
			text := strings.TrimSpace(collapseSpace(row.Text()))
			switch {
			case row.Find("a.bookTitle").Length() > 0 || text == "":
				return
			case strings.HasPrefix(text, "Published"):
				if m := rePublishedBy.FindStringSubmatch(text); m != nil {
					ed.PublishedAt = strings.TrimSpace(m[1])
					ed.Publisher = strings.TrimSpace(m[2])
				}
			case strings.Contains(text, "more details"):
				return
			default:
				ed.Name, ed.Format, ed.NumPages = parseFormatLine(text)
			}
		})

		box.Find("div.moreDetails div.dataRow").Each(func(_ int, row *goquery.Selection) {
			label := strings.Trim(strings.TrimSpace(row.Find("div.dataTitle").First().Text()), ": ")
			value := row.Find("div.dataValue").First()
			text := strings.TrimSpace(collapseSpace(value.Text()))
			switch label {
			case "Author(s)":
				ed.Contributors = contributorRows(value, "s6")
			case "ISBN":
				if m := reISBN13.FindStringSubmatch(text); m != nil {
					ed.ISBN13 = m[1]
				}
				if m := reISBN10.FindStringSubmatch(text); m != nil {
					ed.ISBN = m[1]
				}
			case "ASIN":
				ed.ASIN = text
			case "Edition language":
				ed.Language = text
			case "Average rating":
				if m := reEdRating.FindStringSubmatch(text); m != nil {
					if v := parseFloat(m[1]); v > 0 {
						ed.AverageRating = &v
					}
				}
				if m := reEdRatings.FindStringSubmatch(text); m != nil {
					if v := commaFreeInt(m[1]); v > 0 {
						ed.RatingsCount = &v
					}
				}
			}
		})
		out = append(out, ed)
	})
	return out
}

// parseFormatLine reads "Reprint Edition, Kindle Edition, 387 pages".
//
// Read from the right, because that is the end that is predictable. The page
// count is last when it is there at all, the format is the segment before it,
// and anything still to the left is the edition designation. Reading from the
// left instead means guessing whether the first segment is a format or a
// qualifier, and "Kindle Edition" and "Reprint Edition" look identical to a
// guess like that.
func parseFormatLine(text string) (name, format string, pages *int) {
	// The page count comes off the whole string before anything is split,
	// because a book of 1,024 pages has a comma inside the number and splitting
	// first turns it into a format called "1" and a count of 24.
	if m := reFormatPages.FindStringSubmatch(text); m != nil {
		if n, err := strconv.Atoi(strings.ReplaceAll(m[1], ",", "")); err == nil && n > 0 {
			pages = &n
		}
		text = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(text[:len(text)-len(m[0])]), ","))
	}
	var parts []string
	for _, p := range strings.Split(text, ",") {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return "", "", pages
	}
	format = parts[len(parts)-1]
	name = strings.Join(parts[:len(parts)-1], ", ")
	return name, format, pages
}

// collapseSpace turns the template's indentation into single spaces.
//
// These rows are rendered across several lines with tabs, so the text of one
// comes out with runs of whitespace in the middle of a sentence. Every string
// comparison below would have to account for that otherwise.
func collapseSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// EditionsFrom turns an editions extraction into a record.
func EditionsFrom(e *Extractor, workID string, retrievedAt time.Time) (*Editions, error) {
	if e == nil {
		return nil, fmt.Errorf("no extraction")
	}
	ed := &Editions{Envelope: envelopeOf(e, "editions", retrievedAt)}
	id := workID
	if v := firstString(e, "work_id"); v != "" {
		id = numericPrefix(v)
	}
	ed.Work = Ref{Type: "Work", ID: id, Key: "Work:" + id, Resolved: id != ""}
	ed.WebURL = EditionsURL(id, 0)
	ed.Title, _ = e.Fields["title"].(string)
	ed.Work.Title = ed.Title
	if n, ok := int64Of(e.Fields["total_count"]); ok {
		ed.TotalCount = &n
	}
	if p, ok := e.Fields["page"].(int); ok {
		ed.Page = p
	}
	ed.Editions, _ = e.Fields["editions"].([]Edition)
	ed.Complete = ed.TotalCount != nil && int64(len(ed.Editions)) >= *ed.TotalCount

	if !ed.Complete && ed.TotalCount != nil {
		e.Miss("this page carries %d of %d editions. `goodread editions %s --page N` reads the rest, at %d pages in total.",
			len(ed.Editions), *ed.TotalCount, id, pageCount(*ed.TotalCount, len(ed.Editions)))
		ed.Missed = append(ed.Missed, e.Missed[len(e.Missed)-1])
	}
	return ed, nil
}

// pageCount says how many requests the rest of a paginated surface would cost.
//
// Worth stating rather than leaving to the reader. 527 editions at ten a page
// is 53 requests and just under two minutes at the polite pace, which is a
// decision somebody should get to make with the number in front of them.
func pageCount(total int64, perPage int) int {
	if perPage <= 0 {
		return 0
	}
	return int((total + int64(perPage) - 1) / int64(perPage))
}
