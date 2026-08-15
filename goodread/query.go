package goodread

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// SQL over the store.
//
// The store is a SQLite file and hiding that behind a query language of our own
// would be inventing a worse SQL. What this adds is the two things a person
// running SQL against their own crawl should not have to arrange themselves:
// the statement cannot write, and the results come back as rows the same
// renderer prints as everything else.

// QueryRow is one row of a query result.
//
// It carries its own column order because a SQL result set is not a struct and
// nothing can reflect over it. The renderer asks it for its columns and its
// cells rather than reading tags.
type QueryRow struct {
	cols []string
	vals map[string]any
}

// Columns is the order the statement asked for, which is the order it prints.
func (r QueryRow) Columns() []string { return append([]string(nil), r.cols...) }

// Cells is the row as display strings.
func (r QueryRow) Cells() map[string]string {
	out := make(map[string]string, len(r.vals))
	for k, v := range r.vals {
		out[k] = cellString(v)
	}
	return out
}

// Value reads one column, for a caller that wants the value and not the string.
func (r QueryRow) Value(col string) any { return r.vals[col] }

// MarshalJSON writes the row as an object.
func (r QueryRow) MarshalJSON() ([]byte, error) { return json.Marshal(r.vals) }

func cellString(v any) string {
	switch t := v.(type) {
	case nil:
		// Empty and not "NULL", because a cell that prints NULL is one that
		// sorts and greps as the four letter word rather than as nothing.
		return ""
	case []byte:
		return string(t)
	case string:
		return t
	case time.Time:
		return t.UTC().Format(time.RFC3339)
	default:
		return fmt.Sprint(t)
	}
}

// Query runs one read only statement.
//
// Two guards, and they are different in kind. The prefix check is the one that
// gives a person a sentence they can act on, and query_only on the connection
// is the one that is actually load bearing, because a statement can be spelled
// in more ways than a prefix check knows about.
func (s *Store) Query(ctx context.Context, statement string, limit int) ([]QueryRow, error) {
	q := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(statement), ";"))
	if q == "" {
		return nil, fmt.Errorf("no statement")
	}
	if strings.Contains(q, ";") {
		return nil, fmt.Errorf("one statement at a time, and this one has a semicolon in the middle of it")
	}
	head := strings.ToLower(strings.Fields(q)[0])
	if head != "select" && head != "with" && head != "explain" && head != "pragma" {
		return nil, fmt.Errorf("query reads, it does not write, so %q is not allowed. it takes a select, a with or a pragma", head)
	}

	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.ExecContext(ctx, `PRAGMA query_only=1`); err != nil {
		return nil, err
	}
	// Off again before the connection goes back to the pool, since the pragma
	// belongs to the connection and not to this call, and leaving it on would
	// make the next write on this handle fail somewhere unrelated.
	defer func() { _, _ = conn.ExecContext(context.Background(), `PRAGMA query_only=0`) }()

	rows, err := conn.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var out []QueryRow
	for rows.Next() {
		if limit > 0 && len(out) >= limit {
			break
		}
		cells := make([]any, len(cols))
		into := make([]any, len(cols))
		for i := range cells {
			into[i] = &cells[i]
		}
		if err := rows.Scan(into...); err != nil {
			return nil, err
		}
		vals := make(map[string]any, len(cols))
		for i, c := range cols {
			if b, ok := cells[i].([]byte); ok {
				vals[c] = string(b)
				continue
			}
			vals[c] = cells[i]
		}
		out = append(out, QueryRow{cols: cols, vals: vals})
	}
	return out, rows.Err()
}

// Tables lists what a person can query, which is the first thing anybody needs.
func (s *Store) Tables() ([]string, error) {
	rows, err := s.db.Query(`SELECT name FROM sqlite_master WHERE type IN ('table','view') AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}
