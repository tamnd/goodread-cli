---
title: "The MCP server"
description: "Serve public Goodreads data to a model over stdio, with eleven read tools and three deliberate omissions."
weight: 50
---

```bash
goodread mcp
```

That is an [MCP](https://modelcontextprotocol.io) server speaking JSON-RPC over stdio.
Point a client at it and a model can read books, works, authors, series, editions, quotes, genres and lists, and search your local store.

## Wiring it up

Most clients take a command and its arguments.
For Claude Desktop or any client with the same shape:

```json
{
  "mcpServers": {
    "goodread": {
      "command": "goodread",
      "args": ["mcp", "--store", "/Users/you/books.db"]
    }
  }
}
```

The store is optional.
A server started before anything has been crawled works fine, and the two store tools say so themselves rather than the command refusing to start.

The startup notice goes to stderr, because stdout is the transport and a notice printed onto the wire would be a protocol error rather than a notice.

## The eleven tools

| Tool | Reads |
|---|---|
| `book_get` | one book, by id, ISBN or URL |
| `work_get` | one work, through its best edition |
| `author_get` | one author |
| `series_get` | one series |
| `editions_list` | the editions of a work |
| `quotes_list` | a work's or an author's quotes |
| `genre_get` | a genre, its featured books and its related genres |
| `list_get` | a Listopia list |
| `book_lookup` | an identifier resolved to a book |
| `store_find` | full text search over the local store |
| `store_query` | one read only SQL statement over the local store |

Every tool has an input schema with at least one required argument, so an empty call is invalid rather than expensive.
Every one takes an id, a bare number or a full Goodreads URL, because somebody pasting a URL into a tool call is the common case and refusing it would cost a round trip to explain something the tool can just do.

`store_query` is read only.
`delete`, `update`, `drop` and `insert` are refused.

## The three it does not have

No `search`, no `shelf`, no `reviews`.

Those are the surfaces `robots.txt` disallows, and the CLI will read them if you pass `--no-robots`.
The server will not, and `--no-robots` has no effect on it even when the process was started with the flag set.

The reason is not that the flag is wrong.
It is that the flag's whole justification is a person deciding it is their call, and a model calling a tool is not that person deciding.
An override a model can trigger is not an override, it is a default.

This is enforced rather than documented.
A test walks the tool list against the op registry and fails the build if any tool reads a surface marked disallowed, and a second test checks the names, because a surface check alone would pass for a tool that reached a disallowed page through an op registered later without the flag on it.

The server also builds its own client from a config with the override taken back out, so even a tool added tomorrow that pointed at a disallowed path would be refused by the client rather than warned about and fetched.

## Errors

A tool that fails answers with a result carrying `isError`, not a JSON-RPC error.
A book that does not exist is a normal answer to a reasonable question, and a model that gets a protocol error for it learns the wrong lesson.

A JSON-RPC error is reserved for the protocol going wrong, like a method the server does not implement.

Asking for a tool that does not exist gets a message naming the tools that do, since the likeliest reason for asking is one of the three deliberate omissions.

## Pace

The server is the same client as everything else.
One request at a time, two seconds apart by default, one second floor, and the same on-disk cache, so a model asking for the same book twice costs one request.
