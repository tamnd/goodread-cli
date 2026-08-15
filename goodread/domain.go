package goodread

import (
	"context"
	"errors"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// domain.go exposes Goodreads as a kit Domain: a driver that a multi-domain host
// (ant) enables with a single blank import,
//
//	import _ "github.com/tamnd/goodread-cli/goodread"
//
// exactly as a database/sql program enables a driver with `import _
// "github.com/lib/pq"`. The init below registers it; the host then dereferences
// goodreads:// URIs by routing to the operations Register installs. The standalone
// goodread binary does not use any of this, so the CLI is unchanged.
func init() { kit.Register(Domain{}) }

// Domain is the Goodreads driver. It carries no state; the per-run client is
// built by the factory Register hands kit.
type Domain struct{}

// Info describes the scheme, the hostnames a pasted link is matched against, and
// the identity a host reuses for help and version.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme:  "goodreads",
		Aliases: []string{"gr"},
		Hosts:   []string{"goodreads.com"},
		Identity: kit.Identity{
			Binary: "goodread",
			Short:  "Read public Goodreads data",
			Site:   "goodreads.com",
			Repo:   "https://github.com/tamnd/goodread-cli",
		},
	}
}

// Register installs the client factory and every Goodreads operation onto app.
// A resolver op (Single) names its own record type and answers `ant get`; a List
// op enumerates a parent resource's members and answers `ant ls`.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	// Resolver ops: one record per id, the home of `ant get goodreads://<type>/<id>`.
	kit.Handle(app, kit.OpMeta{Name: "book", Group: "read", Single: true,
		Summary: "Fetch a book by id or URL", URIType: "book", Resolver: true,
		Args: []kit.Arg{{Name: "ref", Help: "book id or URL"}}}, getBook)
	kit.Handle(app, kit.OpMeta{Name: "author", Group: "read", Single: true,
		Summary: "Fetch an author by id or URL", URIType: "author", Resolver: true,
		Args: []kit.Arg{{Name: "ref", Help: "author id or URL"}}}, getAuthor)
	kit.Handle(app, kit.OpMeta{Name: "series", Group: "read", Single: true,
		Summary: "Fetch a series header", URIType: "series", Resolver: true,
		Args: []kit.Arg{{Name: "ref", Help: "series id or URL"}}}, getSeries)
	kit.Handle(app, kit.OpMeta{Name: "list", Group: "read", Single: true,
		Summary: "Fetch a Listopia list header", URIType: "list", Resolver: true,
		Args: []kit.Arg{{Name: "ref", Help: "list id or URL"}}}, getList)
	kit.Handle(app, kit.OpMeta{Name: "user", Group: "read", Single: true,
		Summary: "Fetch a public user profile", URIType: "user", Resolver: true,
		Args: []kit.Arg{{Name: "ref", Help: "user id or URL"}}}, getUser)
	kit.Handle(app, kit.OpMeta{Name: "genre", Group: "read", Single: true,
		Summary: "Fetch a genre", URIType: "genre", Resolver: true,
		Args: []kit.Arg{{Name: "ref", Help: "genre slug or URL"}}}, getGenre)
	kit.Handle(app, kit.OpMeta{Name: "shelf", Group: "read", Single: true,
		Summary: "Fetch a user's shelf header", URIType: "shelf", Resolver: true,
		Args: []kit.Arg{{Name: "ref", Help: "user id or URL"}}}, getShelf)
	kit.Handle(app, kit.OpMeta{Name: "quote", Group: "read",
		Summary: "Fetch quotes for an author or work", URIType: "quote", Resolver: true,
		Args: []kit.Arg{{Name: "ref", Help: "author/work id or quotes URL"}}}, getQuotes)

	// List ops: members of a parent resource, the home of `ant ls`.
	kit.Handle(app, kit.OpMeta{Name: "series-books", Group: "read", List: true,
		Summary: "List a series' books", URIType: "series",
		Args: []kit.Arg{{Name: "ref", Help: "series id or URL"}}}, listSeriesBooks)
	kit.Handle(app, kit.OpMeta{Name: "list-books", Group: "read", List: true,
		Summary: "List a Listopia list's books", URIType: "list",
		Args: []kit.Arg{{Name: "ref", Help: "list id or URL"}}}, listListBooks)
	kit.Handle(app, kit.OpMeta{Name: "reviews", Group: "read", List: true,
		Summary: "List reviews embedded on a book page", URIType: "book",
		Args: []kit.Arg{{Name: "ref", Help: "book id or URL"}}}, listReviews)

	// Search is not URI-addressable but rounds out the surface a host can drive.
	kit.Handle(app, kit.OpMeta{Name: "search", Group: "read",
		Summary: "Search books and authors",
		Args:    []kit.Arg{{Name: "query", Help: "search terms", Variadic: true}}}, search)
}

