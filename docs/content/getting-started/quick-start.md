---
title: "Quick start"
description: "From an empty terminal to a real search result, a real shelf, and a real record, in a handful of commands."
weight: 30
---

This walks the core loop: search the catalog, read a shelf, and look up a record
by id. The first two commands hit the open endpoints (autocomplete and RSS), so
they are quick and reliable. The third reads a `/book/show/` page, which is the
one the WAF sometimes challenges.

## 1. Search the catalog

```bash
goodread search "the hunger games" -n 10
```

Each row is a book or an author from the autocomplete endpoint. Want rich book
records instead of the thin autocomplete rows? Add `--books`:

```bash
goodread search dune --books --format json
```

`search` is the reliable starting point because the autocomplete endpoint is
open and not WAF-challenged.

## 2. Read a shelf

```bash
goodread shelf 1 --shelf read
```

Each row is a book on that reader's shelf, with its rating, review, ISBN, and
the dates it was added and read. By default `shelf` reads the public RSS feed,
which is rich and not challenged. Pass `--html` to walk the paginated HTML shelf
instead when you want more than the feed carries:

```bash
goodread shelf 1 --shelf read --html --max-pages 3
```

## 3. Look up a record by id

`book` takes an id or a full URL and fetches the book page:

```bash
goodread book 2767052
```

That is "The Hunger Games". The same record as JSON:

```bash
goodread book 2767052 --format json
```

`book` reads the `/book/show/` HTML page, which is the page the AWS WAF
sometimes challenges. If you see exit code 5 ("blocked"), the page was
challenged this time. Slow down, try again, or pass a signed-in session with
`--cookies` (see [troubleshooting](/reference/troubleshooting/)). The open
`search` and `shelf` endpoints above are not affected.

## 4. Classify without fetching

`id` turns a URL or id into an (entity, id) pair without touching the network,
which is handy in scripts:

```bash
goodread id https://www.goodreads.com/book/show/2767052
```

```
book	2767052
```

## 5. Compose

Output that pipes is the point. Pull the cover URLs off a shelf:

```bash
goodread shelf 1 --shelf read --format jsonl | jq -r .cover_url
```

Count the books an author has:

```bash
goodread author 153394 --fields name,books_count
```

## Where to next

You have the core loop. From here:

- [Books and authors](/guides/books-and-authors/) covers `book`, `author`,
  `series`, `similar`, and `reviews`, and the WAF caveat.
- [Search and discovery](/guides/search-and-discovery/) goes deep on `search`,
  `genre`, `list`, and the `id` classifier.
- [Shelves and quotes](/guides/shelves-and-quotes/) covers `shelf`, `quote`, and
  `user`.
- The [CLI reference](/reference/cli/) lists every command and flag.
