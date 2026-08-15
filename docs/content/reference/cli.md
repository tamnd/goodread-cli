---
title: "CLI reference"
description: "Every goodread command and every flag, grouped by what it is for."
weight: 10
---

```
goodread [command] [--flags]
```

Every command accepts the [global flags](/reference/configuration/#global-flags).
The tables below list only the flags each command adds.

## Reading Goodreads

### book

```
goodread book <id|url> [id|url ...] [--flags]
```

Fetch one or more books.
A book is one printing, not the work behind it.

| Flag | Meaning |
|---|---|
| `--check` | Read the book into the model and reconcile its numbers |
| `--with-reviews` | Also fetch the reviews embedded on the page |

```bash
goodread book 2767052
goodread book 2767052 5907 --format csv
goodread book https://www.goodreads.com/book/show/2767052 --json
goodread book 2767052 --check
```

### work

```
goodread work <work-id|url>
```

Read a work through its best edition.
`/work/<id>` is disallowed, so this reads the editions page and then the first edition's own page, which carries the work with its places, characters and awards.
Two requests, and the record says which edition it came through.

```bash
goodread work 2792775
```

### author

```
goodread author <id|url> [id|url ...] [--flags]
```

| Flag | Meaning |
|---|---|
| `--books` | List the author's books instead of the author |

### series

```
goodread series <id|url> [--flags]
```

| Flag | Meaning |
|---|---|
| `--books` | List the series' books instead of the series header |

### editions

```
goodread editions <work-id|url> [--flags]
```

Every printing of a work, which is where the ISBNs are.

| Flag | Default | Meaning |
|---|---|---|
| `--page` | `1` | Which page of editions to read |
| `--pages` | `1` | How many pages to read from `--page` onward |

### quotes

```
goodread quotes <work-id|author-id|url> [--flags]
```

| Flag | Default | Meaning |
|---|---|---|
| `--author` | off | Read the author's quotes page instead of a work's |
| `--page` | `1` | Which page of quotes to read |
| `--pages` | `1` | How many pages to read from `--page` onward |

### genre

```
goodread genre <slug|url> [--flags]
```

| Flag | Meaning |
|---|---|
| `--books` | List the featured books instead of the genre |
| `--related` | List the related genres, which is the graph this page alone publishes |

### list

```
goodread list <id|url> [--flags]
```

| Flag | Default | Meaning |
|---|---|---|
| `--books` | off | List the list's books instead of the list header |
| `--page` | `1` | Which page of the list to read |

### lookup

```
goodread lookup <isbn|isbn13|asin|title> [--flags]
```

Resolve an identifier to a book, through `/book/auto_complete`, which needs no key and is allowed.
This is the allowed replacement for site search.

| Flag | Meaning |
|---|---|
| `--ids` | Print the matching ids and stop, without reading the book pages |

### similar

```
goodread similar <book-id|url>
```

The books Goodreads lists as similar to the one you name.

## Disallowed by robots.txt

These read surfaces Goodreads' `robots.txt` disallows, so they refuse without `--no-robots`.
See [robots.txt and what it costs](/guides/robots-and-limits/).

### search

```
goodread search <query> [--flags]
```

Autocomplete by default, which is allowed.
`--deep` also reads `/search`, which is not.

| Flag | Meaning |
|---|---|
| `--books` | Return rich book records from autocomplete |
| `--deep` | Read `/search` as well, which needs `--no-robots` |

### shelf

```
goodread shelf <user-id|url> [--flags]
```

Both routes are disallowed and need `--no-robots`.
The RSS feed carries more per row than the rendered page does, and `--html` is the only way to get a whole shelf.

| Flag | Default | Meaning |
|---|---|---|
| `--shelf` | `read` | Shelf name: `read`, `currently-reading`, `to-read` or a custom one |
| `--books` | off | List the rows instead of the shelf header |
| `--html` | off | Walk the paginated HTML shelf instead of the RSS feed |
| `--max-pages` | `1` | Maximum pages to walk in `--html` mode, `0` is all |

### reviews

```
goodread reviews <book-id|url> [--flags]
```

The thirty reviews the book page embeds are allowed.
`--all` walks `/book/reviews`, which is not, so it needs `--no-robots` and `--yes`.

| Flag | Default | Meaning |
|---|---|---|
| `--all` | off | Walk `/book/reviews` as well |
| `--max-pages` | `10` | Pages of `/book/reviews` to walk with `--all` |
| `--yes` | off | Go ahead with the requests `--all` costs |

### user

```
goodread user <id|url>
```

A public reader profile.

## The local store

### find

```
goodread find <text> [--flags]
```

Full text search over the local store, offline.

| Flag | Meaning |
|---|---|
| `--kind` | Limit to one record kind: book, author, series, list, genre, user |

### query

```
goodread query <sql> [--flags]
```

One read only SQL statement over the local store.
`delete`, `update`, `drop` and `insert` are refused.

| Flag | Meaning |
|---|---|
| `--tables` | List the tables and views in the store and stop |

### graph

```
goodread graph <uri|id> [--flags]
```

Walk the local graph around a node.
A walk of more than 500 nodes needs `--yes`.

| Flag | Meaning |
|---|---|
| `--edges` | Print the edges rather than the nodes |
| `--yes` | Go ahead with a walk of more than 500 nodes |

`--depth` is global and applies here.

### export

```
goodread export [--flags]
```

| Flag | Default | Meaning |
|---|---|---|
| `--to` | `jsonl` | `jsonl` or `rdf` |
| `--kind` | all | Limit to these node kinds, repeatable |
| `--edges` | off | Write the edge table instead of the nodes, jsonl only |

### db

```
goodread db [command]
```

The v0.2.0 records table, which lives beside the graph tables.

| Subcommand | Meaning |
|---|---|
| `db info` | Summarize stored records and the crawl queue |
| `db count [entity-type]` | Count stored records, all types or one |
| `db get <entity-type> <id>` | Print a stored record as JSON |
| `db export [--flags]` | Export stored records to JSONL or NDJSON |
| `db vacuum` | Compact the store database file |

## Crawling

### crawl

```
goodread crawl [--flags]
```

Walk out from a set of seeds and build a store.
One connection at the usual pace, and there is no parallelism flag.

| Flag | Default | Meaning |
|---|---|---|
| `--seed` | none | A `gr:` URI, a Goodreads URL or an id to start from, repeatable |
| `--seed-file` | none | A file of seeds, one per line, `#` for a comment |
| `--from-sitemap` | none | Seed from a sitemap category: author, list, quote, genre, user |
| `--max` | `0` | Stop after this many pages, `0` is until the frontier is empty |
| `--dry-run` | off | Print the plan and the request count, and read nothing |
| `--reset` | off | Clear the frontier and start over, keeping the records |
| `--yes` | off | Required alongside `--no-robots` |

`--depth` is global and defaults to `1` hop of expansion here.

### seed

```
goodread seed [--flags]
```

Discover sitemap categories, shards and page URLs.

| Flag | Meaning |
|---|---|
| `--type` | Sitemap category to expand: author, list, quote, genre, user and more |
| `--urls` | Drill into shards and emit page URLs |
| `--max` | Cap the number of rows emitted, `0` is no limit |
| `--enqueue` | Put the discovered URLs on the crawl frontier, with `--urls` |

### cache

```
goodread cache [command]
```

| Subcommand | Meaning |
|---|---|
| `cache info` | Show cache location, file count and size |
| `cache path <url>` | Print the cache path for a URL |
| `cache clear` | Delete the entire page cache |

## Serving

### mcp

```
goodread mcp
```

Run as an MCP server over stdio, with eleven read tools.
No search, no shelf, no reviews, and `--no-robots` has no effect on it.
See [the MCP server](/guides/mcp/).

## Inspecting

### robots

```
goodread robots [command]
```

The rules, and which surfaces they permit.

| Subcommand | Meaning |
|---|---|
| `robots check <url\|path>` | Say whether one URL may be fetched, and which rule decides |

### extraction

```
goodread extraction
```

Which rung of the ladder answers for each surface, and how many CSS selectors are in play on each.

### verify

```
goodread verify [--flags]
```

Check the extractor against the pinned captures.

| Flag | Meaning |
|---|---|
| `--strict` | Exit non-zero when a known field went missing |
| `--sample` | Read this many live book pages instead of the pinned captures |

### id

```
goodread id <url|id> [url|id ...]
```

Classify a URL or id into (entity, id) without fetching.
Pure local work, never blocked, made for scripts.

### open

```
goodread open <id|url>
```

Open a Goodreads page in the default browser.

### info

```
goodread info
```

Configuration, resolved paths and the affiliation disclaimer.

### version

```
goodread version
```

Version, commit and build date.

### completion

```
goodread completion [bash|zsh|fish|powershell]
```

Generate the shell completion script.

## Exit codes

| Code | Meaning |
|---|---|
| `0` | success |
| `1` | an error nothing else classified |
| `2` | usage, including a config file that will not load |
| `3` | network, meaning the site never answered |
| `4` | the site answered and the answer was an error or a block |
| `5` | extraction failed, or a record did not reconcile |
| `6` | not found |
| `7` | refused because `robots.txt` disallows the path |
| `8` | `robots.txt` could not be read, so nothing can be checked against it |
