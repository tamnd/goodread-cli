---
title: "Troubleshooting"
description: "The handful of things that trip people up, and how to fix each one."
weight: 40
---

Most of these come down to network reality, not a bug. Goodreads is a public
website behind a WAF, and goodread is honest about what it can and cannot read.

## "blocked" and exit code 5

Goodreads sits behind an AWS WAF that intermittently challenges some HTML pages.
When the page goodread asks for comes back as a challenge, it exits with code 5
("blocked") rather than returning the challenge as if it were data. This hits the
commands that read the `/book/show/` page: `book`, `similar`, and `reviews`.

What to do, in order:

1. **Use the open endpoints when you can.** `search` and `search --books` use the
   autocomplete JSON endpoint, and `shelf` uses the public RSS feed. Neither is
   WAF-challenged, so for the fields they carry, prefer them.
2. **Slow down and retry.** The default `--delay` is already two seconds. A
   challenge is often transient; the same page frequently succeeds a moment
   later.
3. **Lend a session with `--cookies`.** Export a Netscape `cookies.txt` jar from
   a signed-in browser and pass it:

   ```bash
   goodread book 2767052 --cookies ~/cookies.txt
   ```

   A real session usually clears the challenge.

## The cookies.txt format

`--cookies` expects a Netscape cookie jar: the plain-text format most browser
extensions export and `curl` reads. Each line is tab-separated:

```
www.goodreads.com	FALSE	/	TRUE	0	session_id	abc123...
```

Lines starting with `#` are comments. Export it from a browser where you are
signed in to Goodreads, save it somewhere private, and pass its path to
`--cookies`. goodread only replays the jar; it never logs in for you and never
stores credentials.

## "no data" and exit code 3

Exit code 3 means goodread reached the page but found nothing to return: a 404,
an empty shelf, a search with no matches. Check the id or URL is right (use
`goodread id <url>` to see how goodread classifies it), try a broader search, or
confirm the shelf actually has books on it.

## Rate limiting (429)

If Goodreads returns 429 (too many requests), goodread backs off and retries up
to `--retries` times. If you see this often, you are going too fast: raise
`--delay`, lower `--workers`, and let the cache absorb repeat fetches. The
defaults (two second delay, two workers) are set to avoid this.

## A crawl reports failures (exit code 4)

`crawl` exits 4 when it processed some URLs but others failed (often a WAF
challenge on a `/book/show/` page in the queue). The records that did parse are
in the store; re-run `crawl` later to retry the queue, or pass `--cookies` for
the challenged ones. Exit 3 from `crawl` means nothing was processed at all (an
empty queue).

## Where state lives

The on-disk cache and the SQLite store both live under the data dir (the XDG
data directory by default, or `GOODREAD_DATA_DIR` / `--data-dir`). The store
file alone can be moved with `--store`. To see the resolved paths:

```bash
goodread info
```

To clear the cache and start fresh:

```bash
goodread cache clear
```
