package goodread

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// The graph store, as 04_graph.md section 7 lays it out.
//
// One table per node type plus one edge table. The full record lives in `json`
// and the columns are an index over it rather than a second copy of the model,
// which is what lets a field be added to a record without a migration.
//
// The one rule worth reading twice is the upsert. A later read of the same URI
// merges rather than replaces, field by field, newest non absent wins. A quick
// read after a full read must not delete the editions list. Getting that
// backwards does not fail loudly; it quietly eats data over a long crawl and
// nobody notices until the crawl is finished, which is why it is a test.

// nodeTables is every node type the store holds.
//
// One table each rather than a single polymorphic table, because the columns
// that matter differ: a book has an ISBN13 and an ASIN and `goodread lookup`
// has to hit an index on them, and no other node type has anything like them.
var nodeTables = []string{"books", "works", "authors", "series", "lists", "genres", "quotes", "shelves", "users"}

// graphSchema is the shared shape.
//
// uri is the primary key everywhere, and it is the `gr:` form rather than a
// bare id, because a bare 153394 is an author to one table and a book to
// another and the graph has both in it at once.
const graphSchema = `
CREATE TABLE IF NOT EXISTS %s (
  uri          TEXT PRIMARY KEY,
  id           TEXT,
  legacy_id    INTEGER,
  title        TEXT,
  json         TEXT NOT NULL,
  surfaces     TEXT NOT NULL DEFAULT '[]',
  retrieved_at INTEGER NOT NULL
);
`

// bookColumns are the extra columns only books carry.
//
// isbn13 and asin are indexed because lookup is the allowed replacement for
// site search, and a replacement that takes a table scan is not a replacement.
const bookColumns = `
ALTER TABLE books ADD COLUMN isbn13 TEXT;
ALTER TABLE books ADD COLUMN asin TEXT;
ALTER TABLE books ADD COLUMN num_pages INTEGER;
ALTER TABLE books ADD COLUMN publisher TEXT;
ALTER TABLE books ADD COLUMN published_at INTEGER;
ALTER TABLE books ADD COLUMN work_uri TEXT;
`

const graphIndexes = `
CREATE INDEX IF NOT EXISTS idx_books_isbn13 ON books(isbn13) WHERE isbn13 IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_books_asin ON books(asin) WHERE asin IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_books_legacy ON books(legacy_id);
CREATE INDEX IF NOT EXISTS idx_books_work ON books(work_uri) WHERE work_uri IS NOT NULL;

CREATE TABLE IF NOT EXISTS edges (
  src       TEXT NOT NULL,
  predicate TEXT NOT NULL,
  dst       TEXT NOT NULL,
  props     TEXT,
  surface   TEXT NOT NULL,
  seen_at   INTEGER NOT NULL,
  PRIMARY KEY (src, predicate, dst)
);
CREATE INDEX IF NOT EXISTS idx_edges_dst ON edges(dst, predicate);

-- The full text index. Contentless, with the row keyed by uri, because the
-- record it indexes already lives in a node table and a second copy would be a
-- second thing to keep in step.
CREATE VIRTUAL TABLE IF NOT EXISTS search USING fts5(
  uri UNINDEXED, kind UNINDEXED, title, description, author, tokenize='porter unicode61'
);
`

// migrateGraph builds the graph tables next to the v0.2.0 records table.
//
// The old table stays. It is what the crawl queue and `goodread db` were built
// on and they still work; this is the shape the record model wants and the two
// live side by side until the crawl commands move too.
func (s *Store) migrateGraph() error {
	for _, t := range nodeTables {
		if _, err := s.db.Exec(fmt.Sprintf(graphSchema, t)); err != nil {
			return fmt.Errorf("create %s: %w", t, err)
		}
	}
	// Added one at a time and the error swallowed, because SQLite has no ADD
	// COLUMN IF NOT EXISTS and the second run of a migration is the normal case
	// rather than the exceptional one.
	for _, stmt := range strings.Split(strings.TrimSpace(bookColumns), "\n") {
		if stmt = strings.TrimSpace(stmt); stmt != "" {
			_, _ = s.db.Exec(stmt)
		}
	}
	_, err := s.db.Exec(graphIndexes)
	return err
}

