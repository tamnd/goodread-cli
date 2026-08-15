package goodread

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// The MCP server, per 05_commands.md section 7.
//
// Eleven read tools mapping one to one onto commands, and three commands that
// are deliberately not here: search, shelf and reviews.
//
// The reason is not that --no-robots is wrong. It is that the flag's whole
// justification is a person deciding it is their call, and a model calling a
// tool is not that person deciding. An override a model can trigger is not an
// override, it is a default. So the server does not read a disallowed surface
// at all, and MCPServer ignores Config.NoRobots rather than honouring it.
//
// The transport is JSON-RPC 2.0 over stdio, newline delimited, which is what
// the stdio transport in the MCP spec is.

// MCPTool is one tool the server offers.
type MCPTool struct {
	Name        string
	Command     string
	Surface     string
	Description string

	// Args are the tool's arguments, named after the flags of the command it
	// stands for, because somebody reading a transcript should be able to map
	// a tool call onto a command line without a table.
	Args []MCPArg
}

// MCPArg is one argument of one tool.
type MCPArg struct {
	Name     string
	Type     string // "string", "integer" or "boolean"
	Required bool
	Help     string
}

// MCPTools is the whole tool list, and it is the whole of what the server can do.
//
// TestMCPHasNoDisallowedTools walks this against the op registry, so a tool
// added here that reaches a surface marked Disallowed fails the build rather
// than shipping.
var MCPTools = []MCPTool{
	{Name: "book_get", Command: "book", Surface: "s1",
		Description: "Read a book by id, ISBN13 or URL, with its work, series, contributors and stats.",
		Args: []MCPArg{
			{Name: "id", Type: "string", Required: true, Help: "book id, ISBN13, ASIN or a goodreads book URL"},
			{Name: "depth", Type: "string", Help: "meta or full, per the extraction ladder"},
		}},
	{Name: "work_get", Command: "work", Surface: "s6",
		Description: "Read a work through its best edition, since the work page itself is disallowed.",
		Args: []MCPArg{
			{Name: "id", Type: "string", Required: true, Help: "work id"},
		}},
	{Name: "author_get", Command: "author", Surface: "s2",
		Description: "Read an author and the books their page lists.",
		Args: []MCPArg{
			{Name: "id", Type: "string", Required: true, Help: "author id or a goodreads author URL"},
		}},
	{Name: "series_get", Command: "series", Surface: "s3",
		Description: "Read a series and its books in series order.",
		Args: []MCPArg{
			{Name: "id", Type: "string", Required: true, Help: "series id or a goodreads series URL"},
		}},
	{Name: "editions_list", Command: "editions", Surface: "s6",
		Description: "List the editions of a work, which is where the ISBNs live.",
		Args: []MCPArg{
			{Name: "work", Type: "string", Required: true, Help: "work id"},
			{Name: "page", Type: "integer", Help: "which page of editions to read, from 1"},
		}},
	{Name: "quotes_list", Command: "quotes", Surface: "s7",
		Description: "List a work's quotes, or an author's with author set.",
		Args: []MCPArg{
			{Name: "id", Type: "string", Required: true, Help: "work id, or author id with author set"},
			{Name: "author", Type: "boolean", Help: "read the author's quotes page instead of a work's"},
			{Name: "page", Type: "integer", Help: "which page of quotes to read, from 1"},
		}},
	{Name: "genre_get", Command: "genre", Surface: "s5",
		Description: "Read a genre, its featured books and the genres it is related to.",
		Args: []MCPArg{
			{Name: "slug", Type: "string", Required: true, Help: "genre slug, such as fantasy"},
		}},
	{Name: "list_get", Command: "list", Surface: "s4",
		Description: "Read a Listopia list and its ranked books.",
		Args: []MCPArg{
			{Name: "id", Type: "string", Required: true, Help: "list id, such as 1 or 1.Best_Books_Ever"},
			{Name: "page", Type: "integer", Help: "which page of the list to read, from 1"},
		}},
	{Name: "book_lookup", Command: "lookup", Surface: "s14",
		Description: "Resolve an ISBN, ISBN13, ASIN or title to books, through the open autocomplete endpoint.",
		Args: []MCPArg{
			{Name: "query", Type: "string", Required: true, Help: "an identifier or a title"},
			{Name: "limit", Type: "integer", Help: "how many hits to return"},
		}},
	{Name: "store_find", Command: "find",
		Description: "Full text search over the local store. Reads nothing from the network and finds nothing not yet crawled.",
		Args: []MCPArg{
			{Name: "text", Type: "string", Required: true, Help: "what to search for"},
			{Name: "kind", Type: "string", Help: "limit to one record kind: book, author, series, list, genre, user"},
			{Name: "limit", Type: "integer", Help: "how many rows to return"},
		}},
	{Name: "store_query", Command: "query",
		Description: "Run one read only SQL statement over the local store. Offline, and it cannot write.",
		Args: []MCPArg{
			{Name: "sql", Type: "string", Required: true, Help: "a select, with, explain or pragma statement"},
			{Name: "limit", Type: "integer", Help: "row cap"},
		}},
}

