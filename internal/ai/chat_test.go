package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeAPI is a made-up OpenAI-compatible server: it records the requests and answers each chat request with the next
// of its answers (the last one again when they run out).
type fakeAPI struct {
	mu       sync.Mutex
	srv      *httptest.Server
	requests []map[string]any
	headers  []http.Header
	answers  []func(w http.ResponseWriter)
	models   string
}

func newFakeAPI(t *testing.T, answers ...func(w http.ResponseWriter)) *fakeAPI {
	f := &fakeAPI{answers: answers}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.headers = append(f.headers, r.Header.Clone())
		f.mu.Unlock()
		switch {
		case r.Method == "GET" && r.URL.Path == "/v1/models":
			io.WriteString(w, f.models)
		case r.Method == "POST" && r.URL.Path == "/v1/chat/completions":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			f.mu.Lock()
			f.requests = append(f.requests, body)
			n := len(f.requests) - 1
			if n >= len(f.answers) {
				n = len(f.answers) - 1
			}
			a := f.answers[n]
			f.mu.Unlock()
			a(w)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeAPI) client(key string) *Client { return &Client{BaseURL: f.srv.URL + "/v1/", Key: key} }

func jsonAnswer(v any) func(w http.ResponseWriter) {
	return func(w http.ResponseWriter) { json.NewEncoder(w).Encode(v) }
}

func statusAnswer(code int, body string) func(w http.ResponseWriter) {
	return func(w http.ResponseWriter) { w.WriteHeader(code); io.WriteString(w, body) }
}

func toolCallAnswer(text string, calls ...ToolCall) func(w http.ResponseWriter) {
	return jsonAnswer(map[string]any{"choices": []any{map[string]any{
		"message": map[string]any{"role": "assistant", "content": text, "tool_calls": calls}, "finish_reason": "tool_calls"}}})
}

func textAnswer(text string) func(w http.ResponseWriter) {
	return jsonAnswer(map[string]any{"choices": []any{map[string]any{
		"message": map[string]any{"role": "assistant", "content": text}, "finish_reason": "stop"}}})
}

func call(id, name, args string) ToolCall {
	return ToolCall{ID: id, Type: "function", Function: FunctionCall{Name: name, Arguments: args}}
}

var testTools = []Tool{
	{Name: "find_files", Description: "Finds files.", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"namePattern": map[string]any{"type": "string"}}}},
	{Name: "get_state", Description: "Says where the user is."},
}

// recorder collects what a loop emits and the tool calls it makes.
type recorder struct {
	mu    sync.Mutex
	steps []Progress
	calls []string
}

func (r *recorder) emit(p Progress) { r.mu.Lock(); r.steps = append(r.steps, p); r.mu.Unlock() }

