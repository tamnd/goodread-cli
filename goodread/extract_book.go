package goodread

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ExtractBook reads a book page into fields, walking the ladder.
//
// Level 1 answers almost everything here, because the book page is the one
// Next.js route on the site. Level 2 runs anyway, both as a cross check and so
// that the day the framework goes the tool degrades rather than stops.
func ExtractBook(body []byte) (*Extractor, error) {
	e := NewExtractor("s1")

	if nd, err := nextData(body); err == nil {
		if cache, err := nd.ApolloState(); err == nil {
			e.set("build_id", LevelNextData, nd.BuildID)
			e.fromApollo(cache)
		}
	}
	e.fromMeta(body)

	if len(e.Fields) == 0 {
		return e, fmt.Errorf("no book data on this page")
	}
	return e, nil
}

// fromApollo is level 1.
func (e *Extractor) fromApollo(cache Apollo) {
	key, ok := cache.RootRefKey("getBookByLegacyId")
	if !ok {
		return
	}
	book, ok := cache.Entity(key)
	if !ok {
		return
	}

	e.set("id", LevelNextData, strOf(book["id"]))
	if n, ok := LegacyID(book); ok {
		e.set("legacy_id", LevelNextData, n)
	}
	e.set("title", LevelNextData, strOf(book["title"]))
	e.set("title_complete", LevelNextData, strOf(book["titleComplete"]))
	e.set("web_url", LevelNextData, strOf(book["webUrl"]))
	e.set("image_url", LevelNextData, strOf(book["imageUrl"]))

	// Two description keys, one raw with markup and one stripped. Both kept,
	// because the stripped one is what a text pipeline wants and the raw one is
	// what a renderer wants, and deriving either from the other is lossy.
	for field, raw := range book {
		fk := ParseFieldKey(field)
		if fk.Name != "description" {
			continue
		}
		if len(fk.Args) == 0 {
			e.set("description_html", LevelNextData, strOf(raw))
			continue
		}
		if strings.Contains(string(fk.Args), `"stripped":true`) {
			e.set("description", LevelNextData, strOf(raw))
		}
	}

	if d, ok := decode[bookDetails](book["details"]); ok {
		e.set("isbn", LevelNextData, d.ISBN)
		e.set("isbn13", LevelNextData, d.ISBN13)
		e.set("asin", LevelNextData, d.ASIN)
		e.set("format", LevelNextData, d.Format)
		e.set("publisher", LevelNextData, d.Publisher)
		e.set("language", LevelNextData, d.Language.Name)
		if d.NumPages > 0 {
			e.set("num_pages", LevelNextData, d.NumPages)
		}
		// publicationTime is epoch milliseconds. Kept as milliseconds rather
		// than converted here, so the model layer decides the representation
		// once instead of every extractor deciding it again.
		if d.PublicationTime != 0 {
			e.set("published_at_ms", LevelNextData, d.PublicationTime)
		}
	}

	e.contributorsFrom(cache, book)
	e.workFrom(cache, book)
	e.set("genres", LevelNextData, genresFrom(cache, book["bookGenres"]))
}

type bookDetails struct {
	ASIN            string `json:"asin"`
	ISBN            string `json:"isbn"`
	ISBN13          string `json:"isbn13"`
	Format          string `json:"format"`
	NumPages        int    `json:"numPages"`
	Publisher       string `json:"publisher"`
	PublicationTime int64  `json:"publicationTime"`
	Language        struct {
		Name string `json:"name"`
	} `json:"language"`
}

// contributorsFrom keeps the role rather than flattening to an author list.
//
// primaryContributorEdge and secondaryContributorEdges each carry a role, so
// author, illustrator and translator stay distinguishable. v0.2.0 flattened
// them, which is why it could not tell you who drew a book.
func (e *Extractor) contributorsFrom(cache Apollo, book map[string]json.RawMessage) {
	type contributor struct {
		Role string `json:"role"`
		Name string `json:"name"`
		ID   string `json:"id"`
		URL  string `json:"web_url"`
	}
	var out []contributor

	add := func(raw json.RawMessage) {
		if len(raw) == 0 {
			return
		}
		var edge struct {
			Role string          `json:"role"`
			Node json.RawMessage `json:"node"`
		}
		if err := json.Unmarshal(raw, &edge); err != nil {
			return
		}
		node, ok := cache.Resolve(edge.Node).(map[string]any)
		if !ok {
			return
		}
		c := contributor{Role: edge.Role}
		c.Name, _ = node["name"].(string)
		c.ID, _ = node["id"].(string)
		c.URL, _ = node["webUrl"].(string)
		if c.Name == "" {
			return
		}
		out = append(out, c)
	}

	add(book["primaryContributorEdge"])
	if raw, ok := book["secondaryContributorEdges"]; ok {
		var edges []json.RawMessage
		if err := json.Unmarshal(raw, &edges); err == nil {
			for _, edge := range edges {
				add(edge)
			}
		}
	}
	if len(out) > 0 {
		e.set("contributors", LevelNextData, out)
		e.set("author", LevelNextData, out[0].Name)
	}
}

