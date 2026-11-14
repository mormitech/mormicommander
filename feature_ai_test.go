package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"mormicommander/internal/ai"
)

// aiEvents records what the AI feature sends to the page, and answers tool calls with reply (when set).
type aiEvents struct {
	mu       sync.Mutex
	progress map[int][]ai.Progress
	tools    []aiToolCall
	reply    func(id int, call aiToolCall)
}

func (e *aiEvents) emit(event string, data ...any) {
	switch event {
	case "ai-progress":
		e.mu.Lock()
		e.progress[data[0].(int)] = append(e.progress[data[0].(int)], data[1].(ai.Progress))
		e.mu.Unlock()
	case "ai-tool":
		call := data[1].(aiToolCall)
		e.mu.Lock()
		e.tools = append(e.tools, call)
		reply := e.reply
		e.mu.Unlock()
		if reply != nil {
			go reply(data[0].(int), call)
		}
	}
}

func (e *aiEvents) of(run int) []ai.Progress {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]ai.Progress(nil), e.progress[run]...)
}

// waitFinal waits until a run's last event ("done" or "error") has come.
func (e *aiEvents) waitFinal(t *testing.T, run int) []ai.Progress {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		evs := e.of(run)
		if n := len(evs); n > 0 && (evs[n-1].Kind == "done" || evs[n-1].Kind == "error") {
			return evs
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("run %d did not end: %+v", run, e.of(run))
	return nil
}

// testAI is an AI feature with its events recorded and its options given; nothing reaches a window.
func testAI(t *testing.T, opt aiOptions) (*AI, *aiEvents) {
	t.Helper()
	x := newAI(&App{})
	ev := &aiEvents{progress: map[int][]ai.Progress{}}
	x.emit = ev.emit
	x.options = func() aiOptions { return opt }
	home := t.TempDir()
	x.finder = func() ai.Finder {
		return ai.Finder{GOOS: runtime.GOOS, Home: home, LookPath: func(string) (string, error) { return "", errors.New("no") },
			LoginShell: func() string { return "" }, IsFile: func(p string) bool {
				// Only the test's own temporary folders exist, so a claude really installed in /usr/local/bin or
				// /opt/homebrew/bin is never found (or started).
				fi, err := os.Stat(p)
				return strings.HasPrefix(p, filepath.Dir(home)) && err == nil && !fi.IsDir()
			}}
	}
	t.Cleanup(x.shutdown)
	return x, ev
}

func TestAIStartRefusals(t *testing.T) {
	x, _ := testAI(t, aiOptions{})
	if _, err := x.Start(AIRequest{Prompt: "  "}); err == nil {
		t.Error("an empty prompt was started")
	}
	if _, err := x.Start(AIRequest{Prompt: "hi", Provider: "openrouter"}); err == nil || !strings.Contains(err.Error(), "openrouter") {
		t.Errorf("unknown provider: %v", err)
	}
	// Nothing found: the message says how to fix it, and nothing listens yet.
	if _, err := x.Start(AIRequest{Prompt: "hi"}); err == nil || !strings.Contains(err.Error(), "Options > AI") {
		t.Errorf("no claude: %v", err)
	}
	if p := x.Providers().Claude; p.Available || !strings.Contains(p.Error, "Options > AI") {
		t.Errorf("providers %+v", p)
	}
	if x.mcp != nil {
		t.Error("the MCP endpoint was started without a run")
	}
	x2, _ := testAI(t, aiOptions{Path: "/nowhere/claude"})
	if p := x2.Providers().Claude; p.Available || p.Path != "/nowhere/claude" || !strings.Contains(p.Error, "/nowhere/claude") {
		t.Errorf("wrong option: %+v", p)
	}
}

func TestAICallToolAnswers(t *testing.T) {
	x, ev := testAI(t, aiOptions{})
	ev.reply = func(id int, call aiToolCall) {
		switch call.Name {
		case "fails":
			x.ToolReply(id, `{"text":"","error":"No such folder."}`)
		case "plain":
			x.ToolReply(id, `not json`)
		}
	}
	if txt, isErr := x.callTool(context.Background(), 1, "fails", map[string]any{}); txt != "No such folder." || !isErr {
		t.Errorf("error answer: %q %v", txt, isErr)
	}
	if txt, isErr := x.callTool(context.Background(), 1, "plain", map[string]any{}); txt != "not json" || isErr {
		t.Errorf("plain answer: %q %v", txt, isErr)
	}
	// No answer: the call gives up after the timeout, with an error for the model.
	x.replyTimeout = 50 * time.Millisecond
	if txt, isErr := x.callTool(context.Background(), 1, "silent", map[string]any{}); !isErr || !strings.Contains(txt, "did not answer") {
		t.Errorf("timeout: %q %v", txt, isErr)
	}
	// The client went away: the call ends at once.
	x.replyTimeout = time.Minute
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, isErr := x.callTool(ctx, 1, "silent", map[string]any{}); !isErr {
		t.Error("a cancelled call did not fail")
	}
	x.mu.Lock()
	left := len(x.waiting)
	x.mu.Unlock()
	if left != 0 {
		t.Errorf("%d calls left waiting", left)
	}
	if ev.tools[0].Run != 1 || ev.tools[0].Name != "fails" {
		t.Errorf("ai-tool event %+v", ev.tools[0])
	}
}
