package goodread

import (
	"context"
	"time"
)

// BookRecord is a book read into the v0.3.0 model.
//
// The work comes back alongside rather than nested, because the book page
// carries a partly filled work and the two are separate records with separate
// provenance. Nesting it would make it look like a work read.
type BookRecord struct {
	Book *Book `json:"book"`
	Work *Work `json:"work,omitempty"`
}

// GetBookRecord fetches a book page and reads it with the extractor.
//
// This is the v0.3.0 path. GetBook is still the v0.2.0 one and still returns a
// ScrapedBook, and the two live side by side until the commands are ported.
func (c *Client) GetBookRecord(ctx context.Context, id string, depth Depth) (*BookRecord, error) {
	u := BookURL(id)
	body, code, err := c.Fetch(ctx, u)
	if err != nil {
		return nil, err
	}
	if code == 404 || len(body) == 0 {
		return nil, ErrNotFound
	}

	e, err := ExtractBook(body)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	b, err := BookFrom(e, now)
	if err != nil {
		return nil, err
	}
	rec := &BookRecord{Book: b}
	if w, ok := WorkFrom(e, now); ok {
		rec.Work = w
	}

	// full and deep read further surfaces, and neither is wired up yet. Saying
	// so in the record beats accepting the flag and quietly doing meta, which
	// is how somebody ends up with a crawl they think is deeper than it is.
	if depth == DepthFull || depth == DepthDeep {
		b.Missed = append(b.Missed, "depth "+string(depth)+" is not implemented yet, so this record is a meta read of the book page.")
	}
	return rec, nil
}
