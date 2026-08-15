---
title: "Output formats"
description: "Render records as a table, JSON, CSV, RDF or your own template, and script against the exit codes."
weight: 60
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

RDF is not in that list because it is not a per-record shape.
It comes out of the store rather than off a command: `goodread export --to rdf`.

## What the JSON carries that the table does not

A table shows the fields worth reading by eye.
The JSON also carries the provenance every record has:

- `surfaces` and `sources`: which pages this record was built from.
- `retrieved_at` and `build_id`: when, and against which deployment of the site.
- `via`: which surface each field came from.
- `level`: which rung of the extraction ladder answered for each field, `1` for the Apollo cache, `2` for `ld+json` and `og:`, `3` for a CSS selector.
- `missed`: plain sentences about what this page did not carry, like how many of a work's quotes the book page shows and which command reads the rest.

A field the read did not find is absent, not zero.
So checking for a key is meaningful, and a missing `num_pages` means nobody knows rather than zero pages.

## Narrowing fields

```bash
goodread book 2767052 --fields title,isbn13,num_pages -f tsv
goodread editions 2792775 --fields isbn13,format,publisher -f csv
```

`--fields` names the JSON keys.
`--no-header` drops the header row in table, csv and tsv output, which is handy when a downstream tool expects bare rows.

## Templating records

For full control over each line, apply a Go text/template.
It walks the Go struct, not the JSON, so the field names are the Go names:

```bash
goodread book 2767052 --template '{{.Book.Title}} isbn {{.Book.ISBN13}}'
goodread book 2767052 --template '{{.Book.Title}} ({{.Book.NumPages}}pp)'
```

The two spellings are worth keeping straight: `--fields isbn13` and `--template '{{.Book.ISBN13}}'` reach the same field by different routes.

## Piping

Because the default adapts to the destination, the same command reads well by hand and parses cleanly in a pipe:

```bash
goodread editions 2792775                    # a table, because this is a terminal
goodread editions 2792775 | jq -r .isbn13    # JSONL, because this is a pipe
```

`--limit` (or `-n`) caps the number of rows, and `0` means all.

## Color

`--color` is `auto` by default: color on a terminal and none when piped.
Force it with `--color always` or turn it off with `--color never`.

## Verbosity

`-v` says what is being read and what was not.
`-vv` adds every request and the extraction ladder.
Both go to stderr, so piping the output stays clean, and `-q` silences progress entirely.

## Exit codes for scripting

goodread returns a stable exit code, and the numbering says what went wrong rather than how the run ended, which is what a script wants when it is deciding whether to retry.

| Code | Meaning |
|---|---|
| `0` | success |
| `1` | an error nothing else classified |
| `2` | usage, including a config file that will not load |
| `3` | network, meaning the site never answered |
| `4` | the site answered and the answer was an error or a block |
| `5` | extraction failed, or a record did not reconcile |
| `6` | not found |
| `7` | refused because `robots.txt` disallows the path |
| `8` | `robots.txt` could not be read, so nothing can be checked against it |

These changed in v0.3.0.
v0.2.0 used 3 for no data, 4 for partial and 5 for blocked.

Codes 7 and 8 are separate on purpose.
7 is a decision you can reverse by passing `--no-robots`.
8 is the tool refusing to guess because it could not read the rules at all, and no flag turns that into a proceed.

Code 1 stays a distinct code rather than being folded into the specific ones, because a run that exits 4 is telling a script something true about the site and a run that exits 1 is telling it we do not know.

```bash
goodread book 2767052 --json > book.json
case $? in
  0) echo "got it" ;;
  3|4) echo "the site, not us. retry later" ;;
  7) echo "robots.txt says no" ;;
  *) echo "failed" ;;
esac
```

See [troubleshooting](/reference/troubleshooting/) for what to do about each.
