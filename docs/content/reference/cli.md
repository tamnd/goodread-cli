---
title: "CLI"
description: "Every command and subcommand, with the flags that matter and one example each."
weight: 10
---

```
goodread <command> [args] [flags]
```

Run `goodread <command> --help` for the full flag list on any command. This page
is the map. Commands that read the `/book/show/` page (`book`, `similar`,
`reviews`) are the ones the AWS WAF can challenge; the rest lean on open
endpoints. See [troubleshooting](/reference/troubleshooting/).

## Commands

| Command | What it does |
|---|---|
| `book` | Fetch one or more books from `/book/show/` (WAF-prone) |
| `author` | Fetch one or more author profiles |
| `series` | A series header, or its books with `--books` |
| `list` | A Listopia list header, or its books with `--books` |
| `quote` | Quotes from a quotes page, author, or book |
| `user` | A public reader profile |
| `shelf` | A reader's bookshelf (open RSS feed by default) |
| `genre` | A genre header, or its book ids with `--books` |
| `search` | Search books and authors via the open autocomplete endpoint |
| `reviews` | Reviews embedded on a book page (WAF-prone) |
| `similar` | Books similar to a given book (WAF-prone) |
| `id` | Classify a URL or id into (entity, id) without fetching |
| `seed` | Discover the sitemap tree from `robots.txt` |
| `crawl` | Process the crawl queue: fetch, cache, optionally parse |
| `db` | Inspect and export the local SQLite store |
| `cache` | Inspect and clear the on-disk page cache |
| `open` | Open a Goodreads page in the default browser |
| `info` | Show configuration, paths, and the affiliation disclaimer |
| `version` | Print version, commit, and build date |

## book

```
goodread book <id|url> [id|url ...] [flags]
```

Fetches one or more books from the `/book/show/` page. `--with-reviews` also
pulls the reviews embedded on the page. WAF-prone; `--cookies` helps when
challenged.

```bash
goodread book 2767052 --format json
```

## author

```
goodread author <id|url> [id|url ...]
```

Fetches one or more author profiles.

```bash
goodread author 153394 1077326 --format csv
```

## series

```
goodread series <id|url> [--books]
```

The series header, or the books in it with `--books`.

```bash
goodread series 73758 --books
```

## list

```
goodread list <id|url> [--books] [--max-pages N]
```

A Listopia list header, or its books with `--books`. Lists hold 100 books per
page; `--max-pages N` walks that many (default 1, `0` for the whole list).

```bash
goodread list 1.Best_Books_Ever --books
goodread list 1.Best_Books_Ever --books --max-pages 3
```

## quote

```
goodread quote <url|author-id|book-id> [--max-pages N]
```

Quotes from a quotes-page URL, an author id, or a book id. Quote pages hold 30
per page; `--max-pages N` walks that many (default 1, `0` for all).

```bash
goodread quote https://www.goodreads.com/work/quotes/2792775
goodread quote 153394 --max-pages 3
```

## user

```
goodread user <id|url>
```

A public reader profile.

```bash
goodread user 1
```

## shelf

```
goodread shelf <user-id|url> [flags]
```

A reader's bookshelf. `--shelf NAME` picks the shelf (`read`,
`currently-reading`, `to-read`, or a custom name; default `read`). Reads the
open RSS feed by default; `--html` walks the paginated HTML shelf instead, and
`--max-pages N` caps how far (default 1, html mode).

```bash
goodread shelf 1 --shelf read --html --max-pages 3
```

## genre

```
goodread genre <slug|url> [--books]
```

A genre header, or its book ids with `--books`.

```bash
goodread genre fantasy --books
```

## search

```
goodread search <query> [flags]
```

Searches books and authors through the open autocomplete endpoint. `--books`
returns rich book records from the autocomplete payload. `--html` runs the full
HTML search (paginated).

```bash
goodread search "the hunger games" -n 10
```

## reviews

```
goodread reviews <book-id|url>
```

Reviews embedded on a book's page (WAF-prone).

```bash
goodread reviews 2767052
```

## similar

```
goodread similar <book-id|url>
```

Books similar to the given book (WAF-prone).

```bash
goodread similar 2767052
```

## id

```
goodread id <url|id> [url|id ...]
```

Classifies each argument into an (entity, id) pair without fetching. Pure local,
never blocked.

```bash
goodread id https://www.goodreads.com/book/show/2767052
```

## seed

```
goodread seed [flags]
```

Discovers the sitemap tree advertised in `robots.txt`. No flags lists the
categories. `--type <category>` lists that category's gzipped shard sitemaps.
`--urls` drills into shards and emits page URLs (cap with `--max`). `--enqueue`
enqueues discovered URLs into the crawl queue (with `--urls`).

```bash
goodread seed --type quote --urls --max 50 --enqueue
```

## crawl

```
goodread crawl [flags]
```

Processes the crawl queue: fetch, cache, and with `--parse` parse each page into
the records table. `--max N` caps the count (`0` drains the queue). Uses
`--workers` and `--delay`. Exit 3 if nothing processed, exit 4 if some failed.

```bash
goodread crawl --max 50 --parse
```

## db

| Subcommand | Does |
|---|---|
| `db info` | Summarize stored records and the crawl queue |
| `db count [entity-type]` | Count stored records |
| `db get <entity-type> <id>` | Print a stored record as JSON |
| `db export [--type T] [-o file] [--format jsonl]` | Export stored records |
| `db vacuum` | Reclaim space in the store |

```bash
goodread db export --type quote -o quotes.jsonl --format jsonl
```

## cache

| Subcommand | Does |
|---|---|
| `cache info` | Location, file count, and size |
| `cache clear` | Remove every cached page |
| `cache path <url>` | Print the cache file path for a URL |

```bash
goodread cache info
```

## Meta

| Command | Does |
|---|---|
| `open <id\|url>` | Open a Goodreads page in the default browser |
| `info` | Show configuration, paths, and the affiliation disclaimer |
| `version` | Print version, commit, and build date |

```bash
goodread open 2767052
```

## Global flags

These apply to every command. See [configuration](/reference/configuration/) for
the full list and their defaults.

| Flag | Meaning |
|---|---|
| `-f, --format` | Output format (default table on a TTY, jsonl piped) |
| `--fields` | Comma-separated fields to show |
| `--no-header` | Omit the header row in table/csv output |
| `--template` | Go text/template applied per record |
| `--color` | `auto`, `always`, or `never` |
| `-n, --limit` | Maximum rows (`0` means all) |
| `-q, --quiet` | Suppress progress output |
| `--workers` | Concurrency (default 2) |
| `--delay` | Minimum delay between requests (default 2s) |
| `--timeout` | Per-request timeout (default 30s) |
| `--retries` | Retry attempts (default 3) |
| `--cache-ttl` | Cache lifetime (default 24h) |
| `--no-cache` | Bypass the on-disk cache for this run |
| `--refresh` | Force a re-fetch, ignoring the cache |
| `--data-dir` | Root data directory (env `GOODREAD_DATA_DIR`) |
| `--store` | SQLite store path (default `<data-dir>/goodread.db`) |
| `--cookies` | Netscape cookie jar to lend a signed-in session |
