# goodread

A fast, friendly command line for public [Goodreads](https://www.goodreads.com)
data. One binary that turns books, authors, series, lists, quotes, users,
shelves, genres, reviews, and search into rich, structured records as a table,
JSON, JSONL, CSV, TSV, or plain URLs.

```
goodread search "the hunger games" -n 5
```

```
POSITION  ENTITY_TYPE  TITLE                                     URL
1         book         The Hunger Games (The Hunger Games, #1)   https://www.goodreads.com/book/show/2767052
2         book         Catching Fire (The Hunger Games, #2)      https://www.goodreads.com/book/show/6148028
3         book         Mockingjay (The Hunger Games, #3)         https://www.goodreads.com/book/show/7260188
4         author       Suzanne Collins                           https://www.goodreads.com/author/show/153394
5         book         The Hunger Games Trilogy Boxset           https://www.goodreads.com/book/show/7938275
```

Full documentation: [goodread-cli.tamnd.com](https://goodread-cli.tamnd.com).

> goodread is an independent, open-source tool. It is not affiliated with,
> endorsed by, or sponsored by Goodreads or Amazon. It reads only public pages,
> at a polite default rate.

## Why

Goodreads has no public API anymore, so pulling structured data out of it
usually means hand-rolling a scraper, guessing at selectors, and re-doing the
work every time a page changes. goodread puts the whole public surface behind
one tool with sensible defaults, real output formats, and pipelines that
compose. It reads each page JSON-LD first and falls back to HTML selectors, so a
book or an author comes back as a complete record, not a bag of strings.

It speaks to `www.goodreads.com` over plain HTTPS, with no API key and nothing
to sign up for. The binary is pure Go with no runtime dependencies.

## Install

```sh
go install github.com/tamnd/goodread-cli/cmd/goodread@latest
```

Or grab a prebuilt binary from the [releases page](https://github.com/tamnd/goodread-cli/releases),
install a Linux package (`deb`, `rpm`, `apk`), or pull the container image:

```sh
docker run --rm ghcr.io/tamnd/goodread search dune --books
```

Homebrew and Scoop:

```sh
brew install --cask tamnd/tap/goodread
scoop install goodread
```

Build from source:

```sh
git clone https://github.com/tamnd/goodread-cli
cd goodread-cli
make build      # produces ./bin/goodread
```

## Quick start

```sh
goodread search "the hunger games"        # books and authors that match
goodread search dune --books -f json      # rich book records, as JSON
goodread author 153394                     # an author profile
goodread shelf 1 --shelf read             # a reader's "read" shelf, with reviews
goodread quote 153394 -n 3                 # an author's quotes
goodread book 2767052                      # a full book record
```

## How it works

goodread reads the same pages your browser sees and normalizes each one into a
struct with explicit empty, zero, or `[]` for fields that are genuinely absent.
Most pages carry a JSON-LD block; goodread reads that first and uses CSS
selectors to fill in the rest. Responses are cached on disk (content-addressed
and gzipped) so a repeat call is instant and does not hit the network.

Goodreads sits behind an AWS WAF that intermittently challenges some HTML
pages. goodread leans on the open, un-challenged endpoints wherever it can:

- `search` and `search --books` read the **autocomplete JSON** endpoint, which
  returns rich book and author records and is not challenged.
- `shelf` reads the public **RSS feed** by default, which returns full rows
  (author, ISBN, rating, dates, review text) without a challenge.
- `seed` walks the **sitemap tree** advertised in `robots.txt` to discover URLs
  for bulk crawling.

The `book`, `similar`, and `reviews` commands read the `/book/show/` HTML page,
which is the one most likely to be challenged. When that happens goodread exits
cleanly with code 5 and suggests lending a signed-in session with `--cookies`
(a Netscape `cookies.txt` jar exported from your browser).

## Commands

| Command | What it does |
| --- | --- |
| `book <id\|url>...` | Fetch one or more full book records (`--with-reviews`) |
| `author <id\|url>...` | Fetch one or more author profiles |
| `series <id\|url>` | A series header, or its books with `--books` |
| `list <id\|url>` | A Listopia list header, or its books with `--books` |
| `quote <id\|url>` | Quotes from a quotes page, an author, or a book |
| `user <id\|url>` | A public reader profile |
| `shelf <id\|url>` | A reader's shelf via RSS (`--shelf`, `--html`, `--max-pages`) |
| `genre <slug\|url>` | A genre header, or its book ids with `--books` |
| `search <query>` | Search books and authors (`--books`, `--html`) |
| `reviews <id\|url>` | Reviews embedded on a book page |
| `similar <id\|url>` | Books similar to a given book |
| `id <url\|id>...` | Classify a URL or id into (entity, id) without fetching |
| `seed` | Discover sitemap categories, shards, and page URLs |
| `crawl` | Process the crawl queue (`--max`, `--parse`) |
| `db` | Inspect and export the local store (`info`, `count`, `get`, `export`, `vacuum`) |
| `cache` | Inspect and clear the page cache (`info`, `clear`, `path`) |
| `open <id\|url>` | Open a Goodreads page in the default browser |
| `info` | Show configuration, paths, and the disclaimer |
| `version` | Print version, commit, and build date |

## Output

Output is a table on a terminal and JSONL when piped, so it drops straight into
a pipeline. Pick any format explicitly with `-f`:

```sh
goodread search dune --books -f json        # pretty JSON array
goodread search dune --books -f jsonl        # one JSON object per line
goodread author 153394 -f csv                # CSV with a header row
goodread search dune -f url                   # just the URLs
goodread book 2767052 --fields title,isbn13,pages -f tsv
goodread author 153394 --template '{{.Name}} has {{.RatingsCount}} ratings'
```

Choose columns with `--fields`, drop the header with `--no-header`, and apply a
Go `text/template` per record with `--template`.

## Bulk crawling

For dataset-scale work, seed the queue from the sitemaps, crawl it, and export
the results from the local SQLite store:

```sh
goodread seed --type list --urls --max 200 --enqueue   # fill the queue
goodread crawl --parse                                  # fetch, cache, parse
goodread db count                                       # how many records
goodread db export --type list -o lists.jsonl           # dump them
```

The crawler is polite by default (2 workers, a 2s spacing) and you can tune it
with `--workers` and `--delay`.

## Exit codes

| Code | Meaning |
| --- | --- |
| 0 | success |
| 1 | error |
| 2 | usage error |
| 3 | no data (not found, empty result) |
| 4 | partial (some items in a batch failed) |
| 5 | blocked (a WAF challenge; try `--cookies`) |

## Configuration

State lives under `$XDG_DATA_HOME/goodread` (or `~/.local/share/goodread`),
overridable with `--data-dir` or `GOODREAD_DATA_DIR`. The page cache and the
SQLite store both sit there. Politeness and networking knobs (`--delay`,
`--workers`, `--timeout`, `--retries`, `--cache-ttl`, `--no-cache`, `--refresh`,
`--cookies`) are global flags on every command. Run `goodread info` to see the
resolved paths and `goodread <command> --help` for the full surface.

## Development

```sh
make build      # build ./bin/goodread
make test       # go test ./...
make vet        # go vet ./...
make fmt        # gofmt -s -w .
```

## License

[Apache-2.0](LICENSE).
