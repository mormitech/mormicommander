package main

// The OpenRouter provider from start to end, against a made-up OpenAI-compatible server (httptest) and the fake
// keychain: the key is never written to the user's real keychain, and nothing leaves the computer.

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mormicommander/internal/ai"
	"mormicommander/internal/keychain"
)

// fakeOpenRouter answers chat requests with the next of its answers (the last one again when they run out) and the
// model list with a fixed list; it records the chat requests and counts the model list requests.
type fakeOpenRouter struct {
	mu       sync.Mutex
	srv      *httptest.Server
	answers  []string
	requests []map[string]any
	auth     []string
	modelReq atomic.Int32
	hold     chan struct{} // when set, chat requests wait for it (or for the client to go away)
}

func newFakeOpenRouter(t *testing.T, answers ...string) *fakeOpenRouter {
	f := &fakeOpenRouter{answers: answers}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/models":
			f.modelReq.Add(1)
			io.WriteString(w, `{"data":[{"id":"anthropic/claude-sonnet-4.5","name":"Claude Sonnet 4.5","context_length":200000,"pricing":{"prompt":"0.000003","completion":"0.000015"},"supported_parameters":["tools"]}]}`)
		case "/api/v1/chat/completions":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			f.mu.Lock()
			f.requests = append(f.requests, body)
			f.auth = append(f.auth, r.Header.Get("Authorization"))
			n := min(len(f.requests), len(f.answers)) - 1
			a, hold := f.answers[n], f.hold
			f.mu.Unlock()
			if hold != nil {
				select {
				case <-hold:
				case <-r.Context().Done():
					return
				}
			}
			if code, rest, ok := strings.Cut(a, " "); ok && len(code) == 3 && code[0] >= '1' && code[0] <= '5' {
				w.WriteHeader(int(code[0]-'0')*100 + int(code[1]-'0')*10 + int(code[2]-'0'))
				a = rest
			}
			io.WriteString(w, a)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeOpenRouter) url() string { return f.srv.URL + "/api/v1" }

const orToolCall = `{"choices":[{"message":{"role":"assistant","content":"Searching.","tool_calls":[{"id":"c1","type":"function","function":{"name":"find_files","arguments":"{\"namePattern\":\"*.pdf\"}"}}]},"finish_reason":"tool_calls"}]}`
const orAnswer = `{"choices":[{"message":{"role":"assistant","content":"Found 2 PDFs."},"finish_reason":"stop"}]}`

// orAI is an AI feature set up for OpenRouter at the fake server, with a key in the fake keychain.
func orAI(t *testing.T, f *fakeOpenRouter) (*AI, *aiEvents, *keychain.Fake) {
	t.Helper()
	x, ev := testAI(t, aiOptions{Provider: 1, ORModel: "anthropic/claude-sonnet-4.5", ORURL: f.url()})
	kc := keychain.NewFake()
	x.openKeychain = func() (keychain.Keychain, error) { return kc, nil }
	if err := x.SetAPIKey("openrouter", "  sk-or-test  "); err != nil {
		t.Fatal(err)
	}
	x.SetTools([]AITool{{Name: "find_files", Description: "Finds files.", InputSchema: map[string]any{"type": "object"}}})
	return x, ev, kc
}

func TestOpenRouterRun(t *testing.T) {
	f := newFakeOpenRouter(t, orToolCall, orAnswer)
	x, ev, kc := orAI(t, f)
	if got, _ := kc.Get("ai.openrouter"); got != "sk-or-test" {
		t.Fatalf("keychain holds %q", got)
	}
	ev.reply = func(id int, call aiToolCall) { x.ToolReply(id, `{"text":"a.pdf\nb.pdf","error":""}`) }
	p := x.Providers()
	if p.Selected != "openrouter" || !p.OpenRouter.Available || p.OpenRouter.Model != "anthropic/claude-sonnet-4.5" || p.OpenRouter.URL != f.url() {
		t.Fatalf("providers %+v", p)
	}
	// An empty provider means the one of the options.
	run, err := x.Start(AIRequest{Prompt: "find my PDFs", Context: "The user is in the Find files dialog."})
	if err != nil {
		t.Fatal(err)
	}
	evs := ev.waitFinal(t, run)
	want := []ai.Progress{{Kind: "text", Text: "Searching."}, {Kind: "tool", Tool: "find_files", Args: map[string]any{"namePattern": "*.pdf"}},
		{Kind: "done", Text: "Found 2 PDFs."}}
	if len(evs) != len(want) {
		t.Fatalf("events %+v", evs)
	}
	for i := range want {
		if evs[i].Kind != want[i].Kind || evs[i].Text != want[i].Text || evs[i].Tool != want[i].Tool {
			t.Errorf("event %d: %+v, want %+v", i, evs[i], want[i])
		}
	}
	if len(ev.tools) != 1 || ev.tools[0].Run != run || ev.tools[0].Name != "find_files" || ev.tools[0].Args["namePattern"] != "*.pdf" {
		t.Errorf("ai-tool events %+v", ev.tools)
	}
	// The page's answer went back as a tool message; the system prompt carries the page's context; the key went in
	// the header.
	msgs := f.requests[1]["messages"].([]any)
	if m := msgs[len(msgs)-1].(map[string]any); m["role"] != "tool" || m["content"] != "a.pdf\nb.pdf" || m["tool_call_id"] != "c1" {
		t.Errorf("tool message %v", m)
	}
	if sys := msgs[0].(map[string]any)["content"].(string); !strings.HasPrefix(sys, "The user is in the Find files dialog.") {
		t.Errorf("system prompt %q", sys)
	}
	if f.auth[0] != "Bearer sk-or-test" || f.requests[0]["model"] != "anthropic/claude-sonnet-4.5" {
		t.Errorf("auth %q, model %v", f.auth[0], f.requests[0]["model"])
	}
	x.mu.Lock()
	left := len(x.runs)
	x.mu.Unlock()
	if left != 0 {
		t.Errorf("%d runs left", left)
	}
}

