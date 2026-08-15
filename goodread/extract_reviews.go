package goodread

import (
	"encoding/json"
	"strconv"
	"strings"
)

// The reviews a book page already carries.
//
// robots.txt disallows /book/reviews, so v0.2.0's answer to "show me the
// reviews" was a path it should not have been walking. It did not need to be:
// the book page ships 30 Review entities and their 30 Shelving entities inline,
// with rating, text, like count, comment count and the shelf each reader filed
// the book under. That is one allowed request for what used to be a disallowed
// one, and it carries more per review than the rendered page does.
//
// It is a sample and not the corpus, which is what the missed sentence is for.

// reviewRaw is one review as the cache carries it.
type reviewRaw struct {
	ID           string   `json:"id"`
	Text         string   `json:"text"`
	Rating       *int     `json:"rating"`
	CreatedAt    int64    `json:"created_at_ms"`
	UpdatedAt    int64    `json:"updated_at_ms"`
	LikeCount    int64    `json:"like_count"`
	CommentCount int64    `json:"comment_count"`
	Spoiler      *bool    `json:"spoiler"`
	UserKey      string   `json:"user_key"`
	UserID       string   `json:"user_id"`
	UserName     string   `json:"user_name"`
	UserURL      string   `json:"user_web_url"`
	UserImageURL string   `json:"user_image_url"`
	UserIsAuthor *bool    `json:"user_is_author"`
	ShelvingKey  string   `json:"shelving_key"`
	Shelf        string   `json:"shelf"`
	ShelfURL     string   `json:"shelf_web_url"`
	Tags         []string `json:"tags"`
}

// reviewsFrom reads the getReviews connection off ROOT_QUERY.
//
// Through the root and not through a type scan, for the same reason the book
// entity is: what is on the page is what the page asked for, and a scan would
// pick up whatever else happens to be in the cache.
func (e *Extractor) reviewsFrom(cache Apollo) {
	raw, _, ok := cache.Root("getReviews")
	if !ok {
		return
	}
	conn, ok := cache.Resolve(raw).(map[string]any)
	if !ok {
		return
	}
	if n, ok := conn["totalCount"].(float64); ok {
		e.set("reviews_total", LevelNextData, int64(n))
	}

	edges, _ := conn["edges"].([]any)
	out := make([]reviewRaw, 0, len(edges))
	for _, it := range edges {
		edge, ok := it.(map[string]any)
		if !ok {
			continue
		}
		node, ok := edge["node"].(map[string]any)
		if !ok {
			continue
		}
		r := reviewRaw{
			ID:   strOfAny(node["id"]),
			Text: strOfAny(node["text"]),
		}
		// A zero here means the reader wrote text and gave no stars, which the
		// model represents as no rating rather than as a zero star one.
		if n, ok := node["rating"].(float64); ok && n >= 1 {
			v := int(n)
			r.Rating = &v
		}
		if b, ok := node["spoilerStatus"].(bool); ok {
			r.Spoiler = &b
		}
		r.CreatedAt = millisOf(node["createdAt"])
		r.UpdatedAt = millisOf(node["updatedAt"])
		r.LikeCount = millisOf(node["likeCount"])
		r.CommentCount = millisOf(node["commentCount"])

		if u, ok := node["creator"].(map[string]any); ok {
			r.UserKey = strOfAny(u["__key"])
			r.UserID = strOfAny(u["id"])
			if r.UserID == "" {
				if n, ok := u["id"].(float64); ok {
					r.UserID = strconv.FormatInt(int64(n), 10)
				}
			}
			r.UserName = strOfAny(u["name"])
			r.UserURL = strOfAny(u["webUrl"])
			r.UserImageURL = strOfAny(u["imageUrlSquare"])
			if b, ok := u["isAuthor"].(bool); ok {
				r.UserIsAuthor = &b
			}
		}

		// The shelving is where the reader's own filing lives: the shelf they
		// put it on and the tags they gave it. It is a different fact from the
		// review and it is the only place either one appears.
		if sh, ok := node["shelving"].(map[string]any); ok {
			r.ShelvingKey = strOfAny(sh["__key"])
			if shelf, ok := sh["shelf"].(map[string]any); ok {
				r.Shelf = strOfAny(shelf["name"])
				r.ShelfURL = strOfAny(shelf["webUrl"])
			}
			r.Tags = tagsOf(sh["taggings"])
		}
		if r.ID == "" {
			continue
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return
	}
	e.set("reviews", LevelNextData, out)

	// The sentence is built from both numbers, because "30 reviews" reads like
	// the answer and "30 of 274,808" reads like the sample it is.
	if total, ok := e.Fields["reviews_total"].(int64); ok && total > int64(len(out)) {
		e.Miss("the book page carries %d of %s reviews. `goodread reviews <id> --all --no-robots` reads /book/reviews for ten pages more.",
			len(out), commaInt(total))
	}
}

// tagsOf flattens the taggings list to the tag names.
func tagsOf(v any) []string {
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		tag, ok := m["tag"].(map[string]any)
		if !ok {
			continue
		}
		if name := strOfAny(tag["name"]); name != "" {
			out = append(out, name)
		}
	}
	return out
}

func millisOf(v any) int64 {
	n, ok := v.(float64)
	if !ok {
		return 0
	}
	return int64(n)
}

// commaInt groups thousands for a sentence a person reads.
func commaInt(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var out []byte
	for i := 0; i < len(s); i++ {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, s[i])
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}

// ReviewsFromRecord is the mapping into the model.
//
// Reviews and shelvings come out of the same entities and are returned
// together, because a Shelving with no review attached is not something the
// book page carries and inventing an empty one would be a lie about the source.
func reviewsFromFields(e *Extractor) ([]Review, []Shelving) {
	var raws []reviewRaw
	if !remarshal(e.Fields["reviews"], &raws) {
		return nil, nil
	}
	via := e.Surface
	reviews := make([]Review, 0, len(raws))
	var shelvings []Shelving
	for _, r := range raws {
		user := &Ref{Type: "User", ID: r.UserID, Key: r.UserKey, Title: r.UserName, Resolved: r.UserName != ""}
		rv := Review{
			ID:        r.ID,
			Rating:    r.Rating,
			Text:      r.Text,
			LikeCount: int64Ptr(r.LikeCount),
			Spoiler:   r.Spoiler,
			User:      user,
			Via:       via,
		}
		rv.CreatedAt = timeFromMillis(r.CreatedAt)
		rv.UpdatedAt = timeFromMillis(r.UpdatedAt)
		if r.CommentCount > 0 {
			rv.Extra = map[string]json.RawMessage{
				"comment_count": json.RawMessage(strconv.FormatInt(r.CommentCount, 10)),
			}
		}
		reviews = append(reviews, rv)

		if r.ShelvingKey == "" && r.Shelf == "" {
			continue
		}
		sh := Shelving{
			ID:        r.ShelvingKey,
			ShelfName: r.Shelf,
			Rating:    r.Rating,
			User:      user,
			Via:       via,
		}
		if len(r.Tags) > 0 {
			if b, err := json.Marshal(r.Tags); err == nil {
				sh.Extra = map[string]json.RawMessage{"tags": b}
			}
		}
		shelvings = append(shelvings, sh)
	}
	return reviews, shelvings
}

func int64Ptr(n int64) *int64 {
	if n == 0 {
		return nil
	}
	return &n
}
