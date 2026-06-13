package goodread

import (
	"context"
	"encoding/xml"
	"strconv"
	"strings"
	"time"
)

// rssShelf mirrors the Goodreads shelf RSS feed (/review/list_rss/<id>).
type rssShelf struct {
	Channel struct {
		Title string    `xml:"title"`
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
}

type rssItem struct {
	GUID          string `xml:"guid"`
	Title         string `xml:"title"`
	BookID        string `xml:"book_id"`
	BookImageURL  string `xml:"book_image_url"`
	AuthorName    string `xml:"author_name"`
	ISBN          string `xml:"isbn"`
	UserRating    string `xml:"user_rating"`
	UserReadAt    string `xml:"user_read_at"`
	UserDateAdded string `xml:"user_date_added"`
	UserShelves   string `xml:"user_shelves"`
	UserReview    string `xml:"user_review"`
	AverageRating string `xml:"average_rating"`
	NumPages      string `xml:"book>num_pages"`
}

// ParseShelfRSS parses a shelf RSS feed into a shelf header and its rows.
func ParseShelfRSS(body []byte, userID, shelfName, pageURL string) (*Shelf, []ShelfBook, error) {
	var feed rssShelf
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, nil, err
	}
	shelfID := userID + "/" + shelfName
	shelf := &Shelf{
		ShelfID:   shelfID,
		UserID:    userID,
		Name:      shelfName,
		URL:       pageURL,
		FetchedAt: time.Now(),
	}
	rows := make([]ShelfBook, 0, len(feed.Channel.Items))
	for _, it := range feed.Channel.Items {
		sb := ShelfBook{
			ShelfID:    shelfID,
			UserID:     userID,
			BookID:     strings.TrimSpace(it.BookID),
			Title:      cleanHTML(it.Title),
			AuthorName: strings.TrimSpace(it.AuthorName),
			ISBN:       strings.TrimSpace(it.ISBN),
			NumPages:   atoiClean(it.NumPages),
			AvgRating:  parseFloat(it.AverageRating),
			Rating:     atoiClean(it.UserRating),
			ReviewID:   reviewIDFromGUID(it.GUID),
			ReviewText: cleanHTML(it.UserReview),
			CoverURL:   strings.TrimSpace(it.BookImageURL),
		}
		for _, s := range strings.Split(it.UserShelves, ",") {
			if s = strings.TrimSpace(s); s != "" {
				sb.Shelves = append(sb.Shelves, s)
			}
		}
		sb.DateAdded = parseRSSDate(it.UserDateAdded)
		sb.DateRead = parseRSSDate(it.UserReadAt)
		rows = append(rows, sb)
	}
	shelf.BooksCount = len(rows)
	return shelf, rows, nil
}

// GetShelfRSS fetches a user's shelf via the open RSS feed (no WAF challenge).
func (c *Client) GetShelfRSS(ctx context.Context, userID, shelfName string) (*Shelf, []ShelfBook, error) {
	u := ShelfRSSURL(userID, shelfName)
	body, code, err := c.Fetch(ctx, u)
	if err != nil {
		return nil, nil, err
	}
	if code == 404 {
		return nil, nil, ErrNotFound
	}
	return ParseShelfRSS(body, numericPrefix(userID), shelfName, u)
}

func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f
}

func reviewIDFromGUID(guid string) string {
	if m := reReviewID.FindStringSubmatch(guid); len(m) > 1 {
		return m[1]
	}
	return ""
}

// parseRSSDate parses the RFC1123Z-ish dates Goodreads emits in RSS.
func parseRSSDate(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{
		"Mon, 2 Jan 2006 15:04:05 -0700",
		"Mon, 02 Jan 2006 15:04:05 -0700",
		time.RFC1123Z,
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