// MCPExcluded is what the server does not offer, and why.
//
// Kept as data rather than as a comment, because the startup notice and the
// help text both print it and a list that is printed is a list that stays true.
var MCPExcluded = []string{
	"search, because /search is disallowed. book_lookup is the allowed route and it takes a title.",
	"shelf, because /review/list is disallowed.",
	"reviews, because /book/reviews is disallowed.",
}

// MCPNotice is what the server says on startup and what --help says again.
const MCPNotice = `goodread mcp serves eleven read tools over stdio.

It never reads a surface robots.txt disallows, and --no-robots has no effect on
it. The reason is not that the flag is wrong. It is that the flag's whole
justification is a person deciding it is their call, and a model calling a tool
is not that person deciding. An override a model can trigger is not an
override, it is a default.

Not served:`

// MCPToolNames lists the tool names in the order they are offered.
func MCPToolNames() []string {
	out := make([]string, 0, len(MCPTools))
	for _, t := range MCPTools {
		out = append(out, t.Name)
	}
	return out
}

// InputSchema renders one tool's arguments as JSON Schema.
func (t MCPTool) InputSchema() map[string]any {
	props := map[string]any{}
	var required []string
	for _, a := range t.Args {
		props[a.Name] = map[string]any{"type": a.Type, "description": a.Help}
		if a.Required {
			required = append(required, a.Name)
		}
	}
	schema := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		sort.Strings(required)
		schema["required"] = required
	}
	return schema
}

// MCPServer answers MCP over a pair of streams.
//
// Store may be nil, in which case the two store tools say so rather than
// failing on a nil pointer: a server started before anything was crawled is a
// normal thing to do and the model should be told, not crashed at.
type MCPServer struct {
	Client  *Client
	Store   *Store
	Cache   *Cache
	Config  Config
	Version string

	// Limit caps rows from the store tools, matching the CLI's --limit.
	Limit int
}

