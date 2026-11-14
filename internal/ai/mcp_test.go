package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// testServer is an endpoint with two tools and run 7 going on; calls are answered by the function given.
func testServer(t *testing.T, call func(ctx context.Context, run int, name string, args map[string]any) (string, bool)) *httptest.Server {
	t.Helper()
	s := &Server{Name: "mormicommander", Version: "1.2.3", Token: "secret",
		Tools: func() []Tool {
			return []Tool{
				{Name: "get_state", Description: "Both panels."},
				{Name: "find_files", Description: "Search.", InputSchema: map[string]any{"type": "object",
					"properties": map[string]any{"namePattern": map[string]any{"type": "string"}}}},
			}
		},
		HasRun: func(run int) bool { return run == 7 },
		Call:   call,
	}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts
}

// post sends one JSON-RPC body to run's endpoint and returns the status and the decoded answer.
func post(t *testing.T, ts *httptest.Server, run, body string, edit func(*http.Request)) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest("POST", ts.URL+"/mcp/"+run, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if edit != nil {
		edit(req)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var m map[string]any
	json.Unmarshal(b, &m)
	return resp.StatusCode, m
}

func TestMCPInitialize(t *testing.T) {
	ts := testServer(t, nil)
	code, m := post(t, ts, "7", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"claude-code","version":"2"}}}`, nil)
	if code != 200 {
		t.Fatalf("status %d", code)
	}
	r := m["result"].(map[string]any)
	if r["protocolVersion"] != "2025-03-26" {
		t.Errorf("the client's protocol version is not echoed: %v", r["protocolVersion"])
	}
	if _, ok := r["capabilities"].(map[string]any)["tools"]; !ok {
		t.Errorf("no tools capability: %v", r["capabilities"])
	}
	si := r["serverInfo"].(map[string]any)
	if si["name"] != "mormicommander" || si["version"] != "1.2.3" {
		t.Errorf("serverInfo %v", si)
	}
	if m["id"] != float64(1) || m["jsonrpc"] != "2.0" {
		t.Errorf("envelope %v", m)
	}
	// No version from the client: ours.
	_, m = post(t, ts, "7", `{"jsonrpc":"2.0","id":"a","method":"initialize","params":{}}`, nil)
	if v := m["result"].(map[string]any)["protocolVersion"]; v != ProtocolVersion || m["id"] != "a" {
		t.Errorf("default version %v, id %v", v, m["id"])
	}
}

func TestMCPNotificationAndPing(t *testing.T) {
	ts := testServer(t, nil)
	req, _ := http.NewRequest("POST", ts.URL+"/mcp/7", strings.NewReader(`{"jsonrpc":"2.0","method":"notifications/initialized"}`))
	req.Header.Set("Authorization", "Bearer secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted || len(b) != 0 {
		t.Errorf("notification: %d %q, want 202 and no body", resp.StatusCode, b)
	}
	code, m := post(t, ts, "7", `{"jsonrpc":"2.0","id":2,"method":"ping"}`, nil)
	if code != 200 || m["result"] == nil || m["error"] != nil {
		t.Errorf("ping: %d %v", code, m)
	}
	_, m = post(t, ts, "7", `{"jsonrpc":"2.0","id":3,"method":"resources/list"}`, nil)
	if e, _ := m["error"].(map[string]any); e == nil || e["code"] != float64(codeNoMethod) {
		t.Errorf("unknown method: %v", m)
	}
	_, m = post(t, ts, "7", `{not json`, nil)
	if e, _ := m["error"].(map[string]any); e == nil || e["code"] != float64(codeParse) {
		t.Errorf("bad JSON: %v", m)
	}
}

func TestMCPToolsList(t *testing.T) {
	ts := testServer(t, nil)
	_, m := post(t, ts, "7", `{"jsonrpc":"2.0","id":4,"method":"tools/list"}`, nil)
	tools := m["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 2 {
		t.Fatalf("tools %v", tools)
	}
	first := tools[0].(map[string]any)
	if first["name"] != "get_state" || first["description"] != "Both panels." {
		t.Errorf("first tool %v", first)
	}
	// A tool registered without a schema still gets one, as MCP clients require.
	if s, _ := first["inputSchema"].(map[string]any); s["type"] != "object" {
		t.Errorf("missing schema not filled in: %v", first["inputSchema"])
	}
	second := tools[1].(map[string]any)
	if p := second["inputSchema"].(map[string]any)["properties"].(map[string]any); p["namePattern"] == nil {
		t.Errorf("schema not passed on: %v", second["inputSchema"])
	}
}

func TestMCPToolsCall(t *testing.T) {
	var gotRun int
	var gotName string
	var gotArgs map[string]any
	ts := testServer(t, func(ctx context.Context, run int, name string, args map[string]any) (string, bool) {
		gotRun, gotName, gotArgs = run, name, args
		if name == "broken" {
			return "No such panel.", true
		}
		return `{"count":3}`, false
	})
	_, m := post(t, ts, "7", `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"find_files","arguments":{"namePattern":"*.pdf"}}}`, nil)
	if gotRun != 7 || gotName != "find_files" || gotArgs["namePattern"] != "*.pdf" {
		t.Errorf("call got run %d, %q, %v", gotRun, gotName, gotArgs)
	}
	r := m["result"].(map[string]any)
	c := r["content"].([]any)[0].(map[string]any)
	if c["type"] != "text" || c["text"] != `{"count":3}` || r["isError"] != false {
		t.Errorf("result %v", r)
	}
	_, m = post(t, ts, "7", `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"broken"}}`, nil)
	r = m["result"].(map[string]any)
	if r["isError"] != true || r["content"].([]any)[0].(map[string]any)["text"] != "No such panel." {
		t.Errorf("error result %v", r)
	}
	if gotArgs == nil {
		t.Error("a call without arguments must get an empty map, not nil")
	}
	_, m = post(t, ts, "7", `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{}}`, nil)
	if e, _ := m["error"].(map[string]any); e == nil || e["code"] != float64(codeInvalidParams) {
		t.Errorf("call without a name: %v", m)
	}
}

func TestMCPRefusals(t *testing.T) {
	ts := testServer(t, func(context.Context, int, string, map[string]any) (string, bool) {
		t.Error("a refused request reached the tools")
		return "", true
	})
	call := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_state"}}`
	cases := []struct {
		name string
		run  string
		edit func(*http.Request)
		want int
	}{
		{"no token", "7", func(r *http.Request) { r.Header.Del("Authorization") }, 401},
		{"wrong token", "7", func(r *http.Request) { r.Header.Set("Authorization", "Bearer guess") }, 401},
		{"from a web page", "7", func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") }, 403},
		{"another host name", "7", func(r *http.Request) { r.Host = "evil.example:80" }, 403},
		{"a run that is not going on", "8", nil, 404},
		{"not a run", "x", nil, 404},
	}
	for _, c := range cases {
		if code, _ := post(t, ts, c.run, call, c.edit); code != c.want {
			t.Errorf("%s: status %d, want %d", c.name, code, c.want)
		}
	}
	// GET (a server-sent event stream) is not offered.
	req, _ := http.NewRequest("GET", ts.URL+"/mcp/7", nil)
	req.Header.Set("Authorization", "Bearer secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET: %d, want 405", resp.StatusCode)
	}
	// The guard refuses an empty token too, so a server set up without one is closed rather than open.
	h := Guard("", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("empty token let through") }))
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "http://127.0.0.1/mcp/1", nil)
	r.Header.Set("Authorization", "Bearer ")
	h.ServeHTTP(rec, r)
	if rec.Code != 401 {
		t.Errorf("empty token: %d", rec.Code)
	}
}
