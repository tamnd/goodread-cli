package goodread

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// Op is one thing this tool knows how to fetch.
//
// Every readable path is registered here and nothing builds a URL at the call
// site. That is the whole point: the robots test walks this list and checks
// each entry against the live rules, and a URL built inline is a path nothing
// tests. v0.2.0 fetched three disallowed paths for two releases without
// anybody noticing, and it did so because the URLs were built where they were
// used.
type Op struct {
	Surface string // s1..s13, as numbered in the spec
	Name    string // the noun a user types
	Path    func(args ...string) string

	// Disallowed records that this op is known to need --no-robots.
	//
	// It is a declaration, not the enforcement. Enforcement is the live check
	// at request time, because the site owns the rules and can change them.
	// The field exists so the CLI can refuse before spending a request, and so
	// a test can assert the declaration still matches what the rules say. When
	// the two disagree the test fails and somebody looks, which is exactly the
	// alarm worth having.
	Disallowed bool

	// Alt names the allowed op a user should reach for instead, so the refusal
	// message can offer something rather than just saying no.
	Alt string
}

// URL renders the op's absolute URL.
func (o Op) URL(args ...string) string { return BaseURL + o.Path(args...) }

// Ops is every read the tool can perform.
//
// s1 through s9 are allowed and are read by default. s10 through s13 are
// disallowed by Goodreads' robots.txt and are only read with --no-robots. The
// surface id lands in a record's provenance, so a store can be filtered on
// which side of that line its records came from without knowing anything about
// the flag.
var Ops = []Op{
	{Surface: "s1", Name: "book", Path: func(a ...string) string {
		return "/book/show/" + numericPrefix(arg(a, 0))
	}},
	{Surface: "s2", Name: "author", Path: func(a ...string) string {
		return "/author/show/" + numericPrefix(arg(a, 0))
	}},
	{Surface: "s3", Name: "series", Path: func(a ...string) string {
		return "/series/" + numericPrefix(arg(a, 0))
	}},
	{Surface: "s4", Name: "list", Path: func(a ...string) string {
		return "/list/show/" + arg(a, 0)
	}},
	{Surface: "s4", Name: "list_tag", Path: func(a ...string) string {
		return "/list/tag/" + arg(a, 0)
	}},
	{Surface: "s5", Name: "genre", Path: func(a ...string) string {
		return "/genres/" + arg(a, 0)
	}},
	{Surface: "s6", Name: "editions", Path: func(a ...string) string {
		return "/work/editions/" + numericPrefix(arg(a, 0))
	}},
	{Surface: "s7", Name: "quotes", Path: func(a ...string) string {
		return "/work/quotes/" + numericPrefix(arg(a, 0))
	}},
	{Surface: "s8", Name: "siteindex", Path: func(a ...string) string {
		return "/siteindex." + arg(a, 0) + ".xml"
	}},
	{Surface: "s9", Name: "robots", Path: func(a ...string) string {
		return "/robots.txt"
	}},

	// s2b. The author's own quote page, distinct from a work's quotes.
	{Surface: "s2", Name: "author_quotes", Path: func(a ...string) string {
		return "/author/quotes/" + numericPrefix(arg(a, 0))
	}},

	// Autocomplete is public JSON, carries no key, and is not disallowed.
	// It is the allowed search route, which is why `goodread search` works
	// without the override. See suggest.go for what it actually returns.
	{Surface: "s14", Name: "suggest", Path: func(a ...string) string {
		v := url.Values{}
		v.Set("format", "json")
		v.Set("q", arg(a, 0))
		return "/book/auto_complete?" + v.Encode()
	}},

	// Disallowed from here down.
	// The args are query, page, search_type and field, in that order. The last
	// two are optional and default to a books search over every field, which is
	// what the site does with them missing.
	{Surface: "s10", Name: "search", Disallowed: true, Alt: "suggest", Path: func(a ...string) string {
		page := 0
		if p := arg(a, 1); p != "" {
			page = int(commaFreeInt(p))
		}
		q := SearchQuery{Query: arg(a, 0), Page: page, Type: arg(a, 2), Field: arg(a, 3)}
		return strings.TrimPrefix(SearchURL(q), BaseURL)
	}},
	{Surface: "s11", Name: "shelf", Disallowed: true, Path: func(a ...string) string {
		v := url.Values{}
		v.Set("shelf", arg(a, 1))
		if p := arg(a, 2); p != "" && p != "1" {
			v.Set("page", p)
		}
		return "/review/list/" + numericPrefix(arg(a, 0)) + "?" + v.Encode()
	}},
	{Surface: "s11", Name: "shelf_rss", Disallowed: true, Path: func(a ...string) string {
		v := url.Values{}
		v.Set("shelf", arg(a, 1))
		return "/review/list_rss/" + numericPrefix(arg(a, 0)) + "?" + v.Encode()
	}},
	{Surface: "s12", Name: "review", Disallowed: true, Path: func(a ...string) string {
		return "/review/show/" + numericPrefix(arg(a, 0))
	}},
	{Surface: "s12", Name: "reviews", Disallowed: true, Path: func(a ...string) string {
		v := url.Values{}
		if p := arg(a, 1); p != "" && p != "1" {
			v.Set("page", p)
		}
		u := "/book/reviews/" + numericPrefix(arg(a, 0))
		if len(v) > 0 {
			u += "?" + v.Encode()
		}
		return u
	}},
	{Surface: "s13", Name: "work", Disallowed: true, Alt: "book", Path: func(a ...string) string {
		return "/work/" + numericPrefix(arg(a, 0))
	}},
}

