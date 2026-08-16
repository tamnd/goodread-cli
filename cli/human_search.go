package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/tamnd/goodread-cli/goodread"
)

// printSearch is the table view of a search.
//
// The header is the part that is worth as much as the rows. Twenty results out
// of eight hundred and sixteen looks exactly like twenty out of twenty once the
// header is gone, and the difference is the whole reason the page metadata is
// on the record at all.
func printSearch(w io.Writer, rec *goodread.SearchRecord) {
	fmt.Fprintf(w, "%s\n\n", rec.Query)

	line(w, "type", rec.SearchType)
	line(w, "field", rec.Field)
	line(w, "results", searchCount(rec))
	if rec.Elapsed != nil {
		line(w, "took", fmt.Sprintf("%.2fs, by the site's own clock", *rec.Elapsed))
	}
	if rec.Genre != nil {
		line(w, "genre", rec.Genre.Label())
	}
	line(w, "qid", rec.QID)
	line(w, "url", rec.WebURL)
	if len(rec.PagesWalked) > 1 {
		line(w, "pages", fmt.Sprintf("%s of %s", pageList(rec.PagesWalked), lastPageLabel(rec)))
	}

	if len(rec.Results) > 0 {
		fmt.Fprintf(w, "\nresults (%d shown)\n", len(rec.Results))
		for _, h := range rec.Results {
			printHit(w, h)
		}
	} else {
		fmt.Fprintln(w, "\nno results")
	}

	if len(rec.RelatedShelves) > 0 {
		var names []string
		for _, s := range rec.RelatedShelves {
			names = append(names, s.Name)
		}
		fmt.Fprintln(w)
		fmt.Fprintln(w, "related shelves, counted over the whole site and not over these results")
		fmt.Fprintln(w, wrap(strings.Join(names, ", "), 78))
	}

	// The tabs that do not answer are printed rather than hidden, because a
	// person who wants Listopia results deserves to know why they cannot have
	// them without spending a request to find out.
	var blocked []string
	for _, t := range rec.Tabs {
		if !t.Readable {
			blocked = append(blocked, t.Type)
		}
	}
	if len(blocked) > 0 {
		fmt.Fprintf(w, "\nnot readable signed out: %s\n", strings.Join(blocked, ", "))
	}

	printNotRead(w, rec.Missed)
}

// printHit is one row.
func printHit(w io.Writer, h goodread.SearchHit) {
	var by string
	if len(h.Contributors) > 0 {
		by = " by " + h.Contributors[0].Name
	}
	fmt.Fprintf(w, "  %2d. %s%s%s\n", h.Rank, h.Title, by, ratingSuffix(h.AverageRating, h.RatingsCount))

	var bits []string
	if h.PublishedAt != "" {
		bits = append(bits, "published "+h.PublishedAt)
	}
	if h.NumPages != nil {
		bits = append(bits, fmt.Sprintf("%d pages", *h.NumPages))
	}
	if h.EditionsCount != nil {
		bits = append(bits, fmt.Sprintf("%s editions", comma(*h.EditionsCount)))
	}
	if h.Work != nil && h.Work.ID != "" {
		bits = append(bits, "work "+h.Work.ID)
	}
	if len(bits) > 0 {
		fmt.Fprintf(w, "      %s\n", strings.Join(bits, " · "))
	}
}

// searchCount is the count line, hedged exactly as the site hedged it.
func searchCount(rec *goodread.SearchRecord) string {
	if rec.TotalResults == nil {
		if len(rec.Results) == 0 {
			return ""
		}
		return fmt.Sprintf("%d, with no total published by this surface", len(rec.Results))
	}
	total := comma(*rec.TotalResults)
	if rec.Approximate {
		total = "about " + total
	}
	if rec.Showing != nil {
		return fmt.Sprintf("%d-%d of %s", rec.Showing.From, rec.Showing.To, total)
	}
	return total
}

func pageList(pages []int) string {
	var parts []string
	for _, p := range pages {
		parts = append(parts, fmt.Sprintf("%d", p))
	}
	return strings.Join(parts, ", ")
}

func lastPageLabel(rec *goodread.SearchRecord) string {
	if rec.LastPage == nil {
		return "an unstated number"
	}
	return fmt.Sprintf("%d", *rec.LastPage)
}
