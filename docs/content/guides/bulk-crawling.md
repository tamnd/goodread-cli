---
title: "Bulk crawling"
description: "Discover URLs from the sitemap, queue them, crawl into a local SQLite store, and export the records."
weight: 40
---

For more than a page at a time, goodread has a small pipeline: discover URLs from
the sitemap, enqueue them, crawl the queue into a local SQLite store, and export
what you collected. Everything lands in one database file under your data dir.

## 1. Discover with seed

`seed` reads the sitemap tree that Goodreads advertises in its `robots.txt`. With
no flags it lists the sitemap categories:

```bash
goodread seed
```

```
author
list
quote
genre
user
...
```

`--type` drills into one category's gzipped shard sitemaps:

```bash
goodread seed --type list
```

`--urls` drills further and emits the actual page URLs, and `--max` caps how
many you pull:

```bash
goodread seed --type quote --urls --max 50
```

## 2. Enqueue

Add `--enqueue` to put the discovered URLs into the crawl queue instead of just
printing them:

```bash
goodread seed --type quote --urls --max 50 --enqueue
```

## 3. Crawl the queue

`crawl` drains the queue: it fetches each URL, caches the page, and with
`--parse` also parses it into the records table. `--max` caps how many to
process (`0` drains the whole queue). It uses the global `--workers` and
`--delay`, so it stays polite:

```bash
goodread crawl --max 50 --parse
```

It reports how many it processed and failed. Exit code 3 means nothing was
processed; exit code 4 means some failed (see
[troubleshooting](/reference/troubleshooting/)).

## 4. Inspect and export the store

`db` works with the local SQLite store:

```bash
goodread db info                       # summarize records and the queue
goodread db count quote                # how many quotes are stored
goodread db get quote 12345            # one stored record as JSON
goodread db export --type quote -o quotes.jsonl --format jsonl
goodread db vacuum                     # reclaim space
```

`db export` writes every stored record (or just one `--type`) to a file or
stdout.

## The page cache

Every fetch goes through an on-disk cache (content-addressed, gzip), so a
re-crawl does not re-fetch pages that have not changed. `cache` manages it:

```bash
goodread cache info                                                  # location, file count, size
goodread cache path https://www.goodreads.com/book/show/2767052      # the cache file for a URL
goodread cache clear                                                 # remove every cached page
```

The cache TTL defaults to 24 hours. Bypass it for one run with `--no-cache`, or
force a re-fetch with `--refresh`.

## The whole pipeline

Put together, collecting a slice of quotes looks like this:

```bash
goodread seed --type quote --urls --max 100 --enqueue
goodread crawl --parse
goodread db export --type quote -o quotes.jsonl --format jsonl
```

The store and cache both live under the data dir; point that elsewhere with
`--data-dir` or `GOODREAD_DATA_DIR`, and point the database file alone with
`--store`. See [configuration](/reference/configuration/).
