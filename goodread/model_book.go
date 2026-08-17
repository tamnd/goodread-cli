package goodread

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// BookFrom turns an extraction into a record.
//
// The mapping is deliberately dumb and total. Every field the extractor found
// lands somewhere or is reported as unknown, and nothing is invented on the way
// through. Anything clever belongs in the extractor, where the page is still
// there to check it against.
func BookFrom(e *Extractor, retrievedAt time.Time) (*Book, error) {
	if e == nil {
		return nil, fmt.Errorf("no extraction")
	}
	b := &Book{}
	b.Kind = "book"
	b.RetrievedAt = retrievedAt
	b.Surfaces = []string{e.Surface}
	b.Sources = []string{SurfaceSource(e.Surface)}
	b.Level = Levels{}
	for k, v := range e.Levels {
		b.Level[k] = v
	}
	b.Missed = append(b.Missed, e.Missed...)
	b.Via = viaOf(e)
	b.BuildID, _ = e.Fields["build_id"].(string)

	b.ID, _ = e.Fields["id"].(string)
	b.LegacyID, _ = int64Of(e.Fields["legacy_id"])
	b.WebURL = firstString(e, "web_url", "canonical_url")

	b.Title, _ = e.Fields["title"].(string)
	b.TitleComplete, _ = e.Fields["title_complete"].(string)
	b.Description, _ = e.Fields["description_html"].(string)
	b.DescriptionStripped, _ = e.Fields["description"].(string)

	b.ISBN, _ = e.Fields["isbn"].(string)
	b.ISBN13, _ = e.Fields["isbn13"].(string)
	b.ASIN, _ = e.Fields["asin"].(string)
	b.Format, _ = e.Fields["format"].(string)
	b.Publisher, _ = e.Fields["publisher"].(string)
	b.Language, _ = e.Fields["language"].(string)
	b.ImageURL, _ = e.Fields["image_url"].(string)

	if n, ok := int64Of(e.Fields["num_pages"]); ok {
		pages := int(n)
		b.NumPages = &pages
	}
	if ms, ok := int64Of(e.Fields["published_at_ms"]); ok {
		b.PublicationTime = timeFromMillis(ms)
	}

	if raw, ok := e.Fields["links"].(json.RawMessage); ok {
		b.Links = raw
	}

	if id, ok := e.Fields["work_id"].(string); ok && id != "" {
		key, _ := e.Fields["work_key"].(string)
		typ, _ := SplitCacheKey(key)
		if typ == "" {
			typ = "Work"
		}
		b.Work = &Ref{Type: typ, ID: id, Key: key, Resolved: true}
	}

	b.Extra, _ = e.Fields["extra"].(map[string]json.RawMessage)
	b.Genres = genreRefsFrom(e.Fields["genres"])
	b.Contributors = contributorsFromFields(e)
	b.Series = seriesFromFields(e)
	b.Stats = statsFrom(e)
	b.Reviews, b.Shelvings = reviewsFromFields(e)

	return b, nil
}

