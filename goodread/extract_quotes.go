package goodread

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// ExtractQuotes reads a quotes page.
//
// Two URLs land here, /work/quotes and /author/quotes, and they render the same
// markup around a different subject. The record says which, because a file with
// both kinds in it and no way to tell them apart is a file nobody can use.
//
// Quote text is authored content and 00_overview.md section 4 governs it the
// same way it governs review text: returned when asked for, not swept up by a
// crawl that was after something else.
func ExtractQuotes(body []byte, pageURL string) (*Extractor, error) {
	e := NewExtractor("s7")

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return e, fmt.Errorf("parse quotes page: %w", err)
	}

	// "The Hunger Games Quotes" with the one word the template adds taken off.
	head := strings.TrimSpace(doc.Find("h1").First().Text())
	e.set("title", LevelSelector, strings.TrimSpace(strings.TrimSuffix(head, "Quotes")))

	e.set("quotes", LevelSelector, quoteBlocks(doc.Selection, "s7"))
	if n := showingOf(doc); n > 0 {
		e.set("total_count", LevelSelector, n)
	}
	if p := currentPage(doc); p > 0 {
		e.set("page", LevelSelector, p)
	}
	if id := numericPrefix(extractIDFromPath(pageURL, "/work/quotes/")); id != "" {
		e.set("subject_type", LevelSelector, "Work")
		e.set("subject_id", LevelSelector, id)
	}
	if id := numericPrefix(extractIDFromPath(pageURL, "/author/quotes/")); id != "" {
		e.set("subject_type", LevelSelector, "Author")
		e.set("subject_id", LevelSelector, id)
	}

	if len(e.Fields) == 0 {
		return e, fmt.Errorf("no quotes on this page")
	}
	return e, nil
}

// QuotesFrom turns a quotes extraction into a record.
func QuotesFrom(e *Extractor, subjectID string, retrievedAt time.Time) (*Quotes, error) {
	if e == nil {
		return nil, fmt.Errorf("no extraction")
	}
	q := &Quotes{Envelope: envelopeOf(e, "quotes", retrievedAt)}

	kind := firstString(e, "subject_type")
	if kind == "" {
		kind = "Work"
	}
	id := subjectID
	if v := firstString(e, "subject_id"); v != "" {
		id = v
	}
	q.Subject = Ref{Type: kind, ID: id, Key: kind + ":" + id, Resolved: id != ""}
	q.Subject.Title, _ = e.Fields["title"].(string)
	if kind == "Work" {
		q.WebURL = WorkQuotesURL(id, 0)
	} else {
		q.WebURL = BaseURL + "/author/quotes/" + id
	}

	if n, ok := int64Of(e.Fields["total_count"]); ok {
		q.TotalCount = &n
	}
	if p, ok := e.Fields["page"].(int); ok {
		q.Page = p
	}
	q.Quotes, _ = e.Fields["quotes"].([]Quote)
	q.Complete = q.TotalCount != nil && int64(len(q.Quotes)) >= *q.TotalCount

	if !q.Complete && q.TotalCount != nil {
		e.Miss("this page carries %d of %d quotes. `goodread quotes %s --page N` reads the rest, at %d pages in total.",
			len(q.Quotes), *q.TotalCount, id, pageCount(*q.TotalCount, len(q.Quotes)))
		q.Missed = append(q.Missed, e.Missed[len(e.Missed)-1])
	}
	return q, nil
}