// Node is one record on its way into or out of the store.
type Node struct {
	URI         string
	Kind        string
	ID          string
	LegacyID    int64
	Title       string
	JSON        []byte
	Surfaces    []string
	RetrievedAt time.Time

	// Book only. Left zero for every other kind, which is why they are not
	// pointers: an author with no ISBN is not a fact anybody needs to record.
	ISBN13      string
	ASIN        string
	NumPages    int
	Publisher   string
	PublishedAt int64
	WorkURI     string

	// The three fields the full text index reads. Kept off the node tables,
	// because a description is a paragraph and the node tables are meant to
	// stay narrow enough to scan.
	Description string
	AuthorName  string
}

// tableFor maps a kind to its table.
func tableFor(kind string) (string, error) {
	switch kind {
	case "book", "books":
		return "books", nil
	case "work", "works":
		return "works", nil
	case "author", "authors":
		return "authors", nil
	case "series":
		return "series", nil
	case "list", "lists":
		return "lists", nil
	case "genre", "genres":
		return "genres", nil
	case "quote", "quotes":
		return "quotes", nil
	case "shelf", "shelves":
		return "shelves", nil
	case "user", "users":
		return "users", nil
	default:
		return "", fmt.Errorf("no table for %q", kind)
	}
}

// PutNode writes a node, merging with whatever is already there.
//
// Merge and not replace. See MergeRecords for what that means field by field.
func (s *Store) PutNode(n Node) error {
	table, err := tableFor(n.Kind)
	if err != nil {
		return err
	}
	if n.URI == "" {
		return fmt.Errorf("a node with no uri cannot be stored")
	}

	var (
		oldJSON []byte
		oldAt   int64
		raw     string
	)
	err = s.db.QueryRow(`SELECT json, retrieved_at FROM `+table+` WHERE uri=?`, n.URI).Scan(&raw, &oldAt)
	switch {
	case err == sql.ErrNoRows:
	case err != nil:
		return err
	default:
		oldJSON = []byte(raw)
	}

	merged := n.JSON
	at := n.RetrievedAt.Unix()
	if oldJSON != nil {
		if merged, err = MergeRecords(oldJSON, n.JSON); err != nil {
			return err
		}
		// The newest of the two, so a merge of an old read into a new one does
		// not date the result to the older read.
		if oldAt > at {
			at = oldAt
		}
	}

	seenOn := mergeSurfaces(oldJSON, n.Surfaces)
	surfaces, _ := json.Marshal(seenOn)

	// The union goes back into the record as well as into the column. The
	// general merge rule is newest non absent wins, which is right for a title
	// and wrong for this one field: surfaces is a list of where the record has
	// been read from, so the second read adds to it rather than replacing it,
	// and a record whose envelope disagreed with its own column would be
	// answering the same question two ways.
	if len(seenOn) > 0 {
		if merged, err = setKey(merged, "surfaces", surfaces); err != nil {
			return err
		}
	}

	cols := []string{"uri", "id", "legacy_id", "title", "json", "surfaces", "retrieved_at"}
	args := []any{n.URI, nullIfEmpty(n.ID), n.LegacyID, n.Title, string(merged), string(surfaces), at}
	if table == "books" {
		cols = append(cols, "isbn13", "asin", "num_pages", "publisher", "published_at", "work_uri")
		args = append(args,
			nullIfEmpty(n.ISBN13), nullIfEmpty(n.ASIN), nullIfZero(int64(n.NumPages)),
			nullIfEmpty(n.Publisher), nullIfZero(n.PublishedAt), nullIfEmpty(n.WorkURI))
	}

	// The columns update only when the new read has something to say, the same
	// rule the json merge follows. A quick read that never looked at the ISBN
	// must not blank the one a full read found.
	var sets []string
	for _, c := range cols {
		if c == "uri" {
			continue
		}
		if c == "json" || c == "surfaces" || c == "retrieved_at" {
			sets = append(sets, c+"=excluded."+c)
			continue
		}
		sets = append(sets, fmt.Sprintf("%s=COALESCE(NULLIF(excluded.%s,''), %s.%s)", c, c, table, c))
	}

	q := fmt.Sprintf(`INSERT INTO %s(%s) VALUES(%s) ON CONFLICT(uri) DO UPDATE SET %s`,
		table, strings.Join(cols, ","), placeholders(len(cols)), strings.Join(sets, ", "))
	if _, err := s.db.Exec(q, args...); err != nil {
		return err
	}
	return s.indexNode(n)
}

