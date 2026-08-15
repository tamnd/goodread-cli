---
title: "goodread"
description: "A command line for public Goodreads data. Read books, works, authors, series, editions, lists, genres and quotes into structured records, store them as a graph, and query them offline."
heroTitle: "Goodreads, from the command line"
heroLead: "goodread is a single pure-Go binary that turns public Goodreads pages into structured records, folds them into a local SQLite graph, and hands them back as a table, JSON, CSV, RDF or your own template. No API key and nothing to sign up for."
heroPrimaryURL: "/getting-started/quick-start/"
heroPrimaryText: "Get started"
---

Goodreads has no public API any more, so getting structured data out of it usually means writing a scraper, guessing at selectors, and doing it again the next time a page changes.
goodread does that part for you.
It reads a public page, folds it into a record with real fields, and records where every number came from.

```bash
goodread book 2767052       # a printing, with its work, series and contributors
goodread work 2792775       # the thing itself, read through its best edition
goodread editions 2792775   # every printing, which is where the ISBNs are
goodread author 153394      # an author and the books their page lists
```

It talks to `www.goodreads.com` over plain HTTPS with no API key.
The binary is pure Go with no runtime dependencies.
Output is a table on a terminal and JSONL when piped, with json, csv, tsv, url, rdf, `--fields` and `--template` when you want something else.

## What you can do with it

- **Read a book, and know it is not a work.**
  A work is what somebody wrote and a book is one printing of it.
  The Hunger Games is one work with 527 editions, each with its own ISBN, page count and rating population.
  `book` gives you a printing and `work` gives you the thing itself.
- **Know where every number came from.**
  Each record carries the surface it was read from and which rung of the extraction ladder answered for each field.
  `goodread extraction` prints the whole ladder.
- **Build a graph, then query it offline.**
  Everything read is folded into SQLite as nodes and edges over a closed set of twelve predicates.
  `graph` walks it, `query` runs SQL over it, `find` does full text search, and `export` writes JSONL or RDF.
- **Crawl politely, and resume.**
  `crawl` walks out from a set of seeds one connection at a time, and there is no flag to make it more.
  The frontier lives in the store, so an interrupted crawl continues where it stopped.
- **Serve it to a model.**
  `goodread mcp` is an MCP server over stdio with eleven read tools, and no search, shelf or reviews.
- **Script it.**
  Stable exit codes that say what went wrong rather than how the run ended, and an `id` command that classifies a URL without fetching.

## What it will not read by default

goodread obeys `robots.txt`.
Four surfaces are disallowed by Goodreads' rules and are not read unless you say so: the search page, shelves, reviews and `/work/<id>`.
There is a `--no-robots` flag for a person who has decided it is their call, and it warns once, and the pace floor still applies.

The MCP server never reads a disallowed surface at all, and `--no-robots` has no effect on it.
The flag's whole justification is a person deciding it is their call, and a model calling a tool is not that person deciding.

## Independent and public-data only

goodread is an independent, open-source tool.
It is not affiliated with, endorsed by, or sponsored by Goodreads or Amazon.
It reads only public pages, at a polite default rate of one request a second at the very fastest and two seconds by default.

## Where to go next

- New here?
  Start with the [introduction](/getting-started/introduction/) for the mental model, then the [quick start](/getting-started/quick-start/).
- Want to install it?
  See [installation](/getting-started/installation/).
- Looking for a specific task?
  The [guides](/guides/) cover books and works, the graph and the store, crawling, the MCP server, robots and output formats.
- Need every flag?
  The [CLI reference](/reference/cli/) is the full surface.
