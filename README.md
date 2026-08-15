# goodread

A command line for public [Goodreads](https://www.goodreads.com) data.
One binary that reads books, works, authors, series, lists, genres, quotes and editions into structured records, stores them in SQLite as a graph, and writes them out as a table, JSON, JSONL, CSV, TSV, RDF or plain URLs.

```
goodread book 2767052
```

```
The Hunger Games (The Hunger Games, #1)
by Suzanne Collins

id          2767052
work        2792775
series      The Hunger Games #1
published   2008-10-14
publisher   Scholastic Press
format      Hardcover
pages       374
language    English
isbn13      9780439023481
genres      Young Adult, Dystopia, Fiction, Fantasy, Science Fiction, Romance
url         https://www.goodreads.com/book/show/2767052-the-hunger-games

4.35 average from 10,233,344 ratings, 275,272 text reviews
5 star  ████████████████████████████████████████ 5,635,655   55.1%
4 star  ██████████████████████                   3,103,721   30.3%
3 star  ███████                                  1,117,155   10.9%
2 star  █                                          242,710    2.4%
1 star                                             134,103    1.3%
```

Full documentation: [goodread-cli.tamnd.com](https://goodread-cli.tamnd.com).

> goodread is an independent, open source tool.
> It is not affiliated with, endorsed by, or sponsored by Goodreads or Amazon.
> It reads only public pages, at a polite default rate, and it obeys `robots.txt`.

## Why

Goodreads has no public API any more, so getting structured data out of it usually means writing a scraper, guessing at selectors, and doing it again the next time a page changes.
goodread puts the public surface behind one tool with a model that distinguishes a work from an edition, provenance on every number, and a local store you can query with SQL.

It needs no API key and nothing to sign up for.
The binary is pure Go with no runtime dependencies.

## Install

```sh
go install github.com/tamnd/goodread-cli/cmd/goodread@latest
```

Or take a prebuilt binary from the [releases page](https://github.com/tamnd/goodread-cli/releases), install a Linux package (`deb`, `rpm`, `apk`), or pull the container image:

```sh
docker run --rm ghcr.io/tamnd/goodread book 2767052
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
make build
```

## Quick start

```sh
goodread book 2767052                  # a book, with its work, series and contributors
goodread work 2792775                  # the work behind it, through its best edition
goodread author 153394                 # an author and their books
goodread series 45175                  # a series in series order
goodread editions 2792775              # every printing, which is where the ISBNs are
goodread lookup 9780439023481          # an ISBN13 resolved to a book
goodread genre fantasy                 # a genre and the genres it is related to
```

## A book is not a work

This is the distinction the whole tool is built on.
A work is what somebody wrote and a book is one printing of it, and conflating them is the most common error in book data.

The Hunger Games is one work with 527 editions.
Each edition has its own ISBN, page count, publisher and rating population, and the work has the original title, the awards, the places and the characters.
Ask for `goodread book` when you want a printing and `goodread work` when you want the thing itself.

Every number says which population it came from.
An average rating on a book page is that edition's readers, and an average rating on a work is all of them, and a record that does not say which one it holds is not usable for anything quantitative.

## What it reads, and what it does not

goodread obeys `robots.txt`.
Every readable path is a registered op and nothing builds a URL at the call site, so `goodread robots` can list every surface the tool can reach and say which side of the line it is on.

Four surfaces are disallowed by Goodreads' rules and are not read by default: the search page, shelves, reviews and `/work/<id>`.
There is a `--no-robots` flag for a person who has decided it is their call.
It warns once, the pace floor still applies, and a crawl with it needs `--yes` as well, because a crawl is the one place where an override multiplies.

The MCP server never reads a disallowed surface at all and `--no-robots` has no effect on it.
The reason is not that the flag is wrong.
It is that the flag's whole justification is a person deciding it is their call, and a model calling a tool is not that person deciding.

## Commands

| Command | What it does |
| --- | --- |
| `book <id\|isbn\|url>...` | One or more books, with their work, series and contributors |
| `work <id\|url>` | A work, read through its best edition |
| `author <id\|url>...` | An author and the books their page lists |
| `series <id\|url>` | A series and its books in series order |
| `editions <work>` | Every edition of a work |
| `quotes <work\|author>` | A work's quotes, or an author's with `--author` |
| `genre <slug>` | A genre, its featured books and its related genres |
| `list <id>` | A Listopia list and its ranked books |
| `lookup <isbn\|asin\|title>` | Resolve an identifier to a book, through the open autocomplete route |
| `search <query>` | Search books and authors |
| `find <text>` | Full text search over the local store, offline |
| `query <sql>` | One read only SQL statement over the local store |
| `graph <uri>` | Walk the local graph around a node |
| `export` | Write the store out as JSONL or RDF |
| `crawl` | Walk out from a set of seeds and build a store |
| `seed` | Discover sitemap categories, shards and page URLs |
| `mcp` | Run as an MCP server over stdio |
| `robots` | The rules, and which surfaces they permit |
| `extraction` | Which rung of the ladder answers for each field |
| `verify` | Check the extractor against the pinned captures |
| `db` | Inspect and export the local store |
| `cache` | Inspect and clear the page cache |
| `id <url\|id>...` | Classify a URL or id without fetching |
| `open <id\|url>` | Open a Goodreads page in the default browser |
| `info` | Configuration, paths and the disclaimer |
| `version` | Version, commit and build date |

## Output

Output is a table on a terminal and JSONL when piped, so it drops into a pipeline without a flag.
Pick a format explicitly with `-f`:

```sh
goodread book 2767052 -f json
goodread editions 2792775 -f jsonl
goodread author 153394 -f csv
goodread series 45175 -f url
goodread book 2767052 --fields title,isbn13,num_pages -f tsv
goodread book 2767052 --template '{{.Book.Title}} isbn {{.Book.ISBN13}}'
```

Choose columns with `--fields`, drop the header with `--no-header`, and apply a Go `text/template` per record with `--template`.
`--fields` names the JSON keys and `--template` walks the Go struct, so the same field is `isbn13` in one and `.Book.ISBN13` in the other.

## The store is a graph

Everything read is folded into a SQLite store as nodes and edges.
Nodes are keyed by a `gr:` URI built from the legacy id, so `gr:book/2767052` is the same book whatever URL it was reached through.
Edges are a closed set of twelve predicates and a name outside that set is refused at the write.

```sh
goodread graph gr:book/2767052 --depth 2
goodread graph gr:book/2767052 --edges
goodread query "select title, isbn13 from books where num_pages > 500"
goodread query "select src from edges where dst = 'gr:author/153394' and predicate = 'contributed_by'"
goodread export --to rdf --kind book --kind work > books.ttl
```

Cycles are normal rather than an error case.
Book to work to best edition closes in three hops on most popular titles, so every traversal carries a visited set from the first hop.

## Crawling

A crawl walks out from a set of seeds, following the edges each record named.

```sh
goodread crawl --seed gr:author/153394 --depth 2 --dry-run
goodread crawl --seed gr:author/153394 --depth 2
```

It is one connection at the site's pace and there is no flag to make it more, because a crawler that can be told to open ten connections will be.
`--dry-run` prints the plan and reads nothing, which is how you find out a crawl is twelve hours before starting it.
The frontier lives in the store, so an interrupted crawl continues where it stopped when you run the same command again, and the pages it already read come back out of the cache.

## MCP

`goodread mcp` serves eleven read tools over stdio: `book_get`, `work_get`, `author_get`, `series_get`, `editions_list`, `quotes_list`, `genre_get`, `list_get`, `book_lookup`, `store_find` and `store_query`.

There is no `search`, no `shelf` and no `reviews`, and `--no-robots` has no effect on the server.
An override a model can trigger is not an override, it is a default.

## Exit codes

| Code | Meaning |
| --- | --- |
| 0 | success |
| 1 | an error nothing else classified |
| 2 | usage, including a config file that will not load |
| 3 | network, meaning the site never answered |
| 4 | the site answered and the answer was an error or a block |
| 5 | extraction failed, or a record did not reconcile |
| 6 | not found |
| 7 | refused because `robots.txt` disallows the path |
| 8 | `robots.txt` could not be read, so nothing can be checked against it |

## Configuration

State lives under `$XDG_DATA_HOME/goodread`, or `~/.local/share/goodread`, and `--data-dir` or `GOODREAD_DATA_DIR` moves it.
The page cache and the SQLite store both sit there.

The networking and politeness flags are global on every command: `--delay`, `--timeout`, `--retries`, `--cache-ttl`, `--no-cache` and `--refresh`.
`--delay` has a floor of one second and a value below it is clamped with a note rather than honoured or silently dropped.
Run `goodread info` for the resolved paths and `goodread <command> --help` for the rest.

There is no `--workers` and no other parallelism flag, on purpose.

## Development

```sh
make build
make test
make vet
make fmt
```

## License

[Apache-2.0](LICENSE).