// indexNode keeps the full text index in step with the node.
//
// Deleted and reinserted rather than updated, because fts5 has no upsert and a
// row that was indexed under an old title would otherwise answer to both.
func (s *Store) indexNode(n Node) error {
	if _, err := s.db.Exec(`DELETE FROM search WHERE uri=?`, n.URI); err != nil {
		return err
	}
	if n.Title == "" && n.Description == "" && n.AuthorName == "" {
		return nil
	}
	_, err := s.db.Exec(`INSERT INTO search(uri,kind,title,description,author) VALUES(?,?,?,?,?)`,
		n.URI, n.Kind, n.Title, n.Description, n.AuthorName)
	return err
}

// PutEdge records one relationship.
//
// The surface is on the edge and not just on the node, because "this author
// wrote this book" read off a Listopia row and read off the book page are the
// same claim from two places, and which one said it is worth keeping.
func (s *Store) PutEdge(src, predicate, dst string, props any, surface string, seenAt time.Time) error {
	if src == "" || dst == "" || predicate == "" {
		return fmt.Errorf("an edge needs a src, a predicate and a dst")
	}
	var raw any
	if props != nil {
		b, err := json.Marshal(props)
		if err != nil {
			return err
		}
		raw = string(b)
	}
	_, err := s.db.Exec(
		`INSERT INTO edges(src,predicate,dst,props,surface,seen_at) VALUES(?,?,?,?,?,?)
		 ON CONFLICT(src,predicate,dst) DO UPDATE SET
		   props=COALESCE(excluded.props, edges.props),
		   surface=excluded.surface,
		   seen_at=MAX(excluded.seen_at, edges.seen_at)`,
		src, predicate, dst, raw, surface, seenAt.Unix())
	return err
}

// Edge is one stored relationship.
type Edge struct {
	Src       string          `json:"src"`
	Predicate string          `json:"predicate"`
	Dst       string          `json:"dst"`
	Props     json.RawMessage `json:"props,omitempty"`
	Surface   string          `json:"surface"`
	SeenAt    time.Time       `json:"seen_at"`
}

// Edges reads the edges out of or into a node.
//
// Both directions, because the useful questions run each way: what did this
// author write, and who wrote this book.
func (s *Store) Edges(uri string, incoming bool) ([]Edge, error) {
	col := "src"
	if incoming {
		col = "dst"
	}
	rows, err := s.db.Query(`SELECT src,predicate,dst,props,surface,seen_at FROM edges WHERE `+col+`=? ORDER BY predicate, dst`, uri)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []Edge
	for rows.Next() {
		var e Edge
		var props sql.NullString
		var at int64
		if err := rows.Scan(&e.Src, &e.Predicate, &e.Dst, &props, &e.Surface, &at); err != nil {
			return nil, err
		}
		if props.Valid {
			e.Props = json.RawMessage(props.String)
		}
		e.SeenAt = time.Unix(at, 0).UTC()
		out = append(out, e)
	}
	return out, rows.Err()
}

// GetNode reads one node's record back.
func (s *Store) GetNode(kind, uri string) ([]byte, error) {
	table, err := tableFor(kind)
	if err != nil {
		return nil, err
	}
	var raw string
	err = s.db.QueryRow(`SELECT json FROM `+table+` WHERE uri=?`, uri).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return []byte(raw), nil
}