// WorkFrom turns the work half of a book page extraction into a record.
//
// The book page carries a partly filled work: stats, the curated lists, and
// pointers to the editions and quotes pages. That is enough to be worth
// emitting and not enough to pretend it is a full work read, which is what
// Surfaces and the missed sentences are for.
func WorkFrom(e *Extractor, retrievedAt time.Time) (*Work, bool) {
	id, _ := e.Fields["work_id"].(string)
	legacy, hasLegacy := int64Of(e.Fields["work_legacy_id"])
	if id == "" && !hasLegacy {
		return nil, false
	}

	w := &Work{ID: id, LegacyID: legacy}
	w.Kind = "work"
	w.RetrievedAt = retrievedAt
	w.Surfaces = []string{e.Surface}
	w.Sources = []string{SurfaceSource(e.Surface)}
	w.Missed = append(w.Missed, e.Missed...)
	w.BuildID, _ = e.Fields["build_id"].(string)
	w.Via = viaOf(e)
	w.Level = Levels{}
	for k, v := range e.Levels {
		w.Level[k] = v
	}

	w.WebURL, _ = e.Fields["work_web_url"].(string)
	w.ShelvesURL, _ = e.Fields["work_shelves_url"].(string)
	w.OriginalTitle, _ = e.Fields["original_title"].(string)
	if ms, ok := int64Of(e.Fields["work_published_at_ms"]); ok {
		w.PublicationTime = timeFromMillis(ms)
	}

	w.Extra, _ = e.Fields["work_extra"].(map[string]json.RawMessage)
	w.Stats = statsFrom(e)
	w.AwardsWon = awardsFrom(e.Fields["awards"])
	w.Places = placesFrom(e.Fields["places"])
	w.Characters = charactersFrom(e.Fields["characters"])
	w.ChoiceAwards = choiceAwardsFrom(e.Fields["choice_awards"])

	if url, ok := e.Fields["editions_url"].(string); ok && url != "" {
		c := &Conn{NextURL: url}
		if n, ok := int64Of(e.Fields["editions_count"]); ok {
			c.TotalCount = &n
		}
		// Nothing loaded. The book page names the editions page and does not
		// carry a single edition off it, and saying so is the whole reason Conn
		// reports Complete rather than just a list.
		w.Editions = c
	}
	return w, true
}

// statsFrom builds the stats block, or nothing if the page carried no numbers.
func statsFrom(e *Extractor) *Stats {
	s := &Stats{Via: e.Surface}
	found := false

	if v, ok := floatOf(e.Fields["average_rating"]); ok {
		s.AverageRating = &v
		found = true
	}
	if v, ok := int64Of(e.Fields["ratings_count"]); ok {
		s.RatingsCount = &v
		found = true
	}
	if v, ok := int64Of(e.Fields["text_reviews_count"]); ok {
		s.TextReviewsCount = &v
		found = true
	}
	if dist, ok := e.Fields["ratings_dist"].([]int); ok && len(dist) > 0 {
		s.RatingsCountDist = make([]int64, len(dist))
		for i, n := range dist {
			s.RatingsCountDist[i] = int64(n)
		}
		found = true
	}
	if langs := languageCountsFrom(e.Fields["text_reviews_by_language"]); len(langs) > 0 {
		s.TextReviewsByLanguage = langs
		found = true
	}
	if !found {
		return nil
	}
	return s
}

// StatsCheck is what `goodread book <id> --check` reports.
//
// Two reconciliations, both of which catch a class of error that reads as
// plausible otherwise. The histogram has to sum to the ratings count, which
// catches a truncated or misaligned distribution. The mean derived from the
// buckets has to match the published average, which catches a reversed slice:
// five integers with no labels are exactly the kind of thing that gets reversed
// and nothing about the values themselves gives it away.
type StatsCheck struct {
	Checked      bool     `json:"checked"`
	OK           bool     `json:"ok"`
	SumOfBuckets int64    `json:"sum_of_buckets,omitempty"`
	RatingsCount int64    `json:"ratings_count,omitempty"`
	DerivedMean  float64  `json:"derived_mean,omitempty"`
	Published    float64  `json:"published_average,omitempty"`
	Problems     []string `json:"problems,omitempty"`
}

