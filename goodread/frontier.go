package goodread

import (
	"database/sql"
	"strings"
	"time"
)

// The crawl frontier.
//
// State lives in the store rather than in the process, which is the whole of
// what "resumable" means here: a crawl that was interrupted at hour six of
// twelve is continued by running the same command again, and the pages it
// already read come back out of the cache rather than off the site.
//
// Keyed by URI and not by URL, because the same book is reachable as
// /book/show/2767052 and as /book/show/2767052-the-hunger-games, and a frontier
// keyed by URL would read it twice and call that two books.

const frontierSchema = `
CREATE TABLE IF NOT EXISTS frontier (
  uri        TEXT PRIMARY KEY,
  url        TEXT NOT NULL,
  kind       TEXT NOT NULL,
  depth      INTEGER NOT NULL,
  status     TEXT NOT NULL DEFAULT 'pending',
  error      TEXT,
  updated_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_frontier_pending ON frontier(status, depth, uri);
`

// FrontierItem is one thing waiting to be read.
type FrontierItem struct {
	URI   string `json:"uri"`
	URL   string `json:"url"`
	Kind  string `json:"kind"`
	Depth int    `json:"depth"`
}

// Enqueue adds a URI to the frontier at a depth.
//
// A URI already in the frontier is not re-added, with one exception: reaching
// it again at a shallower depth lowers the recorded depth, because the depth
// budget is measured from the seed and the shortest path is the honest distance.
// Something already read is left alone either way.
func (s *Store) EnqueueURI(uri, url, kind string, depth int) error {
	if uri == "" || url == "" {
		return nil
	}
	_, err := s.db.Exec(
		`INSERT INTO frontier(uri,url,kind,depth,updated_at) VALUES(?,?,?,?,?)
		 ON CONFLICT(uri) DO UPDATE SET
		   depth=MIN(excluded.depth, frontier.depth),
		   url=excluded.url`,
		uri, url, kind, depth, time.Now().Unix())
	return err
}

// NextFrontier claims up to n pending items, shallowest first.
//
// Breadth first, because a depth limited crawl that went deep first would spend
// its whole budget down one arm of the graph and come back with one author's
// entire bibliography and nothing else.
func (s *Store) NextFrontier(n int) ([]FrontierItem, error) {
	rows, err := s.db.Query(
		`SELECT uri,url,kind,depth FROM frontier WHERE status='pending' ORDER BY depth, uri LIMIT ?`, n)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []FrontierItem
	for rows.Next() {
		var it FrontierItem
		if err := rows.Scan(&it.URI, &it.URL, &it.Kind, &it.Depth); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// MarkCrawled records the outcome of reading one URI.
//
// The error text is kept, because "412 pages, 3 errors" is not actionable and
// "3 errors, all of them 404 on a series page" is.
func (s *Store) MarkCrawled(uri, status string, cause error) error {
	var msg any
	if cause != nil {
		msg = cause.Error()
	}
	_, err := s.db.Exec(
		`UPDATE frontier SET status=?, error=?, updated_at=? WHERE uri=?`,
		status, msg, time.Now().Unix(), uri)
	return err
}

// FrontierStats counts the frontier by status.
func (s *Store) FrontierStats() (map[string]int, error) {
	rows, err := s.db.Query(`SELECT status, COUNT(*) FROM frontier GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]int{}
	for rows.Next() {
		var st string
		var n int
		if err := rows.Scan(&st, &n); err != nil {
			return nil, err
		}
		out[st] = n
	}
	return out, rows.Err()
}

// FrontierErrors lists what failed and why.
func (s *Store) FrontierErrors(limit int) ([]FrontierItem, []string, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.Query(
		`SELECT uri,url,kind,depth,COALESCE(error,'') FROM frontier WHERE status='failed' ORDER BY uri LIMIT ?`, limit)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rows.Close() }()
	var items []FrontierItem
	var causes []string
	for rows.Next() {
		var it FrontierItem
		var cause string
		if err := rows.Scan(&it.URI, &it.URL, &it.Kind, &it.Depth, &cause); err != nil {
			return nil, nil, err
		}
		items = append(items, it)
		causes = append(causes, cause)
	}
	return items, causes, rows.Err()
}

// ResetFrontier clears the crawl state without touching what was read.
//
// Two separate things, and the flag that does this says so. Forgetting where a
// crawl got to is cheap; forgetting what it read is hours of somebody else's
// bandwidth.
func (s *Store) ResetFrontier() error {
	_, err := s.db.Exec(`DELETE FROM frontier`)
	return err
}

// CrawledDepth reports the depth a URI was enqueued at, and whether it is done.
func (s *Store) CrawledDepth(uri string) (depth int, status string, err error) {
	err = s.db.QueryRow(`SELECT depth, status FROM frontier WHERE uri=?`, uri).Scan(&depth, &status)
	if err == sql.ErrNoRows {
		return 0, "", ErrNotFound
	}
	return depth, status, err
}

// URIToURL turns a gr: URI into the page to read for it.
//
// Not every node kind has a page. A place, a character and an award are curated
// strings on a work rather than things with URLs of their own, and a quote is
// read through the work or author it hangs off, so those come back not ok and
// the crawl skips them rather than inventing a URL that 404s.
func URIToURL(uri string) (kind, id, pageURL string, ok bool) {
	rest, found := strings.CutPrefix(uri, "gr:")
	if !found {
		return "", "", "", false
	}
	kind, id, found = strings.Cut(rest, "/")
	if !found || id == "" {
		return "", "", "", false
	}
	switch kind {
	case "book":
		return kind, id, BookURL(id), true
	case "author":
		return kind, id, AuthorURL(id), true
	case "series":
		return kind, id, SeriesURL(id), true
	case "list":
		return kind, id, ListURL(id), true
	case "genre":
		return kind, id, GenreURL(id), true
	case "work":
		return kind, id, EditionsURL(id, 1), true
	case "user":
		return kind, id, UserURL(id), true
	}
	return kind, id, "", false
}
