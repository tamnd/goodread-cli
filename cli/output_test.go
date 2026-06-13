package cli

import (
	"bytes"
	"strings"
	"testing"
)

type sample struct {
	BookID string   `json:"book_id"`
	Title  string   `json:"title"`
	Tags   []string `json:"tags"`
	URL    string   `json:"url"`
}

func render(t *testing.T, format Format, fields []string, noHeader bool, tmpl string, recs any) string {
	t.Helper()
	var buf bytes.Buffer
	if err := NewRenderer(&buf, format, fields, noHeader, tmpl).Render(recs); err != nil {
		t.Fatalf("Render(%s): %v", format, err)
	}
	return buf.String()
}

func TestRenderJSONL(t *testing.T) {
	recs := []sample{
		{BookID: "1", Title: "A", Tags: []string{"x"}, URL: "u1"},
		{BookID: "2", Title: "B", URL: "u2"},
	}
	out := render(t, FormatJSONL, nil, false, "", recs)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %q", len(lines), out)
	}
	if !strings.Contains(lines[0], `"book_id":"1"`) {
		t.Errorf("line 0 = %q", lines[0])
	}
}

func TestRenderCSVFields(t *testing.T) {
	recs := []sample{{BookID: "1", Title: "A", URL: "u1"}}
	out := render(t, FormatCSV, []string{"book_id", "title"}, false, "", recs)
	want := "book_id,title\n1,A\n"
	if out != want {
		t.Errorf("CSV = %q, want %q", out, want)
	}
}

func TestRenderCSVNoHeader(t *testing.T) {
	recs := []sample{{BookID: "1", Title: "A"}}
	out := render(t, FormatCSV, []string{"book_id", "title"}, true, "", recs)
	if out != "1,A\n" {
		t.Errorf("CSV no-header = %q", out)
	}
}

func TestRenderURL(t *testing.T) {
	recs := []sample{{URL: "u1"}, {URL: ""}, {URL: "u3"}}
	out := render(t, FormatURL, nil, false, "", recs)
	if out != "u1\nu3\n" {
		t.Errorf("URL = %q, want u1\\nu3\\n", out)
	}
}

func TestRenderTemplate(t *testing.T) {
	recs := []sample{{Title: "A", Tags: []string{"x", "y"}}}
	out := render(t, FormatTable, nil, false, `{{.Title}}: {{join "," .Tags}}`, recs)
	if strings.TrimSpace(out) != "A: x,y" {
		t.Errorf("template = %q", out)
	}
}

func TestRenderSingleStruct(t *testing.T) {
	// A non-slice record is wrapped into a one-element slice.
	out := render(t, FormatJSONL, nil, false, "", sample{BookID: "9", Title: "Z"})
	if !strings.Contains(out, `"book_id":"9"`) {
		t.Errorf("single struct = %q", out)
	}
}
