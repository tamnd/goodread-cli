package goodread

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestMCPHasNoDisallowedTools is the test 06_implementation.md section 5 names.
//
// It walks the tool list against the op registry and fails on any tool whose
// surface is one robots.txt disallows. The point is that adding such a tool
// breaks the build rather than shipping, since the whole argument for the
// exclusions is one nobody will remember to re-make in a year.
func TestMCPHasNoDisallowedTools(t *testing.T) {
	disallowed := map[string]bool{}
	for _, op := range Ops {
		if op.Disallowed {
			disallowed[op.Surface] = true
		}
	}
	if len(disallowed) == 0 {
		t.Fatal("no op is marked Disallowed, so this test is not checking anything")
	}
	for _, tool := range MCPTools {
		if tool.Surface == "" {
			continue // a store tool, which reads no surface at all
		}
		if disallowed[tool.Surface] {
			t.Errorf("tool %q reads %s, which robots.txt disallows", tool.Name, tool.Surface)
		}
	}
}

func TestMCPDoesNotServeSearchShelfOrReviews(t *testing.T) {
	// By name as well as by surface, because the surface check would pass for
	// a tool that reached a disallowed page through an op added later without
	// the Disallowed flag on it.
	for _, banned := range []string{"search", "shelf", "reviews", "review", "shelf_rss"} {
		for _, tool := range MCPTools {
			if tool.Command == banned {
				t.Errorf("tool %q maps to the %s command, which the server does not serve", tool.Name, banned)
			}
			if strings.Contains(tool.Name, banned) {
				t.Errorf("tool %q is named after %s", tool.Name, banned)
			}
		}
	}
}

func TestMCPServesTheElevenToolsTheSpecLists(t *testing.T) {
	want := []string{
		"book_get", "work_get", "author_get", "series_get", "editions_list",
		"quotes_list", "genre_get", "list_get", "book_lookup", "store_find", "store_query",
	}
	got := MCPToolNames()
	if len(got) != len(want) {
		t.Fatalf("the server offers %d tools, want the spec's %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("tool %d is %q, want %q", i, got[i], want[i])
		}
	}
}

func TestEveryToolHasASchemaAndARequiredArgument(t *testing.T) {
	for _, tool := range MCPTools {
		schema := tool.InputSchema()
		if schema["type"] != "object" {
			t.Errorf("%s: schema is not an object", tool.Name)
		}
		req, _ := schema["required"].([]string)
		if len(req) == 0 {
			t.Errorf("%s: nothing is required, so an empty call would be valid", tool.Name)
		}
		props, _ := schema["properties"].(map[string]any)
		for _, a := range tool.Args {
			p, ok := props[a.Name].(map[string]any)
			if !ok {
				t.Errorf("%s: argument %s is not in the schema", tool.Name, a.Name)
				continue
			}
			if p["description"] == "" {
				t.Errorf("%s: argument %s has no description", tool.Name, a.Name)
			}
		}
		if tool.Description == "" {
			t.Errorf("%s has no description", tool.Name)
		}
	}
}