// LookupIdentifier answers "I have an ISBN13 or an ASIN, what book is it" from
// the local store, without a request.
//
// The whole reason the two columns are indexed. Site search is disallowed and
// this is the replacement, so it has to be fast enough that nobody reaches for
// the disallowed thing out of impatience.
func (s *Store) LookupIdentifier(id string) ([]Hit, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, nil
	}
	rows, err := s.db.Query(
		`SELECT uri, legacy_id, title FROM books WHERE isbn13=? OR asin=? ORDER BY uri`, id, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []Hit
	for rows.Next() {
		var uri, title string
		var legacy sql.NullInt64
		if err := rows.Scan(&uri, &legacy, &title); err != nil {
			return nil, err
		}
		h := Hit{Kind: "book", ID: uri, Title: title}
		if legacy.Valid && legacy.Int64 > 0 {
			h.URL = BookURL(strconv.FormatInt(legacy.Int64, 10))
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// FindText is the full text search, ranked.
//
// Different from Search in store.go, which is a LIKE over the stored JSON and
// says so. This one stems, ranks, and looks at the three fields worth looking
// at, which is what `goodread find` promises.
func (s *Store) FindText(kind, query string, limit int) ([]Hit, error) {
	if limit <= 0 {
		limit = 50
	}
	q := `SELECT uri, kind, title FROM search WHERE search MATCH ?`
	args := []any{ftsQuery(query)}
	if kind != "" {
		q += ` AND kind=?`
		args = append(args, kind)
	}
	q += ` ORDER BY rank LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []Hit
	for rows.Next() {
		var h Hit
		if err := rows.Scan(&h.ID, &h.Kind, &h.Title); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// ftsQuery quotes what the user typed.
//
// fts5 reads bare input as its own query language, so an apostrophe or a
// stray hyphen in a book title comes back as a syntax error rather than as a
// search. Each word is quoted and the words are ANDed, which is what somebody
// typing two words means.
func ftsQuery(s string) string {
	var terms []string
	for _, f := range strings.Fields(s) {
		f = strings.ReplaceAll(f, `"`, "")
		if f != "" {
			terms = append(terms, `"`+f+`"`)
		}
	}
	if len(terms) == 0 {
		return `""`
	}
	return strings.Join(terms, " AND ")
}

// MergeRecords merges a newer record into an older one, field by field.
//
// Newest non absent wins, and absent is the operative word. A quick read
// returns a record with no editions key at all, which is not the same as a
// record saying the editions list is empty, and the whole point of the four
// states in 03_model.md section 8 is that those two stay apart here.
//
// The rules, in the order they apply:
//
//   - a key the new record does not have keeps the old value
//   - a key the new record has as null keeps the old value, since null is how
//     json spells "I did not look"
//   - two objects merge recursively, so a stats block with only an average in
//     it does not wipe the histogram underneath
//   - an empty array loses to a non empty one, because a page that rendered no
//     rows and a page that was not asked for rows look identical afterwards
//   - anything else, the new value wins
func MergeRecords(oldJSON, newJSON []byte) ([]byte, error) {
	var oldV, newV any
	if err := json.Unmarshal(oldJSON, &oldV); err != nil {
		return nil, fmt.Errorf("stored record does not parse: %w", err)
	}
	if err := json.Unmarshal(newJSON, &newV); err != nil {
		return nil, fmt.Errorf("new record does not parse: %w", err)
	}
	return json.Marshal(mergeValue(oldV, newV))
}

func mergeValue(old, new any) any {
	if new == nil {
		return old
	}
	oldMap, oldOK := old.(map[string]any)
	newMap, newOK := new.(map[string]any)
	if oldOK && newOK {
		out := make(map[string]any, len(oldMap)+len(newMap))
		for k, v := range oldMap {
			out[k] = v
		}
		for k, v := range newMap {
			if prev, had := out[k]; had {
				out[k] = mergeValue(prev, v)
				continue
			}
			out[k] = v
		}
		return out
	}
	if newArr, ok := new.([]any); ok {
		if len(newArr) == 0 {
			// The one asymmetric rule. An empty list is what both "there are
			// none" and "I did not ask" serialise to, so the longer answer
			// stands rather than the newer one.
			return old
		}
		return newArr
	}
	if s, ok := new.(string); ok && s == "" {
		return old
	}
	return new
}

// mergeSurfaces keeps the union of every surface a record has been read from.
//
// A book read from its own page and then seen again in a Listopia row was read
// from two surfaces, and the record should say both rather than whichever was
// most recent.
func mergeSurfaces(oldJSON []byte, add []string) []string {
	seen := map[string]bool{}
	var out []string
	if len(oldJSON) > 0 {
		var env struct {
			Surfaces []string `json:"surfaces"`
		}
		if json.Unmarshal(oldJSON, &env) == nil {
			for _, s := range env.Surfaces {
				if !seen[s] {
					seen[s] = true
					out = append(out, s)
				}
			}
		}
	}
	for _, s := range add {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

// setKey replaces one top level key in a record without touching the rest.
//
// Through a map rather than through the struct, because the store holds records
// of nine kinds and only ever sees them as json. Round tripping through a typed
// value here would quietly drop every field the Go type does not know about,
// which is the one thing the whole extra map exists to prevent.
func setKey(record []byte, key string, value json.RawMessage) ([]byte, error) {
	m := map[string]json.RawMessage{}
	if len(record) > 0 {
		if err := json.Unmarshal(record, &m); err != nil {
			return nil, err
		}
	}
	m[key] = value
	return json.Marshal(m)
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullIfZero(n int64) any {
	if n == 0 {
		return nil
	}
	return n
}