func TestOpenRouterErrors(t *testing.T) {
	f := newFakeOpenRouter(t, `401 {"error":{"code":401,"message":"User not found."}}`)
	x, ev, _ := orAI(t, f)
	run, err := x.Start(AIRequest{Prompt: "hi", Provider: "openrouter"})
	if err != nil {
		t.Fatal(err)
	}
	evs := ev.waitFinal(t, run)
	if len(evs) != 1 || evs[0].Kind != "error" || !strings.Contains(evs[0].Error, "API key was refused") {
		t.Errorf("events %+v", evs)
	}
}

func TestOpenRouterStop(t *testing.T) {
	f := newFakeOpenRouter(t, orAnswer)
	f.hold = make(chan struct{})
	defer close(f.hold)
	x, ev, _ := orAI(t, f)
	run, err := x.Start(AIRequest{Prompt: "hi", Provider: "openrouter"})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	x.Stop(run)
	evs := ev.waitFinal(t, run)
	if len(evs) != 1 || evs[0].Kind != "error" || evs[0].Error != "Stopped." {
		t.Errorf("events %+v", evs)
	}
	// The loop ends too (its request was cancelled), and nothing comes after the last event.
	deadline := time.Now().Add(5 * time.Second)
	for x.hasRun(run) || func() bool { x.mu.Lock(); defer x.mu.Unlock(); return len(x.runs) > 0 }() {
		if time.Now().After(deadline) {
			t.Fatal("the run did not end")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if n := len(ev.of(run)); n != 1 {
		t.Errorf("%d events after Stop", n)
	}
}

func TestOpenRouterRoundCapThroughThePage(t *testing.T) {
	// A model that never stops calling tools: the run ends with an error that says so, after ai.MaxRounds calls.
	f := newFakeOpenRouter(t, orToolCall)
	x, ev, _ := orAI(t, f)
	ev.reply = func(id int, call aiToolCall) { x.ToolReply(id, `{"text":"nothing"}`) }
	run, err := x.Start(AIRequest{Prompt: "hi", Provider: "openrouter"})
	if err != nil {
		t.Fatal(err)
	}
	evs := ev.waitFinal(t, run)
	if last := evs[len(evs)-1]; last.Kind != "error" || !strings.Contains(last.Error, "without giving an answer") {
		t.Errorf("last %+v", last)
	}
	ev.mu.Lock()
	calls := len(ev.tools)
	ev.mu.Unlock()
	if calls != ai.MaxRounds {
		t.Errorf("%d tool calls, want %d", calls, ai.MaxRounds)
	}
}

func TestOpenRouterRefusalsAndProviders(t *testing.T) {
	f := newFakeOpenRouter(t, orAnswer)
	// No key: Start says so, Providers too, and nothing is sent.
	x, _ := testAI(t, aiOptions{Provider: 1, ORModel: "m", ORURL: f.url()})
	if x.HasAPIKey("openrouter") {
		t.Error("a key with an empty keychain")
	}
	if _, err := x.Start(AIRequest{Prompt: "hi"}); err == nil || !strings.Contains(err.Error(), "API key") {
		t.Errorf("no key: %v", err)
	}
	if p := x.Providers().OpenRouter; p.Available || !strings.Contains(p.Error, "API key") {
		t.Errorf("providers %+v", p)
	}
	// No model.
	x.SetAPIKey("openrouter", "k")
	x.options = func() aiOptions { return aiOptions{Provider: 1, ORURL: f.url()} }
	if _, err := x.Start(AIRequest{Prompt: "hi"}); err == nil || !strings.Contains(err.Error(), "model") {
		t.Errorf("no model: %v", err)
	}
	if p := x.Providers().OpenRouter; p.Available || !strings.Contains(p.Error, "model") {
		t.Errorf("providers %+v", p)
	}
	// A plain-http address to another machine is refused before anything is sent.
	x.options = func() aiOptions { return aiOptions{Provider: 1, ORModel: "m", ORURL: "http://example.com/v1"} }
	if _, err := x.Start(AIRequest{Prompt: "hi"}); err == nil || !strings.Contains(err.Error(), "https://") {
		t.Errorf("http: %v", err)
	}
	if len(f.requests) != 0 {
		t.Errorf("%d requests sent", len(f.requests))
	}
	// Removing the key.
	if err := x.SetAPIKey("openrouter", ""); err != nil || x.HasAPIKey("openrouter") {
		t.Errorf("remove: %v", err)
	}
	if err := x.SetAPIKey("openrouter", "two words"); err == nil {
		t.Error("a key with a space was saved")
	}
	if err := x.SetAPIKey("claude", "k"); err == nil {
		t.Error("a key for claude was saved")
	}

	// Claude Code is the chosen provider: the keychain is not asked at all.
	asked := 0
	y, _ := testAI(t, aiOptions{})
	y.openKeychain = func() (keychain.Keychain, error) { asked++; return keychain.NewFake(), nil }
	if p := y.Providers(); p.Selected != "claude" || p.OpenRouter.Available || !strings.Contains(p.OpenRouter.Error, "not the chosen") {
		t.Errorf("providers %+v", p)
	}
	if asked != 0 {
		t.Errorf("the keychain was opened %d times for a Claude Code user", asked)
	}
}

func TestOpenRouterKeychainUnavailable(t *testing.T) {
	x, _ := testAI(t, aiOptions{Provider: 1, ORModel: "m"})
	x.openKeychain = func() (keychain.Keychain, error) {
		return nil, keychain.Unavailable("Install secret-tool (libsecret-tools) to keep secrets in the keyring.")
	}
	err := x.SetAPIKey("openrouter", "sk-x")
	if err == nil || !strings.Contains(err.Error(), "Install secret-tool") {
		t.Errorf("SetAPIKey: %v", err)
	}
	if x.HasAPIKey("openrouter") {
		t.Error("HasAPIKey without a keychain")
	}
	if p := x.Providers().OpenRouter; p.Available || !strings.Contains(p.Error, "Install secret-tool") {
		t.Errorf("providers %+v", p)
	}
	// The keychain refuses to write.
	y, _ := testAI(t, aiOptions{})
	kc := keychain.NewFake()
	kc.Fail = keychain.Unavailable("The keychain is locked.")
	y.openKeychain = func() (keychain.Keychain, error) { return kc, nil }
	if err := y.SetAPIKey("openrouter", "sk-x"); err == nil || !strings.Contains(err.Error(), "locked") {
		t.Errorf("refused: %v", err)
	}
}

func TestOpenRouterModels(t *testing.T) {
	f := newFakeOpenRouter(t, orAnswer)
	x, _, _ := orAI(t, f)
	for range 2 {
		ms, err := x.Models("openrouter")
		if err != nil {
			t.Fatal(err)
		}
		if len(ms) != 1 || ms[0].ID != "anthropic/claude-sonnet-4.5" || ms[0].ContextLength != 200000 || ms[0].Pricing.Prompt != "0.000003" {
			t.Fatalf("models %+v", ms)
		}
	}
	if n := f.modelReq.Load(); n != 1 {
		t.Errorf("%d model list requests, want 1 (the list is kept for the session)", n)
	}
	// Another address typed in the settings, before it is saved.
	if _, err := x.ModelsFrom("openrouter", f.srv.URL+"/api/v1/"); err != nil {
		t.Fatal(err)
	}
	if _, err := x.ModelsFrom("openrouter", "http://example.com/v1"); err == nil {
		t.Error("plain http to another machine")
	}
	if _, err := x.Models("claude"); err == nil {
		t.Error("claude has no model list")
	}
	// A new key forgets the lists.
	x.SetAPIKey("openrouter", "sk-new")
	x.Models("openrouter")
	if n := f.modelReq.Load(); n != 3 {
		t.Errorf("%d model list requests, want 3", n)
	}
}
