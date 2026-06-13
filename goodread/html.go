package goodread

import (
	"encoding/json"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var reTags = regexp.MustCompile(`<[^>]+>`)

// cleanHTML strips tags and unescapes the common HTML entities.
func cleanHTML(s string) string {
	s = reTags.ReplaceAllString(s, "")
	r := strings.NewReplacer(
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", `"`,
		"&#39;", "'",
		"&nbsp;", " ",
	)
	return strings.TrimSpace(r.Replace(s))
}

var reWS = regexp.MustCompile(`\s+`)

// cleanText strips tags, unescapes entities, and collapses interior whitespace.
// Use it for descriptions where the source markup carries stray newlines and
// indentation between inline elements.
func cleanText(s string) string {
	return strings.TrimSpace(reWS.ReplaceAllString(cleanHTML(s), " "))
}

var reCoverSize = regexp.MustCompile(`\._S[XY]\d+_`)

// fullCoverURL rewrites a Goodreads thumbnail URL to its full-size form by
// dropping the "._SX50_" / "._SY75_" size token. A URL without the token is
// returned unchanged.
func fullCoverURL(u string) string {
	return reCoverSize.ReplaceAllString(u, "")
}

// cleanBio strips the "edit data" affordance and collapses whitespace in a bio.
func cleanBio(s string) string {
	s = cleanHTML(s)
	s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "edit data"))
	return strings.TrimSpace(reWS.ReplaceAllString(s, " "))
}

// cleanQuote collapses whitespace and strips the trailing attribution dash and
// the surrounding smart quotes that wrap a Goodreads quote body.
func cleanQuote(s string) string {
	s = reWS.ReplaceAllString(cleanHTML(s), " ")
	s = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(s), "―-—"))
	return strings.TrimSpace(strings.Trim(s, "“”‘’\"'"))
}

// contains reports whether ss already holds s.
func contains(ss []string, s string) bool {
	return slices.Contains(ss, s)
}

// parseRawFloat parses a JSON value that may be a number or a quoted string.
func parseRawFloat(raw json.RawMessage) float64 {
	if len(raw) == 0 {
		return 0
	}
	var f float64
	if json.Unmarshal(raw, &f) == nil {
		return f
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		f, _ = strconv.ParseFloat(strings.ReplaceAll(s, ",", ""), 64)
	}
	return f
}

// parseRawInt64 parses a JSON value that may be a number or a quoted string.
func parseRawInt64(raw json.RawMessage) int64 {
	if len(raw) == 0 {
		return 0
	}
	var n int64
	if json.Unmarshal(raw, &n) == nil {
		return n
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		n, _ = strconv.ParseInt(strings.ReplaceAll(s, ",", ""), 10, 64)
	}
	return n
}

// atoiClean parses an integer from text that may carry commas and stray runes.
func atoiClean(s string) int {
	s = strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
	n, _ := strconv.Atoi(s)
	return n
}
