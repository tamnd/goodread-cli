package goodread

import (
	"fmt"
	"sort"
	"strings"
)

// The three levels of the ladder, tried in order, per field.
const (
	LevelNextData = 1 // __NEXT_DATA__ Apollo cache
	LevelMeta     = 2 // application/ld+json and og:
	LevelSelector = 3 // CSS selectors over rendered HTML
)

// LevelName is for reports and for nothing else.
func LevelName(level int) string {
	switch level {
	case LevelNextData:
		return "__NEXT_DATA__"
	case LevelMeta:
		return "ld+json / og"
	case LevelSelector:
		return "selectors"
	}
	return "unknown"
}

// SelectorField is one field still read from rendered HTML.
//
// A list rather than scattered calls, so `goodread extraction` can print it and
// so the number is a thing somebody can drive down. A selector is a promise to
// break on a redesign, and a promise that is written down is one somebody can
// pay off.
//
// Since records the version it was added in, so a selector that has survived
// three releases is visible as debt rather than as furniture.
type SelectorField struct {
	Surface string
	Entity  string
	Field   string
	Sel     string
	Since   string
	Why     string
}

// selectorFields is the registry. Anything read by selector is declared here.
//
// The count is high and that is honest rather than embarrassing. Only the book
// page is a Next.js route; author, series, list, genre, editions and quotes are
// still Rails templates with no Apollo cache and nothing but og: tags, so for
// those surfaces a selector is not a shortcut, it is the only thing there is.
// See SurfaceHasNextData.
var selectorFields []SelectorField

// RegisterSelector adds a selector field to the registry.
//
// Called from the parsers rather than declared in one literal, so a selector
// lives next to the code that uses it and cannot be added without being counted.
func RegisterSelector(f SelectorField) { selectorFields = append(selectorFields, f) }

// SelectorFields returns the registry, sorted for a stable report.
func SelectorFields() []SelectorField {
	out := append([]SelectorField(nil), selectorFields...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Surface != out[j].Surface {
			return surfaceNum(out[i].Surface) < surfaceNum(out[j].Surface)
		}
		if out[i].Entity != out[j].Entity {
			return out[i].Entity < out[j].Entity
		}
		return out[i].Field < out[j].Field
	})
	return out
}

// Levels records which rung of the ladder answered for each field.
//
// Separate from Via, which names the surface. A field can come from the book
// page by selector and from the editions page by ld+json, and the drift report
// wants to tell those apart. Kept as a map on the record rather than as a
// parallel struct so it survives a JSON round trip.
type Levels map[string]int

// Count returns how many fields landed on a given level.
func (l Levels) Count(level int) int {
	n := 0
	for _, v := range l {
		if v == level {
			n++
		}
	}
	return n
}

// Fields returns the field names on a level, sorted.
func (l Levels) Fields(level int) []string {
	var out []string
	for k, v := range l {
		if v == level {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// Summary is the one-line form used by -v.
func (l Levels) Summary() string {
	return fmt.Sprintf("extract: level 1 for %d fields, level 2 for %d fields, level 3 for %d fields",
		l.Count(LevelNextData), l.Count(LevelMeta), l.Count(LevelSelector))
}

// Extractor accumulates a record's fields along with the level each came from.
//
// Every field write goes through set, so the extraction report is generated
// rather than maintained. A field assigned directly to the record struct is
// invisible to the report, which is why TestNoDirectFieldWrites greps for it.
type Extractor struct {
	Surface string
	Fields  map[string]any
	Levels  Levels
	Missed  []string
	Unknown map[string]int
}

// NewExtractor starts an extraction for one surface.
func NewExtractor(surface string) *Extractor {
	return &Extractor{
		Surface: surface,
		Fields:  map[string]any{},
		Levels:  Levels{},
		Unknown: map[string]int{},
	}
}

// set records a field and the level that produced it.
//
// First write wins. The ladder is tried in order, so a level 1 answer is
// already there by the time level 2 runs, and letting a lower rung overwrite a
// higher one would quietly downgrade a field that was read correctly.
func (e *Extractor) set(field string, level int, v any) {
	if v == nil || isZeroish(v) {
		return
	}
	if _, seen := e.Fields[field]; seen {
		return
	}
	e.Fields[field] = v
	e.Levels[field] = level
}

// Set is set, exported for parsers in this package's callers.
func (e *Extractor) Set(field string, level int, v any) { e.set(field, level, v) }

// Miss records something the page did not carry, with the command that would
// get it. Generated from data rather than hardcoded, which is the only kind of
// missed sentence that stays true.
func (e *Extractor) Miss(format string, args ...any) {
	e.Missed = append(e.Missed, fmt.Sprintf(format, args...))
}

// NoteUnknown counts a field the model does not know about.
//
// A new field appearing is Goodreads shipping something, which is a chance to
// capture it rather than a failure. Counted here and reported by
// `goodread verify`.
func (e *Extractor) NoteUnknown(path string) { e.Unknown[path]++ }

// isZeroish reports whether a value is worth recording.
//
// An empty string is the page not having said anything, which is different from
// the page saying "". The distinction matters because set is first-write-wins:
// recording an empty level 1 answer would block a real level 2 one.
func isZeroish(v any) bool {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t) == ""
	case []string:
		return len(t) == 0
	case nil:
		return true
	}
	return false
}
