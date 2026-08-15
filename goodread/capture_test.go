package goodread

import (
	"bufio"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Capture is one line of the golden ledger.
type Capture struct {
	File     string
	Surface  string
	Captured string
	URL      string
	Why      string
}

const capturesDir = "testdata/pages"

// loadCaptures reads the ledger.
func loadCaptures(t *testing.T) []Capture {
	t.Helper()
	f, err := os.Open(filepath.Join(capturesDir, "capture.txt"))
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	defer func() { _ = f.Close() }()

	var out []Capture
	sc := bufio.NewScanner(f)
	for line := 1; sc.Scan(); line++ {
		text := strings.TrimRight(sc.Text(), " \t")
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		parts := strings.Split(text, "\t")
		if len(parts) != 5 {
			t.Fatalf("ledger line %d has %d tab separated fields, want 5: %q", line, len(parts), text)
		}
		out = append(out, Capture{parts[0], parts[1], parts[2], parts[3], parts[4]})
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	return out
}

// readCapture opens a capture through gzip.
func readCapture(t *testing.T, name string) []byte {
	t.Helper()
	f, err := os.Open(filepath.Join(capturesDir, name))
	if err != nil {
		t.Fatalf("open %s: %v", name, err)
	}
	defer func() { _ = f.Close() }()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gunzip %s: %v", name, err)
	}
	defer func() { _ = zr.Close() }()
	body, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return body
}

// TestCaptureLedger holds the rule that every capture says where it came from.
//
// Carried over from arxiv-cli, where it caught a real problem: a file that had
// been hand-edited to make a test pass, with nothing recording that it was no
// longer what the site served.
func TestCaptureLedger(t *testing.T) {
	entries, err := os.ReadDir(capturesDir)
	if err != nil {
		t.Fatalf("read %s: %v", capturesDir, err)
	}
	onDisk := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() || e.Name() == "capture.txt" {
			continue
		}
		onDisk[e.Name()] = true
	}

	listed := map[string]bool{}
	for _, c := range loadCaptures(t) {
		listed[c.File] = true
		if !onDisk[c.File] {
			t.Errorf("ledger lists %s, which is not in %s", c.File, capturesDir)
		}
		if !strings.HasSuffix(c.File, ".gz") {
			t.Errorf("%s is not gzipped: a book page is a megabyte", c.File)
		}
		if _, ok := LookupSurface(c.Surface); !ok {
			t.Errorf("%s claims surface %q, which is not registered", c.File, c.Surface)
		}
		if _, err := time.Parse("2006-01-02", c.Captured); err != nil {
			t.Errorf("%s has capture date %q, want YYYY-MM-DD", c.File, c.Captured)
		}
		if !strings.HasPrefix(c.URL, "https://www.goodreads.com/") {
			t.Errorf("%s has url %q, which is not a goodreads url", c.File, c.URL)
		}
		if strings.TrimSpace(c.Why) == "" {
			t.Errorf("%s has no reason recorded, so nobody can tell what it is for", c.File)
		}
	}
	for name := range onDisk {
		if !listed[name] {
			t.Errorf("%s is in %s but not in the ledger, so nothing says where it came from", name, capturesDir)
		}
	}
}

// TestCapturesOpen reads every capture, so a truncated or wrongly compressed
// file fails here rather than inside whichever test happens to touch it first.
func TestCapturesOpen(t *testing.T) {
	for _, c := range loadCaptures(t) {
		body := readCapture(t, c.File)
		if len(body) < 1024 {
			t.Errorf("%s is %d bytes, which is too small to be a real page", c.File, len(body))
		}
		if !strings.Contains(strings.ToLower(string(body[:min(2048, len(body))])), "<html") {
			t.Errorf("%s does not look like HTML", c.File)
		}
	}
}
