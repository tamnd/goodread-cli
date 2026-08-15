package goodread

import (
	"context"
	"time"
)

// The v0.3.0 fetch path for the six Rails surfaces.
//
// Every one of these is the same four steps: fetch, refuse an empty or missing
// page, extract, build the record. They are written out rather than folded into
// one generic because the extractor signatures differ in what they need to know
// about the URL, and a generic that took a closure per surface would be longer
// than the six of them together.
//
// GetAuthor and friends in fetch.go are the v0.2.0 path and still return the
// Scraped shapes. The two live side by side until the v0.2.0 commands are gone.

// GetAuthorRecord fetches an author page and reads it into the v0.3.0 model.
func (c *Client) GetAuthorRecord(ctx context.Context, id string) (*Author, error) {
	u := c.site(AuthorURL(id))
	body, err := c.fetchPage(ctx, u)
	if err != nil {
		return nil, err
	}
	e, err := ExtractAuthor(body, u)
	if err != nil {
		return nil, err
	}
	rec, err := AuthorFrom(e, numericPrefix(id), time.Now().UTC())
	if err != nil {
		return nil, err
	}
	c.stamp(&rec.Envelope, u)
	return rec, nil
}

// GetSeriesRecord fetches a series page and reads it into the v0.3.0 model.
func (c *Client) GetSeriesRecord(ctx context.Context, id string) (*Series, error) {
	u := c.site(SeriesURL(id))
	body, err := c.fetchPage(ctx, u)
	if err != nil {
		return nil, err
	}
	e, err := ExtractSeries(body)
	if err != nil {
		return nil, err
	}
	rec, err := SeriesFrom(e, numericPrefix(id), time.Now().UTC())
	if err != nil {
		return nil, err
	}
	c.stamp(&rec.Envelope, u)
	return rec, nil
}

// GetListRecord fetches a Listopia page and reads it into the v0.3.0 model.
//
// The id keeps its slug, because /list/show/1 redirects and /list/show/1.Best_
// Books_Ever answers.
func (c *Client) GetListRecord(ctx context.Context, id string, page int) (*List, error) {
	u := c.site(ListURL(id))
	if page > 1 {
		u += "?page=" + itoa(page)
	}
	body, err := c.fetchPage(ctx, u)
	if err != nil {
		return nil, err
	}
	e, err := ExtractList(body, u)
	if err != nil {
		return nil, err
	}
	rec, err := ListFrom(e, id, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	c.stamp(&rec.Envelope, u)
	return rec, nil
}

// GetGenreRecord fetches a genre page and reads it into the v0.3.0 model.
func (c *Client) GetGenreRecord(ctx context.Context, slug string) (*Genre, error) {
	u := c.site(GenreURL(slug))
	body, err := c.fetchPage(ctx, u)
	if err != nil {
		return nil, err
	}
	e, err := ExtractGenre(body, u)
	if err != nil {
		return nil, err
	}
	rec, err := GenreFrom(e, slug, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	c.stamp(&rec.Envelope, u)
	return rec, nil
}

// GetEditionsRecord fetches one page of a work's editions.
//
// The id is a work id. Passing a book id here is the most common way to get a
// 404 out of this surface, and the error says so rather than leaving the caller
// to work out that their id was the right shape and the wrong kind.
func (c *Client) GetEditionsRecord(ctx context.Context, workID string, page int) (*Editions, error) {
	u := c.site(EditionsURL(workID, page))
	body, err := c.fetchPage(ctx, u)
	if err != nil {
		return nil, err
	}
	e, err := ExtractEditions(body, u)
	if err != nil {
		return nil, err
	}
	rec, err := EditionsFrom(e, numericPrefix(workID), time.Now().UTC())
	if err != nil {
		return nil, err
	}
	c.stamp(&rec.Envelope, u)
	return rec, nil
}

// GetQuotesRecord fetches one page of a work's or an author's quotes.
//
// The two are one method because they are one page. The subject kind decides
// the URL and nothing else, and the record carries which one it was.
func (c *Client) GetQuotesRecord(ctx context.Context, id string, byAuthor bool, page int) (*Quotes, error) {
	u := c.site(WorkQuotesURL(id, page))
	if byAuthor {
		u = c.site(AuthorQuotesURL(id, page))
	}
	body, err := c.fetchPage(ctx, u)
	if err != nil {
		return nil, err
	}
	e, err := ExtractQuotes(body, u)
	if err != nil {
		return nil, err
	}
	rec, err := QuotesFrom(e, numericPrefix(id), time.Now().UTC())
	if err != nil {
		return nil, err
	}
	c.stamp(&rec.Envelope, u)
	return rec, nil
}

// fetchPage is the fetch and the two refusals every one of these shares.
//
// An empty body is treated as a 404 rather than as an empty page, because that
// is what it has always meant here: the challenge pages and the blocked
// responses come back empty, and a record built out of one would be a page of
// absent fields with no error attached.
func (c *Client) fetchPage(ctx context.Context, u string) ([]byte, error) {
	body, code, err := c.Fetch(ctx, u)
	if err != nil {
		return nil, err
	}
	if code == 404 || len(body) == 0 {
		return nil, ErrNotFound
	}
	return body, nil
}

// GetShelfRecord reads a user's shelf and returns it on the v0.3.0 model.
//
// Two routes, and the caller picks. RSS is the default everywhere it is offered
// because it is small, stable and gives more per row than the rendered page
// does. The HTML shelf is a megabyte of table and is worth walking only when
// somebody wants the whole shelf rather than the feed's window.
func (c *Client) GetShelfRecord(ctx context.Context, userID, shelfName string, useHTML bool, maxPages int) (*ShelfRecord, error) {
	var (
		shelf  *Shelf
		rows   []ShelfBook
		err    error
		source = "rss"
	)
	if useHTML {
		source = "html"
		shelf, rows, err = c.GetShelf(ctx, userID, shelfName, maxPages)
	} else {
		shelf, rows, err = c.GetShelfRSS(ctx, userID, shelfName)
	}
	// Whatever was read before the error is still worth having. A walk that
	// stopped on page nine of twenty has nine pages of real rows in it, and
	// throwing them away costs the user nine requests they already spent.
	if err != nil && len(rows) == 0 {
		return nil, err
	}
	rec := ShelfFrom(shelf, rows, source, time.Now().UTC())
	u := c.site(ShelfRSSURL(userID, shelfName))
	if useHTML {
		u = c.site(ShelfURL(userID, shelfName, 1))
	}
	c.stamp(&rec.Envelope, u)
	if err != nil {
		rec.Missed = append(rec.Missed, "the read stopped early: "+err.Error())
	}
	return rec, nil
}
