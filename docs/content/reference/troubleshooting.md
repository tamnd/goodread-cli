---
title: "Troubleshooting"
description: "What each exit code means, what to do about it, and the handful of things that trip people up."
weight: 40
---

Most of these are network reality or a rule rather than a bug.
Goodreads is a public website with a `robots.txt` and a WAF, and goodread is honest about what it can and cannot read.

Start by asking the tool:

```bash
goodread info                                            # resolved config and paths
goodread robots                                          # the rules, and every surface against them
goodread robots check https://www.goodreads.com/search   # why one URL refused
goodread <command> -vv                                   # every request and the extraction ladder
```

## Exit 7: refused because robots.txt disallows it

The path is on a surface Goodreads' `robots.txt` disallows: the search page, shelves, reviews past the embedded thirty, or `/work/<id>`.
`goodread robots check <url>` names the rule that decided.

Two of those have a real replacement on an allowed surface.
Use `goodread lookup` instead of site search and `goodread work` instead of the work page.
Shelves and the full review set have no replacement, and saying so is more useful than a clever path to the same bytes.

If you have decided it is your call, `--no-robots` reads them anyway.
It warns once, the pace floor still applies, and on a crawl it needs `--yes` as well.
See [robots.txt and what it costs](/guides/robots-and-limits/).

## Exit 8: robots.txt could not be read

Nothing was attempted.
There is no fallback copy of the rules and no bundled default, because a stale copy that says yes is worse than no answer, and no flag turns this into a proceed.

In practice this is a network or DNS problem rather than a Goodreads problem, so check that you can reach `https://www.goodreads.com/robots.txt` at all.

## Exit 4: the site answered, and the answer was an error or a block

Either an HTTP error, or the AWS WAF answering with a challenge instead of a page.

goodread reports a challenge and stops.
It does not run a browser, solve it, or carry a session cookie in, because a public page that has been gated is one somebody chose to gate.

What to do, in order:

1. **Wait and retry.**
   A challenge is often transient, and the same page frequently succeeds later.
2. **Slow down.**
   Raise `--delay`.
   The default is two seconds and there is nothing wrong with five.
3. **Take a different surface.**
   The route around a block is usually a different page rather than a harder request.
   `lookup` for search, `editions` for ISBNs, `work` for a work.

A 429 is handled for you: goodread backs off and retries up to `--retries` times.
Seeing it often means you are going too fast, so raise `--delay` and let the cache absorb the repeats.

## Exit 3: the site never answered

A timeout, a DNS failure, a dropped connection.
Nothing came back at all, which is different from exit 4 where something did.
Check the network, then raise `--timeout`.

## Exit 6: not found

goodread reached the page and there was nothing there: a 404, an id that does not exist, a search with no matches.

Check the id first.
`goodread id <url>` shows how goodread classifies it without spending a request, and a work id used where a book id belongs reads a different book entirely.

## Exit 5: extraction failed, or a record did not reconcile

Either the page came back and nothing on it matched, or the numbers did not add up.

`goodread book <id> --check` derives the mean from the five star buckets and compares it against the published average, which is the check that catches a reversed distribution.

If a page you know exists stops parsing, the likeliest cause is a redesign moving a selector.
`goodread extraction` shows which rung answers for each surface and how many selectors are in play, and `goodread verify` checks the extractor against the pinned captures:

```bash
goodread verify
goodread verify --strict
goodread verify --sample 5
```

`--sample` reads that many live book pages instead of the captures.
Please keep the number small.

## Exit 2: usage

A bad flag, a missing argument, or a config file that will not parse.
A config file that is present and broken is an error rather than a shrug, because the alternative is running with settings you think you changed.

`no_robots` in the config file is a deliberate error with a message saying to pass the flag instead.

## Exit 1: something else

Nothing above classified it.
A run that exits 4 is telling you something true about the site and a run that exits 1 is telling you we do not know, which is why it stays a separate code.
Re-run with `-vv` and open an issue with what it printed.

## A crawl reported failures

A run where every page failed exits non-zero.
A run that lost three of four hundred does not, because that is a normal crawl.

The summary prints up to five failures with the reason recorded for each.
The frontier lives in the store, so running the same command again retries what is left and serves what already succeeded out of the cache.

## Nothing in the store

`find`, `query`, `graph` and `export` read the local store and never make a request, so an empty result means nothing has been crawled into it yet, or `--store` is pointing somewhere else.

```bash
goodread info
goodread query --tables
goodread db info
```

## Where state lives

The page cache and the SQLite store both live under the data dir, which is the XDG data directory by default and moves with `--data-dir` or `GOODREAD_DATA_DIR`.
The store file alone moves with `--store`.

```bash
goodread info          # the resolved paths
goodread cache info    # location, file count, size
goodread cache clear   # start fresh
```
