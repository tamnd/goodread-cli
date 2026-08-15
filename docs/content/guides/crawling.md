---
title: "Crawling"
description: "Walk out from a set of seeds, follow the edges, build a store, and pick up where you stopped."
weight: 30
---

For more than a page at a time, `crawl` reads a seed, stores it, follows the edges the record named, and repeats until `--depth` is spent.

The important thing about it is what it does not have.
It is one connection at the usual pace, and there is no `--workers`, no `--concurrency` and no `--parallel`, because a crawler that can be told to open ten connections will be.
The crawler is therefore never faster than any other command, which is the point.

## Look before you leap

```bash
goodread crawl --seed gr:author/153394 --depth 2 --dry-run
```

`--dry-run` prints the plan and reads nothing.
It tells you how many requests the crawl is worth and roughly how long that takes at the current `--delay`, both stated as "at least", since a crawl discovers as it goes.
This is how you find out a crawl is twelve hours before starting it rather than after.

## Seeds

A seed is a `gr:` URI, a Goodreads URL or a bare id, and `--seed` is repeatable:

```bash
goodread crawl --seed gr:author/153394 --seed gr:series/45175 --depth 1
goodread crawl --seed https://www.goodreads.com/book/show/2767052
goodread crawl --seed 2767052
```

A file of seeds works too, one per line, with blank lines and `#` comments skipped:

```bash
goodread crawl --seed-file seeds.txt --depth 1 --max 200
```

And a sitemap category can seed it directly:

```bash
goodread crawl --from-sitemap author --max 50
```

Without `--max`, a sitemap seed takes the first 100 URLs and says so, rather than queueing a few hundred thousand pages because nobody said not to.

## How it expands

Expansion goes through the edge table rather than the HTML.
A page is read, the record is folded into nodes and edges, and the neighbours of the node just written go on the frontier.
That means the crawl follows exactly the relationships the record model records, and anything the model does not have an edge for is not somewhere the crawl can wander to.

The frontier is keyed by URI and not by URL, so `/book/show/2767052` and `/book/show/2767052-the-hunger-games` are one entry rather than two.

## Stopping and resuming

```bash
goodread crawl --seed gr:author/153394 --depth 3 --max 500
```

`--max` stops after that many pages, and `0` means run until the frontier is empty.

The frontier lives in the store, so an interrupted crawl continues where it stopped when you run the same command again, and the pages it already read come back out of the cache rather than off the site.
Ctrl-C is a clean stop rather than a failure, and it exits zero.

To start over without throwing away what you collected:

```bash
goodread crawl --reset --seed gr:author/153394
```

`--reset` clears the frontier and keeps the records.

## What a run tells you

The summary is per kind: how many books, works, authors and the rest were written, then requests made, pages served from cache, errors, how much of the frontier is left, elapsed time and the size of the store.
Up to five failures are printed with the reason recorded for each.

A run where every page failed exits non-zero.
A run that lost three of four hundred does not, because that is a normal crawl and not a broken one.

## The sitemap

`seed` walks the sitemap tree Goodreads advertises in `robots.txt`, which is how you find URLs without knowing an id.

```bash
goodread seed                                     # the categories
goodread seed --type quote                        # that category's gzipped shard sitemaps
goodread seed --type quote --urls --max 50        # actual page URLs
goodread seed --type quote --urls --max 50 --enqueue
```

`--enqueue` puts the URLs on the crawl frontier instead of only printing them, so `seed` then `crawl` is the two-step version of `crawl --from-sitemap`.

## The page cache

Every fetch goes through an on-disk cache, so a re-crawl does not re-fetch pages that have not changed.

```bash
goodread cache info
goodread cache path https://www.goodreads.com/book/show/2767052
goodread cache clear
```

The TTL defaults to 24 hours.
`--no-cache` bypasses it for one run and `--refresh` forces a re-fetch and rewrites the entry.

A crawl leaning on the cache is the polite thing rather than the lazy thing: the second run of a crawl over the same seeds costs the site nothing.

## Pace

`--delay` defaults to two seconds and one second is a hard floor.
A value below it is clamped with a note on stderr, rather than honoured or silently dropped.

`--no-robots` on a crawl applies to every request the crawl makes rather than to one page, so it needs `--yes` as well.
See [robots.txt and what it costs](/guides/robots-and-limits/).

## The whole pipeline

```bash
goodread crawl --from-sitemap quote --max 100 --dry-run
goodread crawl --from-sitemap quote --max 100
goodread query "select count(*) from quotes"
goodread export --to jsonl --kind quote > quotes.jsonl
```
