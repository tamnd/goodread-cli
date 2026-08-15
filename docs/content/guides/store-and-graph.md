---
title: "The store and the graph"
description: "Everything goodread reads is folded into a local SQLite graph you can walk, search, query with SQL and export as JSONL or RDF."
weight: 20
---

Every record goodread reads can be folded into a local SQLite file as nodes and edges.
Once it is there the work is offline: no requests, no rate limit, and SQL over the whole thing.

## Nodes are keyed by URI

A node's primary key is a `gr:` URI built from the legacy id, the lowercase type, no host and no trailing slash.

```
gr:book/2767052
gr:work/2792775
gr:author/153394
```

That matters because `/book/show/2767052` and `/book/show/2767052-the-hunger-games` are the same book, and a store keyed by URL would hold it twice.
It also matters because a bare `153394` is an author to one table and a book to another, and the graph has both in it at once.

There are twelve node kinds, each with its own table: `book`, `work`, `author`, `series`, `list`, `genre`, `quote`, `shelf`, `user`, `place`, `character` and `award`.
Places, characters and awards are nodes rather than strings on a work, because "which books are set in Dublin" should be a query.

## Edges are a closed set

Twelve predicates, and a name outside the set is refused at the write rather than stored and puzzled over later.

| Predicate | From | To | Properties |
|---|---|---|---|
| `edition_of` | book | work | |
| `best_edition` | work | book | |
| `contributed_by` | book | author | `role` |
| `in_series` | book | series | `position`, `number` |
| `shelved_as` | book | genre | |
| `listed_in` | book | list | `position` |
| `set_in` | work | place | |
| `features` | work | character | |
| `won` | work | award | `category`, `year` |
| `quoted_from` | quote | work | |
| `reviewed` | user | book | `rating` |
| `shelved` | user | book | `shelf`, `rating` |

Two of those carry properties for the same reason: the fact lives on the edge rather than on either end.
An illustrator is not a different person from an author, they contributed differently, and a novella at position 2.5 is not book 2.

## Walking the graph

```bash
goodread graph gr:book/2767052
goodread graph gr:book/2767052 --depth 2
goodread graph gr:book/2767052 --edges
goodread graph 2767052
```

`--edges` prints the edges rather than the nodes, which is what you want when the question is how two things are connected rather than what is nearby.

Cycles are normal here rather than an error case.
Book to work to best edition closes in three hops on most popular titles, so every traversal carries a visited set from the first hop.

A walk of more than 500 nodes needs `--yes`.
That is not about politeness, since the store is local, it is about not filling a terminal with fifty thousand rows because a depth was one higher than you meant.

## Full text search

```bash
goodread find hunger
goodread find "suzanne collins" --kind author
goodread find dystopia --kind book -n 20
```

`find` is an FTS5 index over title, description and author name, with porter stemming.
It reads only what is in the store, so it never makes a request and never meets a rate limit or a block.

## SQL

```bash
goodread query --tables
goodread query "select title, isbn13, num_pages from books where num_pages > 500 order by num_pages desc limit 20"
goodread query "select count(*) from books where isbn13 is not null"
goodread query "select src from edges where dst = 'gr:author/153394' and predicate = 'contributed_by'"
goodread query "select b.title, e.props from books b join edges e on e.src = b.uri where e.predicate = 'in_series'"
```

One statement, read only.
`delete`, `update`, `drop` and `insert` are refused, so a query cannot damage a store that took a week to build.

Every node table has `uri`, `id`, `legacy_id`, `title`, `json`, `surfaces` and `retrieved_at`.
The `books` table adds `isbn13`, `asin`, `num_pages`, `publisher`, `published_at` and `work_uri`, with indexes on the first two, because `lookup` is the allowed replacement for site search and a replacement that takes a table scan is not a replacement.
The `json` column holds the whole record, so anything the columns do not cover is still reachable with `json_extract`.

## Export

```bash
goodread export --to jsonl > all.jsonl
goodread export --to jsonl --kind book --kind work > books.jsonl
goodread export --to rdf --kind book > books.ttl
goodread export --edges > edges.jsonl
```

`--kind` is repeatable and limits which node kinds go out.
`--edges` writes the edge table instead of the nodes, and it is JSONL only, because an edge is already a triple and the RDF export carries the edges already.

## The older store commands

`goodread db` still works on the same file: `db info`, `db count`, `db get`, `db export` and `db vacuum`.
It reads the v0.2.0 records table, which lives beside the graph tables rather than being replaced by them.
For anything new, prefer `query`, `find`, `graph` and `export`.

## Where the file lives

By default `<data-dir>/goodread.db`, and `--store` moves it, which is how you keep one corpus per project:

```bash
goodread crawl --seed gr:genre/science-fiction --store ~/projects/sf/goodread.db
goodread query "select count(*) from books" --store ~/projects/sf/goodread.db
```

See [configuration](/reference/configuration/) and [crawling](/guides/crawling/).
