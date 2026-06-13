---
title: "Introduction"
description: "What goodread reads, how it turns a page into a record, and the WAF reality it routes around."
weight: 10
---

[Goodreads](https://www.goodreads.com) is a large public catalog of books,
authors, series, lists, quotes, and reader shelves. It used to have an API; that
API is closed now. The data is still public on the website, but the only way to
read it programmatically is to fetch a page and parse it.

goodread does that part. It is a single binary that fetches a public Goodreads
page and turns it into a structured record. You ask for a book, an author, or a
shelf, and it hands you fields, not HTML.

## From a page to a record

Most Goodreads pages carry a [JSON-LD](https://json-ld.org) block: a chunk of
structured data the page ships for search engines. goodread reads that first,
because it is the cleanest source on the page. When a page does not carry the
field it needs, goodread falls back to reading the HTML with CSS selectors. The
result either way is a record with real fields: a book has a title, an author,
ratings, an ISBN, genres, and a cover URL, and an author has a bio, a hometown,
and a book count.

## The WAF reality, and how goodread works around it

Here is the honest part. Goodreads sits behind an AWS WAF that intermittently
challenges some HTML pages. When that happens, the page comes back as a
challenge instead of the content, and goodread exits with code 5 ("blocked")
rather than pretending it got data.

goodread leans on the open, un-challenged endpoints wherever it can:

- **`search` and `search --books`** use the autocomplete JSON endpoint, which is
  open and not WAF-challenged. This is the reliable way to find books and
  authors.
- **`shelf`** reads the public RSS feed by default. The feed is rich (every book,
  rating, and review on a shelf) and is not challenged.

The commands that read the `/book/show/` HTML page (`book`, `similar`, and
`reviews`) are the ones most likely to hit a challenge. When one does, goodread
exits 5 cleanly and the hint suggests passing `--cookies`: a Netscape
`cookies.txt` jar exported from a signed-in browser session, which lends the
request a real session and usually gets through. This is not common, and it is
not a reason to avoid those commands. It is just the one place where a public
page is sometimes gated.

## Polite by default

goodread waits two seconds between requests and runs two workers by default, so
a busy session stays a good citizen against a public site. You can tune
`--delay` and `--workers`, but the defaults are deliberately gentle.

## Independent and public-data only

goodread is an independent, open-source tool. It is not affiliated with,
endorsed by, or sponsored by Goodreads or Amazon. It reads only public pages, at
a polite default rate. It does not log in for you, store your credentials, or
touch anything behind an account.

Next: [install it](/getting-started/installation/), then take the
[quick start](/getting-started/quick-start/).