// workFrom reads the work and its stats.
//
// The stats are the part v0.2.0 could not get, because ratingsCountDist is
// drawn as a bar chart and never written out. A mean alone hides the shape, and
// a 4.35 from a flat spread is a different book from a 4.35 with a spike at
// five.
func (e *Extractor) workFrom(cache Apollo, book map[string]json.RawMessage) {
	resolved, ok := cache.Resolve(book["work"]).(map[string]any)
	if !ok {
		return
	}
	if key, ok := resolved["__key"].(string); ok {
		e.set("work_key", LevelNextData, key)
	}
	if id, ok := resolved["id"].(string); ok {
		e.set("work_id", LevelNextData, id)
	}
	if n, ok := resolved["legacyId"].(float64); ok {
		e.set("work_legacy_id", LevelNextData, int(n))
	}

	if stats, ok := resolved["stats"].(map[string]any); ok {
		setNum(e, "average_rating", stats["averageRating"])
		setNum(e, "ratings_count", stats["ratingsCount"])
		setNum(e, "text_reviews_count", stats["textReviewsCount"])
		if dist, ok := stats["ratingsCountDist"].([]any); ok && len(dist) > 0 {
			out := make([]int, 0, len(dist))
			for _, v := range dist {
				n, _ := v.(float64)
				out = append(out, int(n))
			}
			e.set("ratings_dist", LevelNextData, out)
		}
		if langs, ok := stats["textReviewsLanguageCounts"]; ok {
			e.set("text_reviews_by_language", LevelNextData, langs)
		}
	}

	if details, ok := resolved["details"].(map[string]any); ok {
		if s, ok := details["originalTitle"].(string); ok {
			e.set("original_title", LevelNextData, s)
		}
		// places and characters are curated lists nothing else in the family
		// has, and they cost nothing extra to keep. A graph that can answer
		// "which books are set in Dublin" is worth having.
		e.set("places", LevelNextData, details["places"])
		e.set("characters", LevelNextData, details["characters"])
		e.set("awards", LevelNextData, details["awardsWon"])
	}

	// The page asks for a sample of the quotes, and the sample size is in the
	// field key. Reading it out means the missed sentence stays true when
	// Goodreads changes the number.
	work, ok := cache.Entity(strOfAny(resolved["__key"]))
	if !ok {
		return
	}
	legacy := e.Fields["work_legacy_id"]
	for field := range work {
		fk := ParseFieldKey(field)
		if n, has := fk.Limit(); has && n > 0 {
			switch fk.Name {
			case "quotes":
				e.Miss("the book page carries %d quote of the work's set. `goodread quotes %v` reads /work/quotes for all of them.", n, legacy)
			case "questions", "topics":
				e.Miss("the book page carries %d %s of the work's set.", n, fk.Name)
			}
		}
	}
	if n, ok := e.Fields["text_reviews_count"].(int); ok && n > 0 {
		e.Miss("the book page samples the reviews. it carries the page's sample of %d text reviews.", n)
	}
}

// genresFrom flattens the bookGenres edge list to names.
func genresFrom(cache Apollo, raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	items, ok := cache.Resolve(raw).([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		g, ok := m["genre"].(map[string]any)
		if !ok {
			continue
		}
		if name, ok := g["name"].(string); ok && name != "" {
			out = append(out, name)
		}
	}
	return out
}

// fromMeta is level 2, ld+json and og:.
//
// It runs after level 1 and set is first-write-wins, so on a book page it fills
// only what the cache did not carry. That is the point: the fields it does fill
// show up in the report as level 2, which is how you find out the cache stopped
// carrying something.
func (e *Extractor) fromMeta(body []byte) {
	if ld, ok := parseLDJSON(body); ok {
		e.set("title", LevelMeta, TitleWithoutSeries(ld.Name))
		e.set("title_complete", LevelMeta, ld.Name)
		e.set("image_url", LevelMeta, ld.Image)
		e.set("format", LevelMeta, ld.BookFormat)
		e.set("language", LevelMeta, ld.InLanguage)
		e.set("isbn13", LevelMeta, ld.ISBN)
		if ld.NumberOfPages > 0 {
			e.set("num_pages", LevelMeta, ld.NumberOfPages)
		}
		if len(ld.Author) > 0 {
			e.set("author", LevelMeta, ld.Author[0].Name)
		}
		if r := ld.AggregateRating; r.RatingCount > 0 {
			e.set("average_rating", LevelMeta, r.RatingValue)
			e.set("ratings_count", LevelMeta, r.RatingCount)
			e.set("text_reviews_count", LevelMeta, r.ReviewCount)
		}
	}

	og := parseOpenGraph(body)
	e.set("canonical_url", LevelMeta, og["url"])
	e.set("image_url", LevelMeta, og["image"])
	e.set("title_complete", LevelMeta, og["title"])
	e.set("title", LevelMeta, TitleWithoutSeries(og["title"]))
}

// LevelTwo re-reads a page with level 2 only, so a test can compare the rungs.
//
// This is the cross check the ladder exists for. When ld+json and the Apollo
// cache disagree on a title or a rating count, one of them is being read wrong,
// and finding that out from a test beats finding it out from a record.
func LevelTwo(body []byte) *Extractor {
	e := NewExtractor("s1")
	e.fromMeta(body)
	return e
}

func decode[T any](raw json.RawMessage) (T, bool) {
	var v T
	if len(raw) == 0 {
		return v, false
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return v, false
	}
	return v, true
}

func strOf(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}

func strOfAny(v any) string {
	s, _ := v.(string)
	return s
}

func setNum(e *Extractor, field string, v any) {
	n, ok := v.(float64)
	if !ok {
		return
	}
	if n == float64(int(n)) {
		e.set(field, LevelNextData, int(n))
		return
	}
	e.set(field, LevelNextData, n)
}
