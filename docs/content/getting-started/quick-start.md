---
title: "Quick start"
description: "From an empty terminal to a real book, a real work, a real edition list and a local graph you can query, in a handful of commands."
weight: 30
---

This walks the core loop: read a book, read the work behind it, list its editions, build a small local store and query it offline.
Everything here is on a surface `robots.txt` allows, so nothing needs a flag.

## 1. Read a book

```bash
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
url         https://www.goodreads.com/book/show/2767052-the-hunger-games

4.35 average from 10,233,344 ratings, 275,272 text reviews
```

`book` takes a bare id, a full URL, or several at once.
The same record as JSON:

```bash
goodread book 2767052 --json
```

That JSON carries more than the table shows: `surfaces` says which pages it was read from, `via` and `level` say which rung of the extraction ladder answered for each field, and `missed` is a list of plain sentences about what this page did not carry.

## 2. Read the work behind it

The book above is one printing.
The work is the thing Suzanne Collins wrote:

```bash
goodread work 2792775
```

`/work/<id>` is disallowed by `robots.txt`, so `work` reads the editions page and then the first edition's own page, which carries the work with its places, characters and awards.
That costs two requests, and the record says which edition it came through.

## 3. List the editions

```bash
goodread editions 2792775
```

One row per printing, and this is where the ISBNs are.
A work does not have an ISBN and a book has only its own, so if you want them all, this is the command.
Pages are explicit:

```bash
goodread editions 2792775 --page 2 --pages 3
```

## 4. Resolve an identifier

If what you have is an ISBN rather than an id:

```bash
goodread lookup 9780439023481
goodread lookup "the hunger games"
```

`lookup` goes through the open autocomplete route, which needs no key and is allowed.

## 5. Classify without fetching

`id` turns a URL or id into an (entity, id) pair without touching the network, which is handy in scripts:

```bash
goodread id https://www.goodreads.com/book/show/2767052
```

```
book	2767052
```

## 6. Build a small store and query it

Everything read can be folded into a local SQLite graph.
Look before you leap:

```bash
goodread crawl --seed gr:author/153394 --depth 1 --dry-run
```

`--dry-run` prints the plan and reads nothing, which is how you find out a crawl is twelve hours before starting it.
Drop the flag to run it:

```bash
goodread crawl --seed gr:author/153394 --depth 1
```

Then work offline:

```bash
goodread find hunger
goodread graph gr:book/2767052 --depth 2
goodread query "select title, isbn13, num_pages from books order by num_pages desc limit 5"
goodread export --to jsonl > books.jsonl
```

An interrupted crawl continues where it stopped when you run the same command again, because the frontier lives in the store.

## 7. Compose

Output that pipes is the point.
The default is a table on a terminal and JSONL when piped, so the same command reads well by hand and parses cleanly in a pipe:

```bash
goodread editions 2792775 | jq -r .isbn13
goodread book 2767052 --fields title,isbn13,num_pages -f tsv
goodread book 2767052 --template '{{.Book.Title}} isbn {{.Book.ISBN13}}'
```

## Where to next

You have the core loop.
From here:

- [Books, works and editions](/guides/books-and-works/) covers the whole read side and the difference the tool is built on.
- [The store and the graph](/guides/store-and-graph/) covers `find`, `query`, `graph` and `export`.
- [Crawling](/guides/crawling/) covers seeds, the frontier, resuming and the sitemap.
- [robots.txt and what it costs](/guides/robots-and-limits/) covers the disallowed surfaces and `--no-robots`.
- [Output formats](/guides/output-formats/) covers formats, fields, templates and exit codes.
- The [CLI reference](/reference/cli/) lists every command and flag.