// JSON-RPC envelopes.
type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type mcpResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *mcpError       `json:"error,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ServeMCP reads requests until the stream ends.
func (s *MCPServer) ServeMCP(ctx context.Context, in io.Reader, out io.Writer) error {
	dec := json.NewDecoder(bufio.NewReader(in))
	enc := json.NewEncoder(out)
	var mu sync.Mutex
	write := func(resp mcpResponse) {
		mu.Lock()
		defer mu.Unlock()
		resp.JSONRPC = "2.0"
		_ = enc.Encode(resp)
	}

	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		var req mcpRequest
		if err := dec.Decode(&req); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		switch req.Method {
		case "initialize":
			write(mcpResponse{ID: req.ID, Result: s.initResult()})
		case "tools/list":
			tools := make([]map[string]any, 0, len(MCPTools))
			for _, t := range MCPTools {
				tools = append(tools, map[string]any{
					"name":        t.Name,
					"description": t.Description,
					"inputSchema": t.InputSchema(),
				})
			}
			write(mcpResponse{ID: req.ID, Result: map[string]any{"tools": tools}})
		case "tools/call":
			write(s.call(ctx, req))
		case "ping":
			write(mcpResponse{ID: req.ID, Result: map[string]any{}})
		case "notifications/initialized", "notifications/cancelled":
			// Notifications carry no id and want no answer.
		default:
			if len(req.ID) > 0 {
				write(mcpResponse{ID: req.ID, Error: &mcpError{Code: -32601, Message: "no such method: " + req.Method}})
			}
		}
	}
}

func (s *MCPServer) initResult() map[string]any {
	return map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{"tools": map[string]any{}},
		"serverInfo":      map[string]any{"name": "goodread", "version": s.Version},
		"instructions":    MCPNotice + "\n" + strings.Join(MCPExcluded, "\n"),
	}
}

// call runs one tool.
//
// A tool that fails answers with isError set rather than with a JSON-RPC error,
// because a book that does not exist is a result the model should read and act
// on, and a protocol error is a thing the client handles instead of showing.
func (s *MCPServer) call(ctx context.Context, req mcpRequest) mcpResponse {
	var params struct {
		Name string         `json:"name"`
		Args map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return mcpResponse{ID: req.ID, Error: &mcpError{Code: -32602, Message: err.Error()}}
	}
	result, err := s.Invoke(ctx, params.Name, params.Args)
	if err != nil {
		return mcpResponse{ID: req.ID, Result: map[string]any{
			"isError": true,
			"content": []map[string]any{{"type": "text", "text": err.Error()}},
		}}
	}
	body, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return mcpResponse{ID: req.ID, Error: &mcpError{Code: -32603, Message: err.Error()}}
	}
	return mcpResponse{ID: req.ID, Result: map[string]any{
		"content": []map[string]any{{"type": "text", "text": string(body)}},
	}}
}

// Invoke runs one tool by name and returns the record it produced.
//
// Exported so the tests can call every tool without a transport in the way.
func (s *MCPServer) Invoke(ctx context.Context, name string, args map[string]any) (any, error) {
	switch name {
	case "book_get":
		id, err := reqStr(args, "id")
		if err != nil {
			return nil, err
		}
		return s.Client.GetBookRecord(ctx, resolveID(id), Depth(optStr(args, "depth")))
	case "work_get":
		id, err := reqStr(args, "id")
		if err != nil {
			return nil, err
		}
		return s.Client.GetWorkRecord(ctx, resolveID(id))
	case "author_get":
		id, err := reqStr(args, "id")
		if err != nil {
			return nil, err
		}
		return s.Client.GetAuthorRecord(ctx, resolveID(id))
	case "series_get":
		id, err := reqStr(args, "id")
		if err != nil {
			return nil, err
		}
		return s.Client.GetSeriesRecord(ctx, resolveID(id))
	case "editions_list":
		id, err := reqStr(args, "work")
		if err != nil {
			return nil, err
		}
		return s.Client.GetEditionsRecord(ctx, resolveID(id), optPage(args))
	case "quotes_list":
		id, err := reqStr(args, "id")
		if err != nil {
			return nil, err
		}
		return s.Client.GetQuotesRecord(ctx, resolveID(id), optBool(args, "author"), optPage(args))
	case "genre_get":
		slug, err := reqStr(args, "slug")
		if err != nil {
			return nil, err
		}
		return s.Client.GetGenreRecord(ctx, slug)
	case "list_get":
		id, err := reqStr(args, "id")
		if err != nil {
			return nil, err
		}
		return s.Client.GetListRecord(ctx, id, optPage(args))
	case "book_lookup":
		q, err := reqStr(args, "query")
		if err != nil {
			return nil, err
		}
		return s.Client.SearchBooks(ctx, q, s.limit(args))
	case "store_find":
		text, err := reqStr(args, "text")
		if err != nil {
			return nil, err
		}
		if s.Store == nil {
			return nil, errStoreMissing
		}
		return s.Store.FindText(optStr(args, "kind"), text, s.limit(args))
	case "store_query":
		q, err := reqStr(args, "sql")
		if err != nil {
			return nil, err
		}
		if s.Store == nil {
			return nil, errStoreMissing
		}
		return s.Store.Query(ctx, q, s.limit(args))
	}

	// A name that is not here gets the list, since the likeliest reason for
	// asking is a tool the server deliberately does not have.
	return nil, fmt.Errorf("no tool named %q. the tools are: %s", name, strings.Join(MCPToolNames(), ", "))
}

var errStoreMissing = errors.New("there is no local store yet, so this tool has nothing to read. crawl something first")

func (s *MCPServer) limit(args map[string]any) int {
	if n := optInt(args, "limit"); n > 0 {
		return n
	}
	if s.Limit > 0 {
		return s.Limit
	}
	return 20
}

// resolveID accepts what a person would paste, which includes a whole URL.
func resolveID(v string) string {
	v = strings.TrimSpace(v)
	if strings.HasPrefix(v, "http") || strings.HasPrefix(v, "gr:") {
		if _, id := Classify(v); id != "" {
			return id
		}
	}
	return v
}

func reqStr(args map[string]any, key string) (string, error) {
	v := optStr(args, key)
	if v == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return v, nil
}

func optStr(args map[string]any, key string) string {
	s, _ := args[key].(string)
	return strings.TrimSpace(s)
}

func optBool(args map[string]any, key string) bool {
	switch v := args[key].(type) {
	case bool:
		return v
	case string:
		b, _ := strconv.ParseBool(v)
		return b
	}
	return false
}

// optInt takes a number or a numeric string, since a client that sends "2" for
// an integer argument is common enough that refusing it would be pedantry.
func optInt(args map[string]any, key string) int {
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case json.Number:
		n, _ := v.Int64()
		return int(n)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(v))
		return n
	}
	return 0
}

func optPage(args map[string]any) int {
	if n := optInt(args, "page"); n > 0 {
		return n
	}
	return 1
}
