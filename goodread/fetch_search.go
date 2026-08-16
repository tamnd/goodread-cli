package goodread

import (
	"context"
	"fmt"
	"time"
)

// The two search fetchers.
//
// GetSuggestRecord is the default and needs no flag. GetSearchRecord reads the
// page robots.txt disallows and gets nowhere without --no-robots, which the
// client enforces rather than this file.

// GetSuggestRecord reads /book/auto_complete into a search record.
func (c *Client) GetSuggestRecord(ctx context.Context, query string) (*SearchRecord, error) {
	q := SearchQuery{Query: query}.Norm()
	u := c.site(SearchAutocompleteURL(query))
	body, err := c.fetchPage(ctx, u)
	if err != nil {
		return nil, err
	}
	e, err := ExtractSuggest(body, SearchAutocompleteURL(query), query)
	if err != nil {
		return nil, err
	}
	rec, err := SuggestFrom(e, q, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	c.stamp(&rec.Envelope, u)
	return rec, nil
}

// GetSearchRecord reads /search, and walks it when asked for more than a page.
//
// Pages beyond the first come from the site's own next link rather than from a
// page number this tool built, because the link carries the qid and a walk that
// dropped it would be asking for a fresh search on every page and getting
// results that do not line up with the ranks it already has.
//
// A tab this tool has measured as unreadable is refused before a request is
// spent, and the refusal says what the site does rather than pretending the
// tool cannot. Nothing here tries to get past a challenge.
func (c *Client) GetSearchRecord(ctx context.Context, q SearchQuery, pages int) (*SearchRecord, error) {
	q = q.Norm()
	if _, readable, note := SearchTabInfo(q.Type); !readable {
		return nil, fmt.Errorf("%w: the %s tab is not readable anonymously, %s", ErrBlocked, q.Type, note)
	}
	if pages < 1 {
		pages = 1
	}

	u := c.site(SearchURL(q))
	first, err := c.searchPageRecord(ctx, q, u)
	if err != nil {
		return nil, err
	}

	for len(first.PagesWalked) < pages && first.NextPage != "" {
		next, err := c.searchPageRecord(ctx, q, c.site(first.NextPage))
		if err != nil {
			// The pages already read are still real, and so is the reason the
			// walk stopped. Both are kept rather than one being thrown away.
			first.Missed = append(first.Missed, "the walk stopped after page "+
				itoa(first.PagesWalked[len(first.PagesWalked)-1])+": "+err.Error())
			break
		}
		mergeSearchPage(first, next)
	}
	return first, nil
}

// searchPageRecord is one request and the record it makes, stamped.
func (c *Client) searchPageRecord(ctx context.Context, q SearchQuery, u string) (*SearchRecord, error) {
	body, err := c.fetchPage(ctx, u)
	if err != nil {
		return nil, err
	}
	e, err := ExtractSearch(body, u)
	if err != nil {
		return nil, err
	}
	rec, err := SearchFrom(e, q, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	c.stamp(&rec.Envelope, u)
	return rec, nil
}

// mergeSearchPage folds a later page into the first one.
//
// The page level facts stay as the first page stated them, since they describe
// the search and not the page, and the two things that do move are the rows and
// the record of which pages produced them. Ranks are left exactly as the site
// gave them, per page, because rewriting them into a running count would
// silently invent a global ordering the site never published.
func mergeSearchPage(into, next *SearchRecord) {
	into.Results = append(into.Results, next.Results...)
	into.PagesWalked = append(into.PagesWalked, next.Page)
	into.NextPage = next.NextPage
	into.Surfaces = mergeStrings(into.Surfaces, next.Surfaces)
	into.Sources = mergeStrings(into.Sources, next.Sources)
	if into.LastPage == nil {
		into.LastPage = next.LastPage
	}
}

// mergeStrings unions two lists and keeps the order of the first.
func mergeStrings(a, b []string) []string {
	seen := map[string]bool{}
	for _, s := range a {
		seen[s] = true
	}
	for _, s := range b {
		if !seen[s] {
			seen[s] = true
			a = append(a, s)
		}
	}
	return a
}
