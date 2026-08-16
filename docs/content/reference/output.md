---
title: "Output formats"
description: "Every output format, what the JSON carries, how to narrow fields, and how to template records."
weight: 30
---

Every command that emits records renders through the same formatter.
Pick a format with `--format` (or `-f`), or let goodread choose: a table when writing to a terminal, JSONL when piped.

## Formats

```bash
goodread editions 2792775 -f table   # aligned columns for reading
goodread editions 2792775 -f jsonl   # one JSON object per line, for piping
goodread editions 2792775 -f json    # a single JSON array
goodread editions 2792775 -f csv     # spreadsheet friendly
goodread editions 2792775 -f tsv     # tab-separated
goodread editions 2792775 -f url     # just the Goodreads URL of each row
goodread editions 2792775 -f raw     # the underlying bytes, unformatted
```

| Format | Best for |
|---|---|
| `table` | Reading on a terminal |
| `jsonl` | Piping into another tool, one object at a time |
| `json` | Loading a whole result as an array |
| `csv` / `tsv` | Spreadsheets and quick column math |
| `url` | Feeding URLs into other commands |
| `raw` | The unformatted bytes |

`--json` is shorthand for `-f json`.

RDF is not here because it is not a per-record shape.
It comes out of the store: `goodread export --to rdf`.

## The envelope

Every record carries provenance, and the JSON formats show all of it.

| Field | What it says |
|---|---|
| `kind` | Which node kind this is |
| `surfaces` | Which pages the record was built from, as `s1`, `s6` and so on |
| `sources` | What those pages carried, like "Next.js, Apollo cache inline" |
| `retrieved_at` | When it was read |
| `build_id` | Which deployment of the site answered |
| `via` | Which surface each field came from |
| `level` | Which rung of the ladder answered for each field |
| `missed` | Plain sentences about what the page did not carry |
| `robots` | Present only when the page was one robots.txt asked us not to read |

A record read under `--no-robots` carries a `robots` block with `allowed: false`, the path, the rule from the file that matched it, and which robots.txt said so.
A record with no `robots` block came off an allowed surface, so a consumer can tell the two apart without knowing which flags the run used.
Reviews carry the block on the row rather than on an envelope, because `reviews --all` returns the book page's sample and the walked pages in one list and the note is what keeps them distinguishable.

The levels are `1` for the `__NEXT_DATA__` Apollo cache, `2` for `application/ld+json` and `og:` tags, and `3` for a CSS selector over the rendered HTML.

A field the read did not find is absent, not zero.
A missing `num_pages` means nobody knows, rather than zero pages.

A search record is an envelope around a list of hits with the page's own account of itself: the total the site published and whether the site hedged it, the range it was showing, the qid that ties the ranks to one run, the tabs and which of them answer, the genre it mapped the query to and the related shelves it offered.
Those numbers stay as the site stated them even when `--limit` trims the rows, because ten rows out of eight hundred and sixteen is still eight hundred and sixteen results.

Every `stats` block also carries its own `via`, which is never omitted, because a book page publishes one edition's readers and a work read publishes all of them and the field names are the same either way.

## Narrowing fields

```bash
goodread book 2767052 --fields title,isbn13,num_pages -f tsv
goodread editions 2792775 --fields isbn13,format,publisher -f csv
```

`--fields` names the JSON keys.
`--no-header` drops the header row in table, csv and tsv output.

## Templating records

A Go text/template applied per record.
It walks the Go struct rather than the JSON, so the field names are the Go names:

```bash
goodread book 2767052 --template '{{.Book.Title}} isbn {{.Book.ISBN13}}'
```

So `--fields isbn13` and `--template '{{.Book.ISBN13}}'` reach the same field by different routes.

## Why auto-detection helps

Because the default adapts to the destination, the same command reads well by hand and parses cleanly in a pipe:

```bash
goodread editions 2792775                    # a table, because this is a terminal
goodread editions 2792775 | jq -r .isbn13    # JSONL, because this is a pipe
```

You only reach for `--format` when you want something other than that default.

## Color

`--color` is `auto` by default: goodread colors table output on a terminal and drops color when piped.
Force it with `--color always` or turn it off with `--color never`.

## Verbosity

`-v` says what is being read and what was not.
`-vv` adds every request and the extraction ladder.
Both go to stderr, so piping the output stays clean.
`-q` suppresses progress entirely.