// newClient builds the Goodreads client from the host-resolved config, reusing
// the same data dir the standalone binary uses so a lent session and the page
// cache are shared.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	gcfg := DefaultConfig()
	if cfg.DataDir != "" {
		gcfg.DataDir = cfg.DataDir
	}
	gcfg.NoCache = cfg.NoCache
	return NewClient(gcfg), nil
}

// --- inputs ---

type oneRef struct {
	Ref    string  `kit:"arg" help:"id, slug, or Goodreads URL"`
	Client *Client `kit:"inject"`
}

type shelfRef struct {
	Ref    string  `kit:"arg" help:"user id or profile URL"`
	Shelf  string  `kit:"flag" help:"shelf name" default:"read"`
	Client *Client `kit:"inject"`
}

type queryRef struct {
	Query  []string `kit:"arg,variadic" help:"search terms"`
	Limit  int      `kit:"flag,inherit" help:"max results"`
	Client *Client  `kit:"inject"`
}

// --- handlers ---

func getBook(ctx context.Context, in oneRef, emit func(*ScrapedBook) error) error {
	b, err := in.Client.GetBook(ctx, refID(in.Ref))
	if err != nil {
		return mapErr(err)
	}
	return emit(b)
}

func getAuthor(ctx context.Context, in oneRef, emit func(*Author) error) error {
	a, err := in.Client.GetAuthor(ctx, refID(in.Ref))
	if err != nil {
		return mapErr(err)
	}
	return emit(a)
}

func getSeries(ctx context.Context, in oneRef, emit func(*Series) error) error {
	s, _, err := in.Client.GetSeries(ctx, refID(in.Ref))
	if err != nil {
		return mapErr(err)
	}
	return emit(s)
}

func listSeriesBooks(ctx context.Context, in oneRef, emit func(SeriesBook) error) error {
	_, books, err := in.Client.GetSeries(ctx, refID(in.Ref))
	if err != nil {
		return mapErr(err)
	}
	for _, b := range books {
		if err := emit(b); err != nil {
			return err
		}
	}
	return nil
}

func getList(ctx context.Context, in oneRef, emit func(*List) error) error {
	l, _, err := in.Client.GetList(ctx, listID(in.Ref))
	if err != nil {
		return mapErr(err)
	}
	return emit(l)
}

func listListBooks(ctx context.Context, in oneRef, emit func(ListBook) error) error {
	_, books, err := in.Client.GetList(ctx, listID(in.Ref))
	if err != nil {
		return mapErr(err)
	}
	for _, b := range books {
		if err := emit(b); err != nil {
			return err
		}
	}
	return nil
}

func getUser(ctx context.Context, in oneRef, emit func(*ScrapedUser) error) error {
	u, err := in.Client.GetUser(ctx, refID(in.Ref))
	if err != nil {
		return mapErr(err)
	}
	return emit(u)
}

func getGenre(ctx context.Context, in oneRef, emit func(*Genre) error) error {
	g, err := in.Client.GetGenre(ctx, genreSlug(in.Ref))
	if err != nil {
		return mapErr(err)
	}
	return emit(g)
}

