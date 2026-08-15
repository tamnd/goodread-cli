---
title: "robots.txt and what it costs"
description: "Which surfaces goodread will not read, what you lose, the routes around, and what --no-robots actually means."
weight: 40
---

goodread obeys `robots.txt`, and it does that structurally rather than by remembering to.
Every readable path is a registered op, and nothing builds a URL at the call site, so the tool can print every surface it can reach with the rule that decides each one:

```bash
goodread robots
goodread robots check https://www.goodreads.com/search?q=dune
goodread robots check /work/editions/2792775
```

`robots check` says whether one URL may be fetched and which rule decides it, which is the answer to "why did that refuse" without reading the source.

## What is allowed

The book page, the author page, the series page, the list page, the genre page, the editions page, the quotes page, the sitemaps and the autocomplete endpoint.

Two of those are allowed by an explicit `Allow` sitting over a broader `Disallow`.
`Disallow: /work` would cover the editions and quotes pages, and `Allow: /work/editions` and `Allow: /work/quotes` put them back.
That is Goodreads' own rule and not an interpretation, which is why `goodread robots` prints the deciding rule next to each surface.

## What is not

| Surface | Rule | What you lose |
|---|---|---|
| `/search` | `Disallow: /search` | The full search page, with its pagination and filters |
| `/review/list` and `/review/list_rss` | `Disallow: /review/list` | A reader's shelves, by either route |
| `/book/reviews/` and `/review/show` | `Disallow: /book/reviews/` | Reviews past the thirty the book page embeds |
| `/work/<id>` | `Disallow: /work` | The work page itself |

A command that would read one of these refuses and exits 7.

## The routes around

Two of the four have a real replacement on an allowed surface, and those are what you should reach for first.

**Instead of site search, use `lookup`.**
`/book/auto_complete` is allowed, needs no key, and carries enough to resolve an ISBN, an ASIN or a title to a book.

```bash
goodread lookup 9780439023481
goodread lookup "the hunger games"
```

The `books` table indexes `isbn13` and `asin` for the same reason, since a replacement for search that takes a table scan is not a replacement.

**Instead of the work page, use `work`.**
The editions page is allowed, and the first edition's own book page carries the work with its places, characters, awards and work-level stats.

```bash
goodread work 2792775
```

That is two requests instead of one, and the record says which edition it came through.

The other two, shelves and the full review set, have no replacement.
There is no allowed surface that publishes them, and the honest thing is to say so rather than to find a clever path to the same bytes.

## --no-robots

There is a flag for a person who has decided it is their call.

```bash
goodread search dune --deep --no-robots
goodread shelf 1 --shelf read --no-robots
goodread reviews 2767052 --all --no-robots --yes
```

Four things about it are deliberate.

It warns once, on stderr, so a script that uses it is not silently using it.

The pace floor still applies.
The override is about which paths, not about how fast, and one second between requests remains a hard floor.

It is absent from the config file and from the environment.
It has to be typed, every time, by the person running the command, so it cannot be turned on for you by a file you did not write.

A crawl with it needs `--yes` as well.
On one page the flag is one decision about one request.
On a crawl it applies to every request the crawl makes, which is a different decision wearing the same name, so the crawl asks for the second confirmation.

## The MCP server does not have it

`goodread mcp` serves no disallowed surface, and `--no-robots` has no effect on the server even when the process was started with it.

The reason is not that the flag is wrong.
It is that the flag's whole justification is a person deciding it is their call, and a model calling a tool is not that person deciding.
An override a model can trigger is not an override, it is a default.

See [the MCP server](/guides/mcp/).

## When robots.txt cannot be read

If `robots.txt` itself cannot be fetched, nothing is attempted and the exit code is 8.

There is no fallback copy and no bundled default, because a stale copy that says yes is worse than no answer at all.
No flag turns that into a proceed either, which is the difference between exit 7 and exit 8: 7 is a decision you can reverse, 8 is the tool refusing to guess.

## Blocks are a different thing

Goodreads also sits behind an AWS WAF that occasionally answers with a challenge instead of a page.
That is not `robots.txt` and it exits 4, not 7.

goodread reports a challenge and stops.
It does not run a browser, solve it, or carry a session cookie in.
Slowing down and trying later is usually enough, and if it is not, the answer is a different surface rather than a harder request.