// Check reconciles the distribution against the count and the average.
//
// The tolerances are not tight on purpose. Goodreads renders the histogram and
// the summary numbers from the same query but people are rating the book while
// the page is being built, so a small gap is the page being a moment out of
// date rather than a parsing error. A reversed slice is off by far more than
// this on any book with a real skew, which is every book.
func (s *Stats) Check() StatsCheck {
	var c StatsCheck
	if s == nil || len(s.RatingsCountDist) == 0 {
		return c
	}
	c.Checked = true

	var sum, weighted int64
	for i, n := range s.RatingsCountDist {
		if n < 0 {
			c.Problems = append(c.Problems, fmt.Sprintf("bucket %d is negative: %d", i+1, n))
		}
		sum += n
		weighted += int64(i+1) * n
	}
	c.SumOfBuckets = sum

	if len(s.RatingsCountDist) != 5 {
		c.Problems = append(c.Problems, fmt.Sprintf("the distribution has %d buckets, want 5 for one star through five", len(s.RatingsCountDist)))
	}
	if s.RatingsCount != nil {
		c.RatingsCount = *s.RatingsCount
		if drift := relDrift(float64(sum), float64(*s.RatingsCount)); drift > 0.01 {
			c.Problems = append(c.Problems, fmt.Sprintf("the buckets sum to %d and ratings_count is %d, %.1f%% apart", sum, *s.RatingsCount, drift*100))
		}
	}
	if sum > 0 {
		c.DerivedMean = float64(weighted) / float64(sum)
		if s.AverageRating != nil {
			c.Published = *s.AverageRating
			if math.Abs(c.DerivedMean-c.Published) > 0.02 {
				c.Problems = append(c.Problems, fmt.Sprintf("the buckets give a mean of %.4f and the published average is %.2f: the distribution may be reversed", c.DerivedMean, c.Published))
			}
		}
	}
	c.OK = len(c.Problems) == 0
	return c
}

func relDrift(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return math.Abs(a-b) / b
}

// contributorsFromFields carries the roles through from the edges.
func contributorsFromFields(e *Extractor) []Contributor {
	var out []Contributor
	if !remarshal(e.Fields["contributors"], &out) {
		return nil
	}
	for i := range out {
		out[i].Via = e.Surface
		// The cache keys contributors by their kca id and states the legacy one
		// in the profile link. 04_graph.md keeps both id spaces because neither
		// is derivable from the other, and here the page has said both, so
		// leaving legacy_id at zero would be dropping a fact that is on the
		// page and making every reader parse the URL back out again.
		if out[i].LegacyID == 0 && out[i].WebURL != "" {
			if ent, id := Classify(out[i].WebURL); ent == "author" {
				if n, err := strconv.ParseInt(id, 10, 64); err == nil {
					out[i].LegacyID = n
				}
			}
		}
	}
	return out
}

// seriesFromFields parses the position without losing it.
func seriesFromFields(e *Extractor) []SeriesEntry {
	var raw []seriesEntryRaw
	if !remarshal(e.Fields["series"], &raw) {
		return nil
	}
	out := make([]SeriesEntry, 0, len(raw))
	for _, r := range raw {
		entry := SeriesEntry{
			Series:   Ref{Type: "Series", ID: r.ID, Key: "Series:" + r.ID, Title: r.Name, Resolved: r.Name != ""},
			Position: r.Number,
		}
		if n, ok := parsePosition(r.Number); ok {
			entry.Number = &n
		}
		out = append(out, entry)
	}
	return out
}

