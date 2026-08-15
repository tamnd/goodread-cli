package goodread

import (
	"context"
	"strconv"
	"time"
)

// WorkRecord is a work read the only way this tool is allowed to read one.
//
// /work/<id> is s13 and robots.txt disallows it, so there is no page here that
// is about the work and nothing else. What there is instead is the editions
// page, which is allowed by an explicit Allow, and every edition's own page,
// which carries the work the edition belongs to with its places, characters,
// awards and work level stats already on it.
//
// So a work read is two allowed reads standing in for one disallowed one, and
// the record says which edition it went through rather than presenting the
// result as if it came off a work page.
type WorkRecord struct {
	Work *Work `json:"work"`

	// BestBook is the edition the work was read through.
	//
	// The first row of the editions page, which is the edition Goodreads
	// renders first and links from search. That is what "best edition" means
	// here and it is an observation about the page rather than a judgement,
	// which is why the field carries the whole book and lets somebody see
	// which one it was.
	BestBook *Book `json:"best_book,omitempty"`

	// EditionCount is what the editions page says exists, not what was read.
	EditionCount *int64 `json:"edition_count,omitempty"`
}

// GetWorkRecord reads a work through its best edition.
func (c *Client) GetWorkRecord(ctx context.Context, workID string) (*WorkRecord, error) {
	ed, err := c.GetEditionsRecord(ctx, workID, 1)
	if err != nil {
		return nil, err
	}
	if len(ed.Editions) == 0 {
		return nil, ErrNotFound
	}

	best := ed.Editions[0].Book.ID
	rec, err := c.GetBookRecord(ctx, best, DepthMeta)
	if err != nil {
		return nil, err
	}

	out := &WorkRecord{Work: rec.Work, BestBook: rec.Book, EditionCount: ed.TotalCount}
	if out.Work == nil {
		// The edition's page did not carry its work, which happens on the
		// older Rails book pages. The editions page still knows the title and
		// the id, so the record is thin rather than absent, and it says which
		// fields it never had rather than leaving them looking unset.
		legacy, _ := strconv.ParseInt(numericPrefix(workID), 10, 64)
		out.Work = &Work{
			Envelope: Envelope{
				Kind:        "work",
				RetrievedAt: time.Now().UTC(),
				Missed:      []string{"the best edition's page carried no work, so this is the editions page's title and id and nothing else"},
			},
			LegacyID:      legacy,
			OriginalTitle: ed.Title,
		}
	}
	out.Work.Surfaces = withSurface(out.Work.Surfaces, "s6")
	return out, nil
}

// withSurface adds a surface id to a list without repeating it.
func withSurface(have []string, add string) []string {
	for _, s := range have {
		if s == add {
			return have
		}
	}
	return append(have, add)
}