// mcpHarness is a server pointed at the golden captures.
func mcpHarness(t *testing.T) *MCPServer {
	t.Helper()
	srv := captureServer(t)
	hits := new(int)

	cfg := DefaultConfig()
	// The override on, so the test can assert the server ignores it.
	cfg.NoRobots = true
	c := NewClient(cfg)
	c.http = &http.Client{Timeout: 30 * time.Second, Transport: rewriteHost{to: srv.URL, next: http.DefaultTransport, hits: hits}}

	b, err := os.ReadFile("testdata/robots.txt")
	if err != nil {
		t.Fatal(err)
	}
	r := ParseRobots(b, DefaultUserAgent())
	r.Source = RobotsTxtURL
	r.FetchedAt = time.Now()
	c.robots.Set(r)

	st, err := OpenStore(filepath.Join(t.TempDir(), "goodread.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	return &MCPServer{Client: c, Store: st, Config: cfg, Version: "test", Limit: 10}
}

func TestBookGetReturnsTheRecord(t *testing.T) {
	s := mcpHarness(t)
	out, err := s.Invoke(context.Background(), "book_get", map[string]any{"id": "2767052"})
	if err != nil {
		t.Fatalf("book_get: %v", err)
	}
	rec, ok := out.(*BookRecord)
	if !ok {
		t.Fatalf("book_get returned %T", out)
	}
	if rec.Book.Title != "The Hunger Games" {
		t.Errorf("title = %q", rec.Book.Title)
	}
}

func TestBookGetTakesAURLToo(t *testing.T) {
	s := mcpHarness(t)
	// Somebody pasting a URL into a tool call is the common case, and refusing
	// it would cost a round trip to explain something the tool can just do.
	out, err := s.Invoke(context.Background(), "book_get",
		map[string]any{"id": "https://www.goodreads.com/book/show/2767052-the-hunger-games"})
	if err != nil {
		t.Fatalf("book_get with a url: %v", err)
	}
	if out.(*BookRecord).Book.LegacyID != 2767052 {
		t.Errorf("resolved to %d", out.(*BookRecord).Book.LegacyID)
	}
}

func TestWorkGetGoesThroughTheBestEdition(t *testing.T) {
	s := mcpHarness(t)
	out, err := s.Invoke(context.Background(), "work_get", map[string]any{"id": "2792775"})
	if err != nil {
		t.Fatalf("work_get: %v", err)
	}
	rec := out.(*WorkRecord)
	if rec.BestBook == nil {
		t.Fatal("the record does not say which edition it was read through")
	}
	if rec.BestBook.LegacyID != 2767052 {
		t.Errorf("read through book %d, want the editions page's first row", rec.BestBook.LegacyID)
	}
	if rec.Work.LegacyID != 2792775 {
		t.Errorf("work legacy id = %d", rec.Work.LegacyID)
	}
	// The work carries what only a work has: places, characters and awards.
	if len(rec.Work.Places) == 0 && len(rec.Work.Characters) == 0 {
		t.Error("the work came back with neither places nor characters, so the best edition read did not happen")
	}
	if rec.EditionCount == nil || *rec.EditionCount == 0 {
		t.Error("the edition count is missing, and it is the one number the editions page is for")
	}
}

func TestAMissingArgumentIsSaidPlainly(t *testing.T) {
	s := mcpHarness(t)
	for _, tool := range MCPTools {
		if _, err := s.Invoke(context.Background(), tool.Name, map[string]any{}); err == nil {
			t.Errorf("%s ran with no arguments", tool.Name)
		} else if !strings.Contains(err.Error(), "required") {
			t.Errorf("%s: %v, which does not say what is missing", tool.Name, err)
		}
	}
}

func TestAToolNobodyHasIsAnsweredWithTheList(t *testing.T) {
	s := mcpHarness(t)
	// The likeliest reason for asking for a name that is not here is one of the
	// three the server deliberately does not have, so the error says what there is.
	_, err := s.Invoke(context.Background(), "shelf_get", map[string]any{"id": "1"})
	if err == nil {
		t.Fatal("shelf_get ran")
	}
	if !strings.Contains(err.Error(), "book_get") {
		t.Errorf("the refusal does not name what is available: %v", err)
	}
}

func TestStoreToolsSayWhenThereIsNoStore(t *testing.T) {
	s := mcpHarness(t)
	s.Store = nil
	for _, name := range []string{"store_find", "store_query"} {
		args := map[string]any{"text": "hunger", "sql": "select 1"}
		_, err := s.Invoke(context.Background(), name, args)
		if err == nil {
			t.Errorf("%s ran with no store", name)
			continue
		}
		if !strings.Contains(err.Error(), "crawl") {
			t.Errorf("%s: %v, which does not say what to do about it", name, err)
		}
	}
}

func TestStoreQueryCannotWrite(t *testing.T) {
	s := mcpHarness(t)
	for _, stmt := range []string{
		"delete from books",
		"update books set title = 'x'",
		"drop table books",
		"insert into books(uri) values('gr:book/1')",
	} {
		if _, err := s.Invoke(context.Background(), "store_query", map[string]any{"sql": stmt}); err == nil {
			t.Errorf("store_query ran %q", stmt)
		}
	}
}

// TestTheServerAnswersTheProtocol drives the real transport end to end.
func TestTheServerAnswersTheProtocol(t *testing.T) {
	s := mcpHarness(t)

	in := strings.NewReader(strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"ping"}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"book_get","arguments":{"id":"1885"}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"nonsense"}`,
	}, "\n"))

	var out strings.Builder
	if err := s.ServeMCP(context.Background(), in, &out); err != nil {
		t.Fatalf("ServeMCP: %v", err)
	}

	dec := json.NewDecoder(strings.NewReader(out.String()))
	var got []map[string]any
	for {
		var m map[string]any
		if err := dec.Decode(&m); err != nil {
			if err == io.EOF {
				break
			}
			t.Fatalf("the server wrote something that is not json: %v\n%s", err, out.String())
		}
		got = append(got, m)
	}

	// Five answers and not six: the notification carries no id and wants none.
	if len(got) != 5 {
		t.Fatalf("got %d responses, want 5: %s", len(got), out.String())
	}
	for i, m := range got {
		if m["jsonrpc"] != "2.0" {
			t.Errorf("response %d is not jsonrpc 2.0", i)
		}
	}

	// initialize names the server and says what it will not do.
	init, _ := got[0]["result"].(map[string]any)
	if init["protocolVersion"] == "" {
		t.Error("initialize returned no protocol version")
	}
	if s, _ := init["instructions"].(string); !strings.Contains(s, "no effect") {
		t.Errorf("the instructions do not mention the override: %q", s)
	}

	// tools/list is the eleven, with schemas.
	list, _ := got[1]["result"].(map[string]any)
	tools, _ := list["tools"].([]any)
	if len(tools) != len(MCPTools) {
		t.Fatalf("tools/list returned %d tools", len(tools))
	}
	first, _ := tools[0].(map[string]any)
	if first["inputSchema"] == nil {
		t.Error("a tool came back with no input schema")
	}

	// tools/call returns the record as text content.
	call, _ := got[3]["result"].(map[string]any)
	content, _ := call["content"].([]any)
	if len(content) == 0 {
		t.Fatal("tools/call returned no content")
	}
	text, _ := content[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, `"title"`) {
		t.Errorf("the content is not the record: %.200s", text)
	}

	// An unknown method is a protocol error, unlike a tool that fails.
	if got[4]["error"] == nil {
		t.Error("an unknown method came back as a result")
	}
}

// TestAFailedToolIsAResultNotAProtocolError holds the distinction.
func TestAFailedToolIsAResultNotAProtocolError(t *testing.T) {
	s := mcpHarness(t)
	in := strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"book_get","arguments":{"id":"999999999"}}}`)
	var out strings.Builder
	if err := s.ServeMCP(context.Background(), in, &out); err != nil {
		t.Fatalf("ServeMCP: %v", err)
	}
	var resp map[string]any
	if err := json.Unmarshal([]byte(out.String()), &resp); err != nil {
		t.Fatalf("not json: %v", err)
	}
	if resp["error"] != nil {
		t.Errorf("a book that does not exist came back as a protocol error: %v", resp["error"])
	}
	result, _ := resp["result"].(map[string]any)
	if result["isError"] != true {
		t.Errorf("a book that does not exist came back as a success: %v", result)
	}
}