// parsePosition reads a series position that is a single number.
//
// 2.5 parses, because novellas get half positions and dropping the fraction
// puts a novella and the novel before it in the same slot. A range like 1-3 on
// an omnibus does not parse and that is not an error: the raw string is kept
// either way and Number simply is not there.
func parsePosition(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// The node types the cache publishes in camelCase.
//
// These exist because remarshal round trips through JSON and Go's decoder
// matches field names case insensitively but knows nothing about underscores,
// so webUrl will never land in a field tagged web_url. Rather than tag the
// model in camelCase and make every consumer live with it, the cache shape gets
// its own struct and the conversion is one obvious function.
type (
	rawAward struct {
		Name        string `json:"name"`
		Category    string `json:"category"`
		AwardedAt   *int64 `json:"awardedAt"`
		Designation string `json:"designation"`
		WebURL      string `json:"webUrl"`
	}
	rawPlace struct {
		Name        string `json:"name"`
		CountryName string `json:"countryName"`
		Year        *int   `json:"year"`
		WebURL      string `json:"webUrl"`
	}
	rawCharacter struct {
		Name   string `json:"name"`
		Role   string `json:"role"`
		WebURL string `json:"webUrl"`
	}
	rawChoiceAward struct {
		Year     int    `json:"year"`
		Category string `json:"category"`
		WebURL   string `json:"webUrl"`
	}
	rawLanguageCount struct {
		ISOLanguageCode string `json:"isoLanguageCode"`
		Count           int64  `json:"count"`
	}
)

func awardsFrom(v any) []Award {
	var raw []rawAward
	if !remarshal(v, &raw) {
		return nil
	}
	out := make([]Award, 0, len(raw))
	for _, r := range raw {
		a := Award{
			Name:        r.Name,
			Category:    r.Category,
			AwardedAt:   r.AwardedAt,
			Designation: r.Designation,
			WebURL:      r.WebURL,
		}
		// designation is WINNER or NOMINEE, and a nomination is not a win. The
		// field is named awardsWon and carries both, which is the kind of thing
		// that turns into a wrong claim on somebody's book page.
		if r.Designation != "" {
			won := strings.EqualFold(r.Designation, "WINNER")
			a.HasWon = &won
		}
		out = append(out, a)
	}
	return out
}

func placesFrom(v any) []Place {
	var raw []rawPlace
	if !remarshal(v, &raw) {
		return nil
	}
	out := make([]Place, 0, len(raw))
	for _, r := range raw {
		out = append(out, Place(r))
	}
	return out
}

func charactersFrom(v any) []Character {
	var raw []rawCharacter
	if !remarshal(v, &raw) {
		return nil
	}
	out := make([]Character, 0, len(raw))
	for _, r := range raw {
		out = append(out, Character(r))
	}
	return out
}

func choiceAwardsFrom(v any) []ChoiceAward {
	var raw []rawChoiceAward
	if !remarshal(v, &raw) {
		return nil
	}
	out := make([]ChoiceAward, 0, len(raw))
	for _, r := range raw {
		out = append(out, ChoiceAward(r))
	}
	return out
}

func languageCountsFrom(v any) []LanguageCount {
	var raw []rawLanguageCount
	if !remarshal(v, &raw) {
		return nil
	}
	out := make([]LanguageCount, 0, len(raw))
	for _, r := range raw {
		out = append(out, LanguageCount(r))
	}
	return out
}

func genreRefsFrom(v any) []GenreRef {
	var raw []genreRaw
	if !remarshal(v, &raw) {
		return nil
	}
	out := make([]GenreRef, 0, len(raw))
	for _, r := range raw {
		out = append(out, GenreRef{Name: r.Name, WebURL: r.URL})
	}
	return out
}

// viaOf records the surface behind every field, including the blank ones.
//
// A field with a via entry and no value is row three of the four states: the
// surface published it and published it empty. A field with neither was not
// published, or was not looked at, and the surfaces list and the missed
// sentences say which.
func viaOf(e *Extractor) map[string]string {
	out := make(map[string]string, len(e.Levels)+len(e.Empty))
	for f := range e.Levels {
		out[f] = e.Surface
	}
	for f := range e.Empty {
		out[f] = e.Surface
	}
	return out
}

// timeFromMillis converts epoch milliseconds, negatives included.
//
// Anything published before 1970 has a negative publicationTime, which is a
// large share of every classic on the site, so a conversion that guards against
// negatives as if they were errors would drop them.
func timeFromMillis(ms int64) *time.Time {
	t := time.UnixMilli(ms).UTC()
	return &t
}

func firstString(e *Extractor, fields ...string) string {
	for _, f := range fields {
		if s, ok := e.Fields[f].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

func int64Of(v any) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int64:
		return n, true
	case float64:
		return int64(n), true
	}
	return 0, false
}

func floatOf(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

// remarshal moves a decoded cache value into a typed struct.
//
// A round trip through JSON rather than a hand written walk over map[string]any.
// It is slower and it is far harder to get subtly wrong, and the cache values
// came out of JSON in the first place so nothing is lost on the way.
func remarshal(v any, dst any) bool {
	if v == nil {
		return false
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return false
	}
	return json.Unmarshal(raw, dst) == nil
}
