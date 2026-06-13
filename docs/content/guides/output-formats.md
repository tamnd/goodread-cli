---
title: "Output formats"
description: "Render records as a table, JSON, CSV, or your own template, and script against the exit codes."
weight: 50
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

## Narrowing columns

Keep only the fields you want:

```bash
goodread book 2767052 --fields title,avg_rating,ratings_count
goodread shelf 1 --shelf read --fields title,author_name,rating
```

`--no-header` drops the header row in `table` and `csv` output, which is handy
when a downstream tool expects bare rows.

## Templating rows

For full control over each line, apply a Go text/template. The fields are the
record's keys:

```bash
goodread book 2767052 --template '{{.title}} by {{.author_name}} ({{.avg_rating}})'
goodread shelf 1 --shelf read --template '{{.title}}	{{.rating}}'
```

## Piping

Because the default adapts to the destination, the same command reads well by
hand and parses cleanly in a pipe:

```bash
goodread shelf 1 --shelf read                  # a table, because this is a terminal
goodread shelf 1 --shelf read | jq -r .title   # JSONL, because this is a pipe
```

`--limit` (or `-n`) caps the number of rows; `0` means all.

## Exit codes for scripting

goodread returns a stable exit code so a script can branch on the outcome:

| Code | Meaning |
|---|---|
| `0` | OK |
| `1` | Error |
| `2` | Usage error |
| `3` | No data (nothing matched) |
| `4` | Partial (some items failed) |
| `5` | Blocked (a WAF challenge) |

For example, treat a challenge differently from a real failure:

```bash
goodread book 2767052 --format json > book.json
case $? in
  0) echo "got it" ;;
  5) echo "blocked, retry with --cookies" ;;
  *) echo "failed" ;;
esac
```

See [troubleshooting](/reference/troubleshooting/) for what to do about exit 5
and exit 3.
