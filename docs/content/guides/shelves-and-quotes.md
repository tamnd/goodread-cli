---
title: "Shelves and quotes"
description: "Read a reader's bookshelf from the public RSS feed, collect quotes, and look up reader profiles."
weight: 30
---

The reader-facing side of Goodreads: a person's shelves, the quotes they and
others have saved, and the public profile behind them.

## Shelves

```bash
goodread shelf 1 --shelf read
```

`shelf` reads a reader's bookshelf. By default it uses the public RSS feed,
which is rich (every book, rating, and review on the shelf) and is not
WAF-challenged, so this is a reliable command. `--shelf` picks which shelf:

```bash
goodread shelf 1 --shelf read
goodread shelf 1 --shelf currently-reading
goodread shelf 1 --shelf to-read
goodread shelf 1 --shelf favorites      # a custom shelf, by its name
```

The default shelf is `read`. Each row carries `shelf_id`, `user_id`, `book_id`,
`title`, `author_name`, `isbn`, `num_pages`, `avg_rating`, `rating`, `shelves`,
`review_id`, `review_text`, `cover_url`, `date_added`, and `date_read`.

The RSS feed returns one page. When you need more than it carries, `--html`
walks the paginated HTML shelf instead, and `--max-pages` caps how far it goes:

```bash
goodread shelf 1 --shelf read --html --max-pages 3
```

The HTML mode reads the shelf web pages, so it can meet a WAF challenge where the
RSS feed does not. Prefer the default feed unless you specifically need more
pages.

## Quotes

```bash
goodread quote 153394
```

`quote` takes a quotes page URL, an author id, or a book id, and returns the
quotes from it:

```bash
goodread quote 153394                                          # an author's quotes
goodread quote https://www.goodreads.com/work/quotes/2792775   # a work's quotes page
```

Each quote carries `quote_id`, `text`, `author_id`, `author_name`, `book_id`,
`book_title`, `likes_count`, and `tags`.

Quote pages hold 30 quotes each. `--max-pages N` walks that many pages (default
1, `0` for every page):

```bash
goodread quote 153394 --max-pages 3
```

## Reader profiles

```bash
goodread user 1
```

`user` returns a public reader profile: `user_id`, `name`, `username`,
`location`, `joined_date`, `friends_count`, `books_read_count`, `ratings_count`,
`reviews_count`, `avg_rating`, `bio`, `website`, `avatar_url`, and
`favorite_book_ids`. From a profile, `favorite_book_ids` feeds `book` and the
same `user_id` feeds `shelf`:

```bash
goodread user 1 --fields user_id,name,books_read_count
goodread shelf 1 --shelf read
```