func getShelf(ctx context.Context, in shelfRef, emit func(*Shelf) error) error {
	shelf, _, err := in.Client.GetShelfRSS(ctx, refID(in.Ref), in.Shelf)
	if err != nil {
		return mapErr(err)
	}
	return emit(shelf)
}

func getQuotes(ctx context.Context, in oneRef, emit func(Quote) error) error {
	quotes, err := in.Client.GetQuotes(ctx, quotesURL(in.Ref))
	if err != nil {
		return mapErr(err)
	}
	for _, q := range quotes {
		if err := emit(q); err != nil {
			return err
		}
	}
	return nil
}

func listReviews(ctx context.Context, in oneRef, emit func(ScrapedReview) error) error {
	reviews, err := in.Client.GetReviews(ctx, refID(in.Ref))
	if err != nil {
		return mapErr(err)
	}
	for _, r := range reviews {
		if err := emit(r); err != nil {
			return err
		}
	}
	return nil
}

func search(ctx context.Context, in queryRef, emit func(SearchResult) error) error {
	res, err := in.Client.Search(ctx, strings.Join(in.Query, " "), in.Limit)
	if err != nil {
		return mapErr(err)
	}
	for _, r := range res {
		if err := emit(r); err != nil {
			return err
		}
	}
	return nil
}

// --- Resolver: the URI-native string functions, reused from ids.go ---

// Classify turns any accepted input into the canonical (type, id), so `ant
// resolve` and `ant url` need no network.
func (Domain) Classify(input string) (uriType, id string, err error) {
	e, got := Classify(input)
	if e == "" || got == "" {
		return "", "", errs.Usage("unrecognized Goodreads reference: %q", input)
	}
	return e, got, nil
}

// Locate is the inverse: the live page URL for a (type, id).
func (Domain) Locate(uriType, id string) (string, error) {
	switch uriType {
	case "book":
		return BookURL(id), nil
	case "author":
		return AuthorURL(id), nil
	case "series":
		return SeriesURL(id), nil
	case "list":
		return ListURL(id), nil
	case "user":
		return UserURL(id), nil
	case "quote":
		return QuoteURL(id), nil
	case "genre":
		return GenreURL(id), nil
	case "shelf":
		user, shelf, _ := strings.Cut(id, "/")
		if shelf == "" {
			shelf = "read"
		}
		return ShelfURL(user, shelf, 1), nil
	default:
		return "", errs.Usage("goodreads has no resource type %q", uriType)
	}
}

// --- helpers ---

// refID extracts the bare id from any accepted reference, falling back to the
// trimmed input when it is already a bare id or slug.
func refID(s string) string {
	if _, id := Classify(s); id != "" {
		return id
	}
	return strings.TrimSpace(s)
}

// listID keeps a Listopia "<num>.<slug>" id intact, since the list page uses it
// verbatim, but unwraps a full /list/show/ URL.
func listID(s string) string {
	if e, id := Classify(s); e == "list" && id != "" {
		return id
	}
	return strings.TrimSpace(s)
}

// genreSlug keeps a genre slug as-is but unwraps a /genres/ URL.
func genreSlug(s string) string {
	if e, slug := Classify(s); e == "genre" && slug != "" {
		return slug
	}
	return strings.TrimSpace(s)
}

// quotesURL turns an author/work id into its quotes page, or passes a full URL
// through unchanged.
func quotesURL(arg string) string {
	arg = strings.TrimSpace(arg)
	if strings.HasPrefix(arg, "http") {
		return arg
	}
	return BaseURL + "/author/quotes/" + refID(arg)
}

// mapErr converts a library error into the kit error kind that carries the right
// exit code, so a host renders the same blocked/not-found/rate-limited outcomes
// the standalone binary does.
func mapErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrBlocked):
		return errs.NeedAuth("%s", err.Error())
	case errors.Is(err, ErrRateLimited):
		return errs.RateLimited("%s", err.Error())
	case errors.Is(err, ErrNotFound):
		return errs.NotFound("%s", err.Error())
	default:
		return err
	}
}
