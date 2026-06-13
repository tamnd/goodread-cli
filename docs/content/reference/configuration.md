---
title: "Configuration"
description: "The data directory, the store, cookies, politeness knobs, environment, and exit codes."
weight: 20
---

goodread needs almost no configuration. There is no config file; every option is
a flag or an environment variable, and the defaults are chosen so the common
case needs neither. See everything goodread resolved with:

```bash
goodread info
```

It prints the configuration, the paths, and the affiliation disclaimer.

## The data directory

goodread keeps its state under one tree: the on-disk page cache and the SQLite
store. It defaults to the XDG data directory (for example
`~/.local/share/goodread` on Linux). Point it elsewhere with `--data-dir` or the
`GOODREAD_DATA_DIR` environment variable.

## The store

The crawl pipeline writes records and the queue into a SQLite file, by default
`<data-dir>/goodread.db`. Point that single file somewhere else with `--store`,
which is handy when you want one corpus per project:

```bash
goodread crawl --parse --store ~/projects/sf/goodread.db
```

`db info`, `db count`, `db get`, `db export`, and `db vacuum` all read this file.

## Cookies

The `--cookies` flag takes a Netscape `cookies.txt` jar exported from a
signed-in browser session. goodread sends those cookies with each request, which
lends it a real session and usually gets past a WAF challenge on the
`/book/show/` pages. goodread never logs in for you and never stores
credentials; it only replays the jar you hand it.

```bash
goodread book 2767052 --cookies ~/cookies.txt
```

See [troubleshooting](/reference/troubleshooting/) for the cookie file format.

## Caching

Every fetch goes through a content-addressed gzip cache on disk so a repeat run
does not re-fetch unchanged pages. `--cache-ttl` sets how long an entry stays
fresh (default 24h). `--no-cache` bypasses it for one run, and `--refresh`
forces a re-fetch and rewrites the entry. Manage the cache with `cache info`,
`cache path <url>`, and `cache clear`.

## Politeness

goodread is gentle by default so a busy session stays a good citizen against a
public site:

| Flag | Default | Meaning |
|---|---|---|
| `--workers` | `2` | Concurrent requests |
| `--delay` | `2s` | Minimum gap between requests |
| `--timeout` | `30s` | Per-request timeout |
| `--retries` | `3` | Retry attempts on transient failures |

Raise `--workers` and lower `--delay` only when you have a reason to, and keep
them modest.

## Environment variables

| Variable | Used for |
|---|---|
| `GOODREAD_DATA_DIR` | Root data directory (overrides the XDG default) |

## Global flags

| Flag | Default | Meaning |
|---|---|---|
| `-f, --format` | auto | `table`, `json`, `jsonl`, `csv`, `tsv`, `url`, `raw` |
| `--fields` | all | Comma-separated fields to show |
| `--no-header` | off | Omit the header row in table/csv output |
| `--template` | none | Go text/template applied per record |
| `--color` | auto | `auto`, `always`, or `never` |
| `-n, --limit` | `0` | Maximum rows; `0` is all |
| `-q, --quiet` | off | Suppress progress output |
| `--workers` | `2` | Concurrent requests |
| `--delay` | `2s` | Minimum delay between requests |
| `--timeout` | `30s` | Per-request timeout |
| `--retries` | `3` | Retry attempts |
| `--cache-ttl` | `24h` | Cache lifetime |
| `--no-cache` | off | Bypass the on-disk cache |
| `--refresh` | off | Force a re-fetch, ignoring the cache |
| `--data-dir` | XDG | Root data directory (env `GOODREAD_DATA_DIR`) |
| `--store` | `<data-dir>/goodread.db` | SQLite store path |
| `--cookies` | none | Netscape cookie jar |

## Output auto-detection

The default output format adapts to where it is going: an aligned table when the
output is a terminal, JSONL when it is piped. That keeps interactive use readable
and scripted use parseable without you setting `--format` either time. See
[output formats](/reference/output/) for the full set.

## Exit codes

goodread returns a stable exit code so scripts can branch on the outcome:

| Code | Meaning |
|---|---|
| `0` | OK |
| `1` | Error |
| `2` | Usage error |
| `3` | No data (nothing matched) |
| `4` | Partial (some items failed) |
| `5` | Blocked (a WAF challenge) |
