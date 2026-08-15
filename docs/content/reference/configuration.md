---
title: "Configuration"
description: "The config file, the data directory, the store, politeness, environment, and global flags."
weight: 20
---

goodread needs almost no configuration.
The defaults are chosen so the common case needs none, and everything is a flag.
See everything it resolved with:

```bash
goodread info
```

That prints the configuration, the paths and the affiliation disclaimer.

## Precedence

Flag, then environment, then config file, then default.
The flag always wins, because a run that behaves differently on two machines with the same command line is a run nobody can help you debug.

## The config file

Optional, and there is no default one to write.
It lives at `~/.config/goodread/config.toml`, or `$XDG_CONFIG_HOME/goodread/config.toml`, or wherever `GOODREAD_CONFIG` points.

```toml
# ~/.config/goodread/config.toml
pace       = "3s"
depth      = "full"
cache_ttl  = "72h"
no_cache   = false
store      = "~/books/goodread.db"
data_dir   = "~/books"
user_agent = "yourname/1.0 (+https://example.com)"
format     = "jsonl"
timeout    = "45s"
retries    = 5
```

| Key | Overriding flag |
|---|---|
| `pace` | `--delay` |
| `depth` | `--depth` |
| `cache_ttl` | `--cache-ttl` |
| `no_cache` | `--no-cache` |
| `store` | `--store` |
| `data_dir` | `--data-dir` |
| `user_agent` | `--user-agent` |
| `format` | `--format` |
| `timeout` | `--timeout` |
| `retries` | `--retries` |

Every key has a flag, and a key that cannot be a flag does not belong in the file.
The grammar is `key = value` and `# comment`, which is deliberately less than TOML: a config file the flags cannot express is one nobody can reproduce from a command line.

A missing file is not an error.
A file that is there and does not parse is an error, and exits 2, because the alternative is running with settings you think you changed.

`no_robots` is not a key.
Setting it is an error with a message saying why, rather than a silent ignore, so nobody spends an afternoon wondering what it does.
The override's whole justification is that a person decided, this invocation, that it was their call, and a config key would make it ambient.

## The data directory

goodread keeps its state under one tree: the on-disk page cache and the SQLite store.
It defaults to the XDG data directory, which is `~/.local/share/goodread` on Linux.
Move it with `--data-dir` or `GOODREAD_DATA_DIR`.

## The store

The graph and the crawl frontier live in a SQLite file, by default `<data-dir>/goodread.db`.
Point that single file somewhere else with `--store`, which is how you keep one corpus per project:

```bash
goodread crawl --seed gr:genre/science-fiction --store ~/projects/sf/goodread.db
```

`find`, `query`, `graph`, `export`, `db` and `mcp` all read that file.

## Caching

Every fetch goes through a content-addressed gzip cache on disk, so a repeat run does not re-fetch unchanged pages.
`--cache-ttl` sets how long an entry stays fresh, default 24 hours.
`--no-cache` bypasses it for one run, and `--refresh` forces a re-fetch and rewrites the entry.
Manage it with `cache info`, `cache path <url>` and `cache clear`.

## Politeness

| Flag | Default | Meaning |
|---|---|---|
| `--delay` | `2s` | Minimum spacing between requests, with a hard floor of `1s` |
| `--timeout` | `30s` | Per-request timeout |
| `--retries` | `3` | Retry attempts on 429 and 5xx |
| `--user-agent` | `goodread/<version> (+<repo>)` | The header sent |

A `--delay` below one second is clamped, with a note on stderr saying so.
It is neither honoured nor silently dropped.

There is no `--workers` and no other parallelism flag.
There was one in v0.2.0, clamped to a maximum of one, so it did nothing except suggest the tool could be told to open ten connections.
A flag that does nothing but imply that is worse than no flag.

## --no-robots

```
--no-robots   read paths that robots.txt disallows
```

It has no config key and no environment variable, on purpose.
It has to be typed, every time, by the person running the command.
It warns once on stderr, the pace floor still applies, a crawl with it needs `--yes` as well, and the MCP server ignores it entirely.

See [robots.txt and what it costs](/guides/robots-and-limits/).

## Environment variables

| Variable | Used for |
|---|---|
| `GOODREAD_DATA_DIR` | Root data directory, overriding the XDG default |
| `GOODREAD_CONFIG` | Path to the config file |
| `XDG_DATA_HOME` | Where the data directory defaults under |
| `XDG_CONFIG_HOME` | Where the config file defaults under |

## Global flags

| Flag | Default | Meaning |
|---|---|---|
| `-f, --format` | auto | `table`, `json`, `jsonl`, `csv`, `tsv`, `url`, `raw` |
| `--json` | off | Shorthand for `--format json` |
| `--fields` | all | Comma-separated columns to include |
| `--no-header` | off | Omit the header row in table, csv and tsv |
| `--template` | none | Go text/template applied per record |
| `--color` | auto | `auto`, `always` or `never` |
| `-n, --limit` | `0` | Maximum rows, `0` is all |
| `-q, --quiet` | off | Suppress progress on stderr |
| `-v, --verbose` | off | Explain what is read, `-vv` adds every request and the ladder |
| `--depth` | `meta` | How much to read: `quick`, `meta`, `full`, `deep` |
| `--delay` | `2s` | Minimum spacing between requests |
| `--timeout` | `30s` | Per-request timeout |
| `--retries` | `3` | Retry attempts on 429 and 5xx |
| `--cache-ttl` | `24h` | On-disk cache freshness window |
| `--no-cache` | off | Bypass the on-disk page cache |
| `--refresh` | off | Force a re-fetch and overwrite the cache |
| `--data-dir` | XDG | Root directory for cache and store |
| `--store` | `<data-dir>/goodread.db` | SQLite store path |
| `--user-agent` | `goodread/<version>` | User-Agent header |
| `--no-robots` | off | Read paths `robots.txt` disallows |

## Output auto-detection

The default output format adapts to where it is going: an aligned table when stdout is a terminal, JSONL when it is piped.
That keeps interactive use readable and scripted use parseable without you setting `--format` either time.
See [output formats](/reference/output/).

## Exit codes

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