func TestLoopRoundTrip(t *testing.T) {
	f := newFakeAPI(t,
		toolCallAnswer("Let me look.", call("c1", "find_files", `{"namePattern":"*.pdf"}`), call("c2", "get_state", "")),
		toolCallAnswer("", call("c3", "find_files", `not json`), call("c4", "rm_rf", `{}`)),
		textAnswer("  Found 2 PDFs.  "),
	)
	rec := &recorder{}
	l := Loop{Client: f.client("sk-test"), Model: "vendor/model-1", Tools: testTools, Emit: rec.emit,
		Call: func(_ context.Context, name string, args map[string]any) (string, bool) {
			rec.calls = append(rec.calls, name)
			if name == "get_state" {
				return "No panel.", true
			}
			return "2 files: a.pdf, b.pdf", false
		}}
	last := l.Run(context.Background(), "SYSTEM", "find my pdfs")
	if last.Kind != "done" || last.Text != "Found 2 PDFs." {
		t.Fatalf("last step %+v", last)
	}
	if strings.Join(rec.calls, ",") != "find_files,get_state" {
		t.Errorf("calls %v (bad arguments and unknown tools must not reach the page)", rec.calls)
	}
	kinds := []string{}
	for _, s := range rec.steps {
		kinds = append(kinds, s.Kind+":"+s.Tool+s.Text)
	}
	if got := strings.Join(kinds, "|"); got != "text:Let me look.|tool:find_files|tool:get_state|tool:find_files|tool:rm_rf" {
		t.Errorf("steps %s", got)
	}
	if rec.steps[1].Args["namePattern"] != "*.pdf" || rec.steps[2].Args == nil {
		t.Errorf("args %+v %+v", rec.steps[1].Args, rec.steps[2].Args)
	}
	if len(f.requests) != 3 {
		t.Fatalf("%d requests", len(f.requests))
	}
	// The first request: the model, the system prompt and the prompt, and the tools in the OpenAI function format.
	r0 := f.requests[0]
	if r0["model"] != "vendor/model-1" {
		t.Errorf("model %v", r0["model"])
	}
	msgs := r0["messages"].([]any)
	if m := msgs[0].(map[string]any); m["role"] != "system" || m["content"] != "SYSTEM" {
		t.Errorf("system message %v", m)
	}
	if m := msgs[1].(map[string]any); m["role"] != "user" || m["content"] != "find my pdfs" {
		t.Errorf("user message %v", m)
	}
	tools := r0["tools"].([]any)
	fn := tools[0].(map[string]any)["function"].(map[string]any)
	if tools[0].(map[string]any)["type"] != "function" || fn["name"] != "find_files" || fn["description"] != "Finds files." ||
		fn["parameters"].(map[string]any)["properties"].(map[string]any)["namePattern"] == nil {
		t.Errorf("tool %v", tools[0])
	}
	if p := tools[1].(map[string]any)["function"].(map[string]any)["parameters"].(map[string]any); p["type"] != "object" {
		t.Errorf("a tool without a schema gets an empty object schema: %v", p)
	}
	// The second request carries the assistant's tool calls and one tool message per call, in order.
	msgs = f.requests[1]["messages"].([]any)
	if len(msgs) != 5 {
		t.Fatalf("second request messages %v", msgs)
	}
	if a := msgs[2].(map[string]any); a["role"] != "assistant" || len(a["tool_calls"].([]any)) != 2 {
		t.Errorf("assistant message %v", a)
	}
	if m := msgs[3].(map[string]any); m["role"] != "tool" || m["tool_call_id"] != "c1" || m["content"] != "2 files: a.pdf, b.pdf" {
		t.Errorf("tool message %v", m)
	}
	if m := msgs[4].(map[string]any); m["tool_call_id"] != "c2" || m["content"] != "Error: No panel." {
		t.Errorf("error tool message %v", m)
	}
	msgs = f.requests[2]["messages"].([]any)
	if m := msgs[6].(map[string]any); !strings.Contains(m["content"].(string), "not a JSON object") {
		t.Errorf("bad arguments %v", m)
	}
	if m := msgs[7].(map[string]any); !strings.Contains(m["content"].(string), `no tool named "rm_rf"`) {
		t.Errorf("unknown tool %v", m)
	}
	// The key goes in the Authorization header; a server that is not OpenRouter gets no app headers.
	if h := f.headers[0]; h.Get("Authorization") != "Bearer sk-test" || h.Get("X-Title") != "" || h.Get("HTTP-Referer") != "" {
		t.Errorf("headers %v", h)
	}
}

func TestLoopRoundCap(t *testing.T) {
	f := newFakeAPI(t, toolCallAnswer("", call("c", "get_state", "{}")))
	n := 0
	l := Loop{Client: f.client("k"), Model: "m", Tools: testTools, MaxRounds: 3, Emit: func(Progress) {},
		Call: func(context.Context, string, map[string]any) (string, bool) { n++; return "ok", false }}
	last := l.Run(context.Background(), "s", "p")
	if last.Kind != "error" || !strings.Contains(last.Error, "3 times") {
		t.Fatalf("last %+v", last)
	}
	if n != 3 || len(f.requests) != 4 {
		t.Errorf("%d calls, %d requests", n, len(f.requests))
	}
}

