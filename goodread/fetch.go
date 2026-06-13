package goodread

import (
	"context"
	"fmt"
)

// ErrNotFound is returned when a page 404s.
var ErrNotFound = fmt.Errorf("not found")

// GetBook fetches and parses a book page by id (or id-with-slug).
func (c *Client) GetBook(ctx context.Context, id string) (*Book, error) {
	u := BookURL(id)
	doc, code, err := c.FetchHTML(ctx, u)
	if err != nil {
		return nil, err
	}
	if code == 404 || doc == nil {
		return nil, ErrNotFound
	}
	return ParseBook(doc, numericPrefix(id), u)
}

// GetAuthor fetches and parses an author page by id.
func (c *Client) GetAuthor(ctx context.Context, id string) (*Author, error) {
	u := AuthorURL(id)
	doc, code, err := c.FetchHTML(ctx, u)
	if err != nil {
		return nil, err
	}
	if code == 404 || doc == nil {
		return nil, ErrNotFound
	}
	return ParseAuthor(doc, numericPrefix(id), u)
}

// GetSeries fetches and parses a series page by id.
func (c *Client) GetSeries(ctx context.Context, id string) (*Series, []SeriesBook, error) {
	u := SeriesURL(id)
	doc, code, err := c.FetchHTML(ctx, u)
	if err != nil {
		return nil, nil, err
	}
	if code == 404 || doc == nil {
		return nil, nil, ErrNotFound
	}
	return ParseSeries(doc, numericPrefix(id), u)
}

// GetList fetches and parses a Listopia list by id (keeps "<num>.<slug>").
func (c *Client) GetList(ctx context.Context, id string) (*List, []ListBook, error) {
	u := ListURL(id)
	doc, code, err := c.FetchHTML(ctx, u)
	if err != nil {
		return nil, nil, err
	}
	if code == 404 || doc == nil {
		return nil, nil, ErrNotFound
	}
	return ParseList(doc, id, u)
}

// GetGenre fetches and parses a genre page by slug.
func (c *Client) GetGenre(ctx context.Context, slug string) (*Genre, error) {
	u := GenreURL(slug)
	doc, code, err := c.FetchHTML(ctx, u)
	if err != nil {
		return nil, err
	}
	if code == 404 || doc == nil {
		return nil, ErrNotFound
	}
	return ParseGenre(doc, slug, u)
}

// GetUser fetches and parses a public user profile by id.
func (c *Client) GetUser(ctx context.Context, id string) (*User, error) {
	u := UserURL(id)
	doc, code, err := c.FetchHTML(ctx, u)
	if err != nil {
		return nil, err
	}
	if code == 404 || doc == nil {
		return nil, ErrNotFound
	}
	return ParseUser(doc, numericPrefix(id), u)
}

// GetQuotes fetches and parses a quotes page (an author's or a book's quotes URL).
func (c *Client) GetQuotes(ctx context.Context, pageURL string) ([]Quote, error) {
	doc, code, err := c.FetchHTML(ctx, pageURL)
	if err != nil {
		return nil, err
	}
	if code == 404 || doc == nil {
		return nil, ErrNotFound
	}
	return ParseQuotes(doc, pageURL)
}

// GetReviews fetches a book page and returns the reviews embedded on it.
func (c *Client) GetReviews(ctx context.Context, bookID string) ([]Review, error) {
	u := BookURL(bookID)
	doc, code, err := c.FetchHTML(ctx, u)
	if err != nil {
		return nil, err
	}
	if code == 404 || doc == nil {
		return nil, ErrNotFound
	}
	return ParseReviews(doc, numericPrefix(bookID)), nil
}

// GetShelf walks a user's shelf, following pagination up to maxPages
// (maxPages <= 0 means all pages). It returns the shelf header and every row.
func (c *Client) GetShelf(ctx context.Context, userID, shelfName string, maxPages int) (*Shelf, []ShelfBook, error) {
	first := ShelfURL(userID, shelfName, 1)
	doc, code, err := c.FetchHTML(ctx, first)
	if err != nil {
		return nil, nil, err
	}
	if code == 404 || doc == nil {
		return nil, nil, ErrNotFound
	}
	shelf, rows, err := ParseShelf(doc, numericPrefix(userID), shelfName, first)
	if err != nil {
		return nil, nil, err
	}
	pages := 1
	for maxPages <= 0 || pages < maxPages {
		next := ParseShelfNextPage(doc)
		if next == "" {
			break
		}
		doc, code, err = c.FetchHTML(ctx, next)
		if err != nil {
			return shelf, rows, err
		}
		if code == 404 || doc == nil {
			break
		}
		_, more, err := ParseShelf(doc, numericPrefix(userID), shelfName, next)
		if err != nil {
			break
		}
		if len(more) == 0 {
			break
		}
		rows = append(rows, more...)
		pages++
	}
	shelf.BooksCount = len(rows)
	return shelf, rows, nil
}

// SearchBooks returns rich Book records from the open autocomplete endpoint.
func (c *Client) SearchBooks(ctx context.Context, query string, limit int) ([]Book, error) {
	body, code, err := c.Fetch(ctx, SearchAutocompleteURL(query))
	if err != nil {
		return nil, err
	}
	if code != 200 {
		return nil, fmt.Errorf("autocomplete HTTP %d", code)
	}
	books := ParseAutocompleteBooks(body)
	if limit > 0 && len(books) > limit {
		books = books[:limit]
	}
	return books, nil
}

// Search runs autocomplete first (fast, JSON) and falls back to HTML search.
func (c *Client) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	body, code, err := c.Fetch(ctx, SearchAutocompleteURL(query))
	if err == nil && code == 200 {
		if res := ParseSearchAutocomplete(body); len(res) > 0 {
			return trimResults(res, limit), nil
		}
	}
	return c.SearchHTML(ctx, query, limit)
}

// SearchHTML walks the full HTML search results, following pagination until
// limit results are collected (limit <= 0 means a single page).
func (c *Client) SearchHTML(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	var out []SearchResult
	page := 1
	for {
		doc, code, err := c.FetchHTML(ctx, SearchHTMLURL(query, page))
		if err != nil {
			return out, err
		}
		if code == 404 || doc == nil {
			break
		}
		res := ParseSearchHTML(doc)
		out = append(out, res...)
		if limit > 0 && len(out) >= limit {
			break
		}
		if limit <= 0 || ParseSearchHTMLNextPage(doc) == "" {
			break
		}
		page++
	}
	for i := range out {
		out[i].Position = i + 1
	}
	return trimResults(out, limit), nil
}

func trimResults(res []SearchResult, limit int) []SearchResult {
	for i := range res {
		res[i].Position = i + 1
	}
	if limit > 0 && len(res) > limit {
		return res[:limit]
	}
	return res
}
