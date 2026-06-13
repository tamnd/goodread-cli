---
title: "goodread"
description: "A fast, friendly command line for public Goodreads data. Look up a book, an author, a series, or a shelf, search the catalog, pull quotes, and build datasets, all from one binary."
heroTitle: "Goodreads, from the command line"
heroLead: "goodread is a single pure-Go binary that turns public Goodreads pages into clean, structured records. Search the catalog, look up books and authors, read a shelf, collect quotes, and crawl in bulk, with no API key and nothing to sign up for."
heroPrimaryURL: "/getting-started/quick-start/"
heroPrimaryText: "Get started"
---

Goodreads has no public API anymore, so getting structured data out of it
usually means writing a scraper. goodread does that part for you: it reads a
public page and turns it into a record with real fields, JSON-LD first and an
HTML fallback when a page does not carry it.

```bash
goodread search "the hunger games" -n 10   # books and authors from autocomplete
goodread shelf 1 --shelf read              # a reader's shelf, from the public RSS feed
goodread author 153394                     # an author profile as a record
goodread quote 153394                      # that author's quotes
```

It talks to `www.goodreads.com` over plain HTTPS with no API key. The binary is
pure Go with no runtime dependencies. Output is a table on a terminal and JSONL
when piped, with json, csv, tsv, url, raw, `--fields`, and `--template` when you
want something else.

## What you can do with it

- **Search the catalog.** `goodread search` uses the open autocomplete endpoint
  to find books and authors fast, with `--books` for rich book records and
  `--html` for the full search page.
- **Look up records.** Fetch a book, author, series, list, genre, user, or quote
  by id or URL, and get a structured record with ratings, descriptions, ISBNs,
  genres, and cross-references.
- **Read shelves.** `goodread shelf` reads a reader's bookshelf from the public
  RSS feed by default, with every book, rating, and review on it.
- **Crawl in bulk.** Discover URLs from the sitemap with `seed`, queue them,
  `crawl` them into a local SQLite store, and export the records as JSONL.
- **Script it.** Stable exit codes, a `--template` for any line shape, and an
  `id` command that classifies a URL without fetching make goodread safe to
  drop into a pipeline.

## Independent and public-data only

goodread is an independent, open-source tool. It is not affiliated with,
endorsed by, or sponsored by Goodreads or Amazon. It reads only public pages, at
a polite default rate (a two second gap between requests, two workers).

## Where to go next

- New here? Start with the [introduction](/getting-started/introduction/) for
  the mental model, then the [quick start](/getting-started/quick-start/).
- Want to install it? See [installation](/getting-started/installation/).
- Looking for a specific task? The [guides](/guides/) cover books and authors,
  search and discovery, shelves and quotes, bulk crawling, and output formats.
- Need every flag? The [CLI reference](/reference/cli/) is the full surface.