func TestLoopErrors(t *testing.T) {
	cases := []struct {
		answer func(w http.ResponseWriter)
		want   string
	}{
		{statusAnswer(401, `{"error":{"code":401,"message":"No auth credentials found"}}`), "The API key was refused (401)"},
		{statusAnswer(402, `{"error":{"code":402,"message":"Insufficient credits"}}`), "no credits left"},
		{statusAnswer(429, `{"error":{"message":"slow down"}}`), "Rate limit reached (429)"},
		{statusAnswer(400, `{"error":{"code":400,"message":"nope/nope is not a valid model ID"}}`), "model or the API address was not found"},
		{statusAnswer(404, `<html>not here</html>`), "was not found (404). Check both"},
		{statusAnswer(500, `oops`), "The provider had an error (500). Try again in a moment: oops"},
		{statusAnswer(400, `{"error":{"message":"bad tools"}}`), "refused the request (400): bad tools"},
		{jsonAnswer(map[string]any{"error": map[string]any{"code": 502, "message": "upstream down"}}), "had an error (502)"},
		{jsonAnswer(map[string]any{"choices": []any{}}), "without a message"},
		{statusAnswer(200, `garbage`), "could not be read"},
		{jsonAnswer(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": nil}, "finish_reason": "length"}}}), "cut off"},
	}
	for _, c := range cases {
		f := newFakeAPI(t, c.answer)
		l := Loop{Client: f.client("k"), Model: "m", Tools: testTools, Emit: func(Progress) {},
			Call: func(context.Context, string, map[string]any) (string, bool) { return "", false }}
		last := l.Run(context.Background(), "s", "p")
		if last.Kind != "error" || !strings.Contains(last.Error, c.want) {
			t.Errorf("want %q, got %+v", c.want, last)
		}
	}
	// A server that can't be reached says which.
	f := newFakeAPI(t, textAnswer("x"))
	f.srv.Close()
	l := Loop{Client: f.client("k"), Model: "m", Emit: func(Progress) {}}
	if last := l.Run(context.Background(), "s", "p"); last.Kind != "error" || !strings.Contains(last.Error, "could not be reached") {
		t.Errorf("unreachable: %+v", last)
	}
	// A bad address never goes out.
	l = Loop{Client: &Client{BaseURL: "http://example.com/v1", Key: "k"}, Model: "m", Emit: func(Progress) {}}
	if last := l.Run(context.Background(), "s", "p"); last.Kind != "error" || !strings.Contains(last.Error, "https://") {
		t.Errorf("plain http: %+v", last)
	}
}

func TestLoopStop(t *testing.T) {
	// The server holds the request until the client goes away; cancelling the context ends the run with "Stopped.".
	release := make(chan struct{})
	f := newFakeAPI(t, func(w http.ResponseWriter) { <-release })
	defer close(release)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan Progress, 1)
	go func() {
		done <- Loop{Client: f.client("k"), Model: "m", Emit: func(Progress) {}}.Run(ctx, "s", "p")
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case last := <-done:
		if last.Kind != "error" || last.Error != "Stopped." {
			t.Errorf("last %+v", last)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the run did not stop")
	}
	// Stopped while a tool runs: no further request is made.
	f2 := newFakeAPI(t, toolCallAnswer("", call("c", "get_state", "{}")))
	ctx2, cancel2 := context.WithCancel(context.Background())
	l := Loop{Client: f2.client("k"), Model: "m", Tools: testTools, Emit: func(Progress) {},
		Call: func(context.Context, string, map[string]any) (string, bool) {
			cancel2()
			return "The run was stopped.", true
		}}
	if last := l.Run(ctx2, "s", "p"); last.Error != "Stopped." || len(f2.requests) != 1 {
		t.Errorf("stopped during a tool: %+v, %d requests", last, len(f2.requests))
	}
}

func TestModels(t *testing.T) {
	f := newFakeAPI(t)
	f.models = `{"data":[
		{"id":"anthropic/claude-sonnet-4.5","name":"Anthropic: Claude Sonnet 4.5","context_length":1000000,
		 "pricing":{"prompt":"0.000003","completion":"0.000015"},"supported_parameters":["tools","temperature"]},
		{"id":"some/chat-only","name":"","context_length":8192,"pricing":{"prompt":0,"completion":0},"supported_parameters":["temperature"]},
		{"id":"local-model","object":"model"},
		{"id":""}]}`
	ms, err := f.client("").Models(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != 3 {
		t.Fatalf("models %+v", ms)
	}
	if m := ms[0]; m.Name != "Anthropic: Claude Sonnet 4.5" || m.ContextLength != 1000000 || m.Pricing.Prompt != "0.000003" || m.Pricing.Completion != "0.000015" || m.NoTools {
		t.Errorf("model 0 %+v", m)
	}
	if m := ms[1]; m.Name != "some/chat-only" || !m.NoTools || m.Pricing.Prompt != "0" {
		t.Errorf("model 1 %+v %+v", m, m.Pricing)
	}
	if m := ms[2]; m.Name != "local-model" || m.Pricing != nil || m.NoTools {
		t.Errorf("model 2 %+v", m)
	}
	if h := f.headers[0]; h.Get("Authorization") != "" {
		t.Errorf("no key, but an Authorization header: %v", h)
	}
	b, _ := json.Marshal(ms[0])
	if !strings.Contains(string(b), `"contextLength":1000000`) || !strings.Contains(string(b), `"pricing":{"prompt":"0.000003","completion":"0.000015"}`) {
		t.Errorf("JSON %s", b)
	}
}

func TestCheckBaseURL(t *testing.T) {
	for s, ok := range map[string]bool{
		DefaultBaseURL: true, "https://api.example.com/v1": true, "http://localhost:11434/v1": true, "http://127.0.0.1:1234/v1": true,
		"http://[::1]:8080/v1": true, "http://example.com/v1": false, "ftp://x/v1": false, "": false, "openrouter.ai/api/v1": false,
		"https://user:pw@example.com/v1": false,
	} {
		if err := CheckBaseURL(s); (err == nil) != ok {
			t.Errorf("CheckBaseURL(%q) = %v", s, err)
		}
	}
	if !isOpenRouter(DefaultBaseURL) || isOpenRouter("https://api.example.com/v1") || isOpenRouter("https://openrouter.ai.evil.com/v1") {
		t.Error("isOpenRouter")
	}
}

func TestContentText(t *testing.T) {
	for raw, want := range map[string]string{`"hi"`: "hi", `null`: "", ``: "", `[{"type":"text","text":"a"},{"type":"image_url"},{"type":"text","text":"b"}]`: "ab", `42`: ""} {
		if got := contentText(json.RawMessage(raw)); got != want {
			t.Errorf("contentText(%s) = %q", raw, got)
		}
	}
}
