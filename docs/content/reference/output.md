---
title: "Output formats"
description: "Every output format, how to narrow fields, and how to template records."
weight: 30
---

Every command that emits records renders through the same formatter. Pick a
format with `--format` (or `-f`), or let goodread choose: a table when writing to
a terminal, JSONL when piped.

## Formats

```bash
goodread search dune -f table   # aligned columns for reading
goodread search dune -f jsonl   # one JSON object per line, for piping
goodread search dune -f json    # a single JSON array
goodread search dune -f csv     # spreadsheet friendly
goodread search dune -f tsv     # tab-separated
goodread search dune -f url     # just the Goodreads URL of each row
goodread search dune -f raw     # the underlying bytes, unformatted
```

| Format | Best for |
|---|---|
| `table` | Reading on a terminal |
| `jsonl` | Piping into another tool, one object at a time |
| `json` | Loading a whole result as an array |
| `csv` / `tsv` | Spreadsheets and quick column math |
| `url` | Feeding URLs into other commands |
| `raw` | The unformatted bytes |

## Narrowing fields

Keep only the fields you want:

```bash
goodread book 2767052 --fields title,avg_rating,ratings_count
```

`--no-header` drops the header row in `table` and `csv` output, which is handy
when a downstream tool expects bare rows.

## Templating records

For full control over each line, apply a Go text/template. The fields are the
record's keys:

```bash
goodread book 2767052 --template '{{.title}} by {{.author_name}}'
goodread shelf 1 --shelf read --template '{{.title}}	{{.rating}}'
```

## Why auto-detection helps

Because the default adapts to the destination, the same command reads well by
hand and parses cleanly in a pipe:

```bash
goodread shelf 1 --shelf read                  # a table, because this is a terminal
goodread shelf 1 --shelf read | jq -r .title   # JSONL, because this is a pipe
```

You only reach for `--format` when you want something other than that default.

## Color

`--color` is `auto` by default: goodread colors table output on a terminal and
drops color when piped. Force it with `--color always` or turn it off with
`--color never`.
