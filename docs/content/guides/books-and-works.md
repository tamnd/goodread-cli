---
title: "Books, works and editions"
description: "Read a printing, the work behind it, every edition of it, and the people, series, genres, lists and quotes attached."
weight: 10
---

The read side of goodread.
Every command here takes a bare id or a full Goodreads URL, and every one of them is on a surface `robots.txt` allows.

## A book is one printing

```bash
goodread book 2767052
goodread book 2767052 --json
goodread book 2767052 5907 --format csv
goodread book https://www.goodreads.com/book/show/2767052-the-hunger-games
```

A book record carries the fields the printing has: `title`, `title_complete`, `description` and `description_stripped`, `isbn`, `isbn13`, `asin`, `format`, `num_pages`, `publisher`, `publication_time`, `language`, `image_url`, a `work` reference, `series` entries, `contributors`, `genres`, `stats` and the raw `links` block with the buy and availability URLs on it.

Two things about that record are worth knowing before you build on it.

**A field the read did not find is absent, not zero.**
A book with no page count and a book with zero pages are different facts, and audiobooks legitimately have neither.
So `num_pages` is a pointer in the model and simply missing in the JSON when the page did not carry it.

**Contributors carry their role.**
`primaryContributorEdge` and `secondaryContributorEdges` each name what the person did, so an illustrator comes back as an illustrator.
Flattening both into a list of names, which is what v0.2.0 did, records an illustrator as an author, and nothing downstream can detect that.

### Reconciling the numbers

`--check` reads the book into the model and reconciles it: it derives the mean from the five star buckets and compares it against the published average.

```bash
goodread book 2767052 --check
```

That is the check that catches a reversed distribution array, which is exactly the sort of thing nobody notices for six months.

### Depth

`--depth` says how much to read: `quick`, `meta` (the default), `full` or `deep`.
`--with-reviews` also pulls the reviews embedded on the page, which is about thirty of them.

## A work is the thing itself

```bash
goodread work 2792775
```

A work carries `original_title`, `publication_time`, `awards_won`, `places`, `characters`, a `best_book` reference, an edition count and work-level `stats`.
Places and characters are first class here rather than strings, because "which books are set in Dublin" should be a query and not a grep.

`/work/<id>` is disallowed by `robots.txt`.
So `work` reads the editions page, which an explicit `Allow` permits, and then the first edition's own book page, which carries the work.
That costs two requests and the record says which edition it came through, so the route is visible rather than implied.

## Editions are where the ISBNs live

```bash
goodread editions 2792775
goodread editions 2792775 --page 2 --pages 3
```

One row per printing, with the edition count on the record.
A work has no ISBN and a book has only its own, so this is the command when you want them all.
Paging is explicit: `--page` says where to start and `--pages` says how many from there.

## Which population a number came from

Every `stats` block carries a `via` field, and it is never omitted.
A book page publishes that edition's readers and a work read publishes all of them, and they are different populations with the same field names.
Averaging across records without reading `via` mixes the two, and nothing about the numbers themselves would tell you.

## Authors

```bash
goodread author 153394
goodread author 153394 --books
goodread author 153394 1077326 --format csv
```

An author record carries the profile the page publishes and, with `--books`, one row per book the page lists.

## Series

```bash
goodread series 45175
goodread series 45175 --books
```

The header carries the series and `--books` gives its books in series order.

Series positions are kept as strings as well as numbers.
Goodreads uses `2.5` for a novella and ranges like `1-3` for an omnibus, and a float alone cannot carry the second one, so `position` is the raw string and `number` is the parsed form when there is one.
A range has no number, and that is not an error.

## Genres and lists

```bash
goodread genre fantasy
goodread genre fantasy --books
goodread genre fantasy --related
goodread list 1.Best_Books_Ever
goodread list 1.Best_Books_Ever --books --page 2
```

`--related` is worth knowing about: the related genres are a graph that this page alone publishes, and nothing else on the site gives you it.

## Quotes

```bash
goodread quotes 2792775
goodread quotes 153394 --author
goodread quotes 2792775 --page 2 --pages 3
```

`quotes` reads a work's quotes page by default and an author's with `--author`.

## Resolving an identifier

```bash
goodread lookup 9780439023481
goodread lookup B002MQYOFW
goodread lookup "the hunger games"
goodread lookup "the hunger games" --ids
```

`lookup` goes through `/book/auto_complete`, which `robots.txt` allows, needs no key and carries enough to resolve an identifier to a book.
`--ids` prints the matching ids and stops, without spending a request per book page.

## What needs a flag

`search --deep`, `shelf`, `reviews --all` and `/work/<id>` are on surfaces `robots.txt` disallows, so they refuse by default.
See [robots.txt and what it costs](/guides/robots-and-limits/).
