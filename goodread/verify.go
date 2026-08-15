package goodread

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// VerifyReport is one capture's extraction result.
type VerifyReport struct {
	Capture  string   `json:"capture"`
	Surface  string   `json:"surface"`
	URL      string   `json:"url"`
	Captured string   `json:"captured"`
	Fields   int      `json:"fields"`
	Level1   int      `json:"level1"`
	Level2   int      `json:"level2"`
	Level3   int      `json:"level3"`
	Missing  []string `json:"missing,omitempty"`
	Err      string   `json:"error,omitempty"`
}

// expectedBookFields is what a book page has to keep yielding.
//
// Pinned from the 2026-08-15 captures. A field vanishing from this list's
// results is Goodreads having stopped shipping something, which is more urgent
// than a new field appearing and gets reported on its own line.
var expectedBookFields = []string{
	"title", "author", "legacy_id", "id", "work_id", "description",
	"average_rating", "ratings_count", "ratings_dist", "num_pages",
	"image_url", "genres", "contributors",
}

// VerifyCaptures runs the extractor over every pinned capture.
//
// It reads the ledger rather than the directory, so a file somebody dropped in
// without recording where it came from is not silently treated as evidence.
func VerifyCaptures() ([]VerifyReport, error) {
	dir, err := capturesPath()
	if err != nil {
		return nil, err
	}
	entries, err := readLedger(filepath.Join(dir, "capture.txt"))
	if err != nil {
		return nil, err
	}

	var out []VerifyReport
	for _, c := range entries {
		r := VerifyReport{Capture: c.File, Surface: c.Surface, URL: c.URL, Captured: c.Captured}
		body, err := readGzip(filepath.Join(dir, c.File))
		if err != nil {
			r.Err = err.Error()
			out = append(out, r)
			continue
		}
		if c.Surface != "s1" {
			// Only the book page has an extractor so far. Reporting the others
			// as zero rather than skipping them keeps the gap visible.
			r.Err = "no extractor for this surface yet"
			out = append(out, r)
			continue
		}
		e, err := ExtractBook(body)
		if err != nil {
			r.Err = err.Error()
			out = append(out, r)
			continue
		}
		r.Fields = len(e.Fields)
		r.Level1 = e.Levels.Count(LevelNextData)
		r.Level2 = e.Levels.Count(LevelMeta)
		r.Level3 = e.Levels.Count(LevelSelector)
		for _, want := range expectedBookFields {
			if _, ok := e.Fields[want]; !ok {
				r.Missing = append(r.Missing, want)
			}
		}
		sort.Strings(r.Missing)
		out = append(out, r)
	}
	return out, nil
}

// ledgerEntry mirrors one line of capture.txt.
type ledgerEntry struct{ File, Surface, Captured, URL, Why string }

func readLedger(path string) ([]ledgerEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open capture ledger: %w", err)
	}
	defer func() { _ = f.Close() }()

	var out []ledgerEntry
	sc := bufio.NewScanner(f)
	for line := 1; sc.Scan(); line++ {
		text := strings.TrimRight(sc.Text(), " \t")
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		parts := strings.Split(text, "\t")
		if len(parts) != 5 {
			return nil, fmt.Errorf("%s:%d has %d fields, want 5", path, line, len(parts))
		}
		out = append(out, ledgerEntry{parts[0], parts[1], parts[2], parts[3], parts[4]})
	}
	return out, sc.Err()
}

func readGzip(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer func() { _ = zr.Close() }()
	return io.ReadAll(zr)
}

// capturesPath finds testdata/pages relative to this source file.
//
// Captures are test fixtures, not shipped data, so `goodread verify` only works
// from a checkout. That is the right trade: embedding fifteen megabytes of HTML
// in the binary to support a command developers run would be paying everybody
// for something almost nobody uses.
func capturesPath() (string, error) {
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("cannot locate the captures")
	}
	dir := filepath.Join(filepath.Dir(self), "testdata", "pages")
	if _, err := os.Stat(dir); err != nil {
		return "", fmt.Errorf("captures are only available from a source checkout: %w", err)
	}
	return dir, nil
}
