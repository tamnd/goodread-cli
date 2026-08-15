---
title: "Introduction"
description: "What goodread reads, how it turns a page into a record, why a book is not a work, and where it stops."
weight: 10
---

[Goodreads](https://www.goodreads.com) is a large public catalog of books, authors, series, editions, lists, genres and quotes.
It used to have an API, and that API is closed now.
The data is still public on the website, but the only way to read it programmatically is to fetch a page and parse it.

goodread does that part.
It is a single binary that fetches a public Goodreads page and turns it into a structured record.
You ask for a book, a work or an author, and it hands you fields, not HTML.

## A book is not a work

This is the distinction the whole tool is built on, and it is the most common error in book data.

A work is what somebody wrote.
A book is one printing of it.
The Hunger Games is one work with 527 editions, and each edition has its own ISBN, page count, publisher, format and rating population, while the work has the original title, the awards, the places and the characters.

So `goodread book` gives you a printing, `goodread work` gives you the thing itself, and `goodread editions` gives you every printing there is.
Reach for `editions` when you want ISBNs, because a work does not have one and a book has only its own.

Every number says which population it came from.
An average rating on a book page is that edition's readers and an average rating on a work is all of them, and a record that does not say which one it holds is not usable for anything quantitative.

## From a page to a record

goodread reads each field off the highest rung of a three rung ladder that answers.

1. **`__NEXT_DATA__`.**
   The book page is a Next.js route and ships an Apollo cache inline, which is the cleanest source there is and where most of a book record comes from.
2. **`application/ld+json` and `og:` tags.**
   Structured data the page ships for search engines.
3. **CSS selectors over the rendered HTML.**
   The last resort, and a promise to break on a redesign.

Author, series, list, genre, editions and quotes are still Rails templates with no Apollo cache and nothing but `og:` tags, so on those pages a selector is not a shortcut, it is the only thing there is.
That is not hidden.
Every selector is registered with the release it was added in, and `goodread extraction` prints the whole ladder with the count per surface:

```bash
goodread extraction
```

Every record carries which surfaces it was read from, which rung answered for each field, and a plain sentence for anything the page did not carry.
A field that is missing says so rather than coming back as a zero.

## robots.txt is the line

goodread obeys `robots.txt`, and it does that structurally rather than by remembering to.
Every readable path is a registered op and nothing builds a URL at the call site, so the tool can list every surface it can reach and say which side of the line each one is on:

```bash
goodread robots
goodread robots check https://www.goodreads.com/search?q=dune
```

Four surfaces are disallowed by Goodreads' rules: the search page, shelves, reviews and `/work/<id>`.
A command that would read one refuses and exits 7.

There is a `--no-robots` flag for a person who has decided it is their call.
It warns once, the pace floor still applies, it is deliberately absent from the config file and from the environment so it cannot be turned on for you, and a crawl with it needs `--yes` as well, because a crawl is the one place where an override multiplies.

If `robots.txt` cannot be read at all, nothing is attempted and the exit code is 8.
There is no fallback copy of the rules, because a stale copy that says yes is worse than no answer.

## Blocks are reported, not solved

Goodreads sits behind an AWS WAF that occasionally answers with a challenge instead of a page.
When that happens goodread says so and exits 4.
It does not run a browser, it does not solve the challenge, and it does not carry a session cookie in, because a public page that has been gated is one somebody chose to gate.

The route around a block is usually a different surface rather than a different request.
`/work/<id>` is disallowed and the editions page is not, so `goodread work` reads a work through its best edition and says which edition it came through.

## Polite by default

goodread waits two seconds between requests, and one second is a hard floor a flag cannot go under.
It makes one request at a time.
There is no `--workers` and no other parallelism flag, on purpose, because a crawler that can be told to open ten connections will be.

## Independent and public-data only

goodread is an independent, open-source tool.
It is not affiliated with, endorsed by, or sponsored by Goodreads or Amazon.
It reads only public pages, at a polite default rate.
It does not log in for you, store your credentials, or touch anything behind an account.

Next: [install it](/getting-started/installation/), then take the [quick start](/getting-started/quick-start/).
