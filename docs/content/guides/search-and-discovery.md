---
title: "Search and discovery"
description: "Search the catalog through the open autocomplete endpoint, browse genres and lists, and classify URLs."
weight: 20
---

How you find things on Goodreads without a starting id: search the catalog,
browse a genre or a Listopia list, and turn a URL you already have into an
(entity, id) pair.

## Search

```bash
goodread search "the hunger games" -n 10
```

`search` uses the autocomplete JSON endpoint, which is open and not
WAF-challenged, so it is the reliable way to find books and authors. Each row is
a thin match (a title, an author, an id). For rich book records straight from
the autocomplete payload, add `--books`:

```bash
goodread search dune --books --format json
```

When you want the full search page (with pagination) instead of autocomplete,
add `--html`:

```bash
goodread search "ursula le guin" --html -n 20
```

The HTML path reads search result pages, so it is more likely to meet a WAF
challenge than the autocomplete default. Reach for it only when you need what
the full page carries.

## Genres

```bash
goodread genre fantasy            # the genre header
goodread genre fantasy --books    # the book ids in the genre
```

A genre is named by slug (or URL). The header carries `slug`, `name`,
`description`, `books_count`, and `book_ids`. With `--books` you get the book
ids, ready to feed into `book`.

## Listopia lists

```bash
goodread list 1.Best_Books_Ever            # the list header
goodread list 1.Best_Books_Ever --books    # the books on the list
```

A list is named by its id (which includes the slug) or URL. The header carries
`list_id`, `name`, `description`, `books_count`, `voters_count`, `tags`, and
`created_by_user`. With `--books` you get one row per book.

## Classify a URL or id

`id` turns a URL or bare id into an (entity, id) pair without fetching anything.
It is pure local work, never blocked, and made for scripts:

```bash
goodread id https://www.goodreads.com/book/show/2767052
```

```
book	2767052
```

It takes several at once:

```bash
goodread id https://www.goodreads.com/author/show/153394 2767052 list/show/1
```

Use it to route URLs to the right command, or to validate input before you spend
a request on it.

## From a search to a record

`search --books` gives you `book_id` on every row, so a search composes straight
into a lookup:

```bash
goodread search dune --books --format jsonl | jq -r .book_id | head -1
# then: goodread book <that id>
```

For pulling a whole genre or list into records in one pass, see
[bulk crawling](/guides/bulk-crawling/).