func arg(a []string, i int) string {
	if i < len(a) {
		return a[i]
	}
	return ""
}

// LookupOp finds an op by name.
func LookupOp(name string) (Op, bool) {
	for _, o := range Ops {
		if o.Name == name {
			return o, true
		}
	}
	return Op{}, false
}

// LookupSurface finds the first op on a surface, so a surface id can be
// validated and named without the caller knowing which ops share it.
func LookupSurface(surface string) (Op, bool) {
	for _, o := range Ops {
		if o.Surface == surface {
			return o, true
		}
	}
	return Op{}, false
}

// SampleArgs are stand-in arguments per op, used by the policy tests and by
// `goodread robots` so both can render a real path for every registered op
// without inventing one at each call site.
var SampleArgs = map[string][]string{
	"book":          {"2767052"},
	"author":        {"153394"},
	"author_quotes": {"153394"},
	"series":        {"73758"},
	"list":          {"1.Best_Books_Ever"},
	"list_tag":      {"fantasy"},
	"genre":         {"fantasy"},
	"editions":      {"2792775"},
	"quotes":        {"2792775"},
	"siteindex":     {"author"},
	"robots":        {},
	"suggest":       {"hunger games"},
	"search":        {"hunger games", "1"},
	"shelf":         {"1", "read", "1"},
	"shelf_rss":     {"1", "read"},
	"review":        {"2892457"},
	"reviews":       {"2767052", "1"},
	"work":          {"2792775"},
}

// SamplePath renders an op's path using its sample arguments.
func SamplePath(o Op) string { return o.Path(SampleArgs[o.Name]...) }

// Surfaces lists the distinct surface ids in order.
func Surfaces() []string {
	seen := map[string]bool{}
	var out []string
	for _, o := range Ops {
		if !seen[o.Surface] {
			seen[o.Surface] = true
			out = append(out, o.Surface)
		}
	}
	sort.Slice(out, func(i, j int) bool { return surfaceNum(out[i]) < surfaceNum(out[j]) })
	return out
}

func surfaceNum(s string) int {
	n := 0
	for _, c := range strings.TrimPrefix(s, "s") {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// DisallowedError is returned when an op needs --no-robots and did not get it.
//
// It carries the rule that matched so the message can quote the site's own
// words, and the alternative so it can offer something instead of just
// refusing. A refusal that names neither is a refusal the user cannot act on.
type DisallowedError struct {
	Op     string
	Path   string
	Rule   Rule
	Source string
	Alt    string
}

func (e *DisallowedError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s is disallowed by goodreads robots.txt", pathOnly(e.Path))
	fmt.Fprintf(&b, "\n\n  rule       %s\n  source     %s", e.Rule.String(), e.Source)
	b.WriteString("\n\nrun again with --no-robots to do it")
	if e.Alt != "" {
		fmt.Fprintf(&b, ", or use `goodread %s` which is allowed", e.Alt)
	}
	b.WriteString(". see `goodread robots`.")
	return b.String()
}

func (e *DisallowedError) Unwrap() error { return ErrDisallowed }

func pathOnly(p string) string {
	if i := strings.IndexByte(p, '?'); i >= 0 {
		return p[:i]
	}
	return p
}
