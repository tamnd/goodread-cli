---
title: "Books and authors"
description: "Look up books, authors, and series, find similar books, and read embedded reviews, with the WAF caveat."
weight: 10
---

The core lookups: a book, the author behind it, the series it belongs to, the
books like it, and the reviews on its page. Each takes an id or a full Goodreads
URL.

## A book

```bash
goodread book 2767052
```

`book` takes one id or several at once, and accepts URLs as well as bare ids:

```bash
goodread book 2767052 --format json
goodread book 2767052 5907 --format csv
goodread book https://www.goodreads.com/book/show/2767052
```

A book record carries the fields you would expect from the page: `book_id`,
`title`, `title_without_series`, `description`, `author_id`, `author_name`,
`isbn`, `isbn13`, `asin`, `avg_rating`, `ratings_count`, `reviews_count`,
`published_year`, `publisher`, `language`, `pages`, `format`, `series_id`,
`series_name`, `series_position`, `genres`, `cover_url`, `url`, and
`similar_book_ids`.

Add `--with-reviews` to also pull the reviews embedded on the page:

```bash
goodread book 2767052 --with-reviews
```

## An author

```bash
goodread author 153394
```

Like `book`, it takes several ids and accepts URLs:

```bash
goodread author 153394 1077326 --format csv
```

An author record carries `author_id`, `name`, `bio`, `photo_url`, `website`,
`born_date`, `died_date`, `hometown`, `influences`, `genres`, `avg_rating`,
`ratings_count`, `books_count`, `followers_count`, `notable_book_ids`, and
`url`.

## A series

```bash
goodread series 73758            # the series header
goodread series 73758 --books    # the books in the series
```

The header carries `series_id`, `name`, `description`, `total_books`,
`primary_work_count`, and `book_ids`. With `--books` you get one row per book in
reading order.

## Similar books

```bash
goodread similar 2767052
```

`similar` returns the books Goodreads lists as similar to the one you name.

## Reviews

```bash
goodread reviews 2767052
```

`reviews` returns the reviews embedded on a book's page.

## The WAF caveat

`book`, `similar`, and `reviews` all read the `/book/show/` HTML page, which is
the page the AWS WAF sometimes challenges. When a request is challenged, goodread
exits with code 5 ("blocked") rather than returning partial junk, and the hint
suggests `--cookies`.

The fix is to lend the request a real session. Export a Netscape `cookies.txt`
jar from a signed-in browser and pass it:

```bash
goodread book 2767052 --cookies ~/cookies.txt
```

Slowing down helps too (the default `--delay` is already two seconds). For the
fields that the open endpoints carry, prefer `search --books` and `shelf`, which
are not challenged. See [troubleshooting](/reference/troubleshooting/) for the
cookie file format and more.
