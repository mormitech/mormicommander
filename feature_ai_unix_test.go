//go:build unix

package main

// The AI runs from start to end, with a shell script standing in for the claude CLI (the real one is never started by
// tests). Unix only: the script is a shell script, and ending the CLI's child is checked with a signal.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"mormicommander/internal/ai"
)

// fakeClaude writes a shell script that stands in for the claude CLI (the real one is never started by tests). It
// answers --version, notes its arguments, its folder, its prompt and its MCP config in files under out, and then does
// what body says.
func fakeClaude(t *testing.T, body string) (script, out string) {
	t.Helper()
	dir := t.TempDir()
	out = filepath.Join(dir, "out")
	os.Mkdir(out, 0o755)
	script = filepath.Join(dir, "claude")
	src := `#!/bin/sh
if [ "$1" = "--version" ]; then echo "9.9.9 (Claude Code)"; exit 0; fi
OUT='` + out + `'
for a in "$@"; do printf '%s\n' "$a"; done > "$OUT/args"
pwd > "$OUT/pwd"
cat > "$OUT/prompt"
cfg=""; prev=""
for a in "$@"; do [ "$prev" = "--mcp-config" ] && cfg="$a"; prev="$a"; done
echo "$cfg" > "$OUT/cfgpath"
cp "$cfg" "$OUT/cfg"
` + body
	if err := os.WriteFile(script, []byte(src), 0o755); err != nil {
		t.Fatal(err)
	}
	return script, out
}

func readOut(t *testing.T, out, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(out, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestAIRunToTheEnd(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDECODE", "1")
	script, out := fakeClaude(t, `env > "$OUT/env"
echo '{"type":"system","subtype":"init","tools":[]}'
echo 'not json'
echo '{"type":"assistant","message":{"content":[{"type":"text","text":"Searching."},{"type":"tool_use","name":"mcp__mormi__find_files","input":{"namePattern":"*.pdf"}}]}}'
echo '{"type":"result","subtype":"success","is_error":false,"result":"Found 2 PDFs."}'
`)
	x, ev := testAI(t, aiOptions{Path: script, Model: "opus"})
	if p := x.Providers().Claude; !p.Available || p.Path != script || p.Version != "9.9.9" {
		t.Fatalf("providers: %+v", p)
	}
	run, err := x.Start(AIRequest{Prompt: "find my PDFs", Context: "The user is in the Find files dialog.", Provider: "claude"})
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

	args := strings.Split(strings.TrimSuffix(readOut(t, out, "args"), "\n"), "\n")
	cfgPath := strings.TrimSpace(readOut(t, out, "cfgpath"))
	wantArgs := ai.ClaudeArgs(cfgPath, "opus", ai.SystemPrompt("The user is in the Find files dialog."))
	if strings.Join(args, "\x00") != strings.Join(strings.Split(strings.Join(wantArgs, "\n"), "\n"), "\x00") {
		t.Errorf("arguments:\n got %q\nwant %q", args, wantArgs)
	}
	if got := readOut(t, out, "prompt"); got != "find my PDFs" {
		t.Errorf("prompt on stdin: %q", got)
	}
	if got, _ := filepath.EvalSymlinks(strings.TrimSpace(readOut(t, out, "pwd"))); got != mustEval(t, home) {
		t.Errorf("folder %q, want the home folder %q", got, home)
	}
	env := readOut(t, out, "env")
	if strings.Contains(env, "CLAUDECODE=") {
		t.Error("CLAUDECODE was passed on to the CLI")
	}
	if !strings.Contains(env, "PATH="+filepath.Dir(script)+string(os.PathListSeparator)) {
		t.Error("the program's folder is not first in PATH")
	}
	var cfg struct {
		McpServers map[string]struct {
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(readOut(t, out, "cfg")), &cfg); err != nil {
		t.Fatal(err)
	}
	if u := cfg.McpServers["mormi"].URL; !strings.HasPrefix(u, "http://127.0.0.1:") || !strings.HasSuffix(u, "/mcp/1") {
		t.Errorf("MCP url %q", u)
	}
	// The MCP config, which holds the token, is removed once the CLI has exited.
	aiWaitFor(t, "the MCP config to be removed", func() bool { _, err := os.Stat(cfgPath); return os.IsNotExist(err) })
	aiWaitFor(t, "the run to be forgotten", func() bool { x.mu.Lock(); defer x.mu.Unlock(); return len(x.runs) == 0 })
}

func mustEval(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func aiWaitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("waited in vain for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// A run that is stopped while the CLI waits: a tool call goes through the real endpoint to the page and back, then
// Stop ends the CLI with what it started, and the run's only last event is "Stopped.".
func TestAIToolCallAndStop(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	script, out := fakeClaude(t, `echo '{"type":"assistant","message":{"content":[{"type":"text","text":"Working."}]}}'
sleep 30 &
echo $! > "$OUT/childpid"
wait
echo '{"type":"result","subtype":"success","is_error":false,"result":"too late"}'
`)
	x, ev := testAI(t, aiOptions{Path: script})
	ev.reply = func(id int, call aiToolCall) {
		x.ToolReply(id, fmt.Sprintf(`{"text":"%s ran in run %d","error":""}`, call.Name, call.Run))
	}
	x.SetTools([]AITool{{Name: "get_state", Description: "Both panels."}})
	run, err := x.Start(AIRequest{Prompt: "where am I?"})
	if err != nil {
		t.Fatal(err)
	}
	aiWaitFor(t, "the fake CLI to start its child", func() bool { _, err := os.Stat(filepath.Join(out, "childpid")); return err == nil })
	var cfg struct {
		McpServers map[string]struct {
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers"`
		} `json:"mcpServers"`
	}
	json.Unmarshal([]byte(readOut(t, out, "cfg")), &cfg)
	m := cfg.McpServers["mormi"]
	rpc := func(body string) map[string]any {
		req, _ := http.NewRequest("POST", m.URL, strings.NewReader(body))
		for k, v := range m.Headers {
			req.Header.Set(k, v)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		var r map[string]any
		if err := json.Unmarshal(b, &r); err != nil {
			t.Fatalf("status %d, %q", resp.StatusCode, b)
		}
		return r
	}
	list := rpc(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if tools := list["result"].(map[string]any)["tools"].([]any); len(tools) != 1 || tools[0].(map[string]any)["name"] != "get_state" {
		t.Errorf("tools/list %v", list)
	}
	res := rpc(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"get_state","arguments":{}}}`)["result"].(map[string]any)
	if txt := res["content"].([]any)[0].(map[string]any)["text"]; txt != "get_state ran in run 1" || res["isError"] != false {
		t.Errorf("tools/call %v", res)
	}

	child := strings.TrimSpace(readOut(t, out, "childpid"))
	x.Stop(run)
	evs := ev.waitFinal(t, run)
	if last := evs[len(evs)-1]; last.Kind != "error" || last.Error != "Stopped." {
		t.Errorf("last event %+v", last)
	}
	aiWaitFor(t, "the run to be forgotten", func() bool { x.mu.Lock(); defer x.mu.Unlock(); return len(x.runs) == 0 })
	finals := 0
	for _, e := range ev.of(run) {
		if e.Kind == "done" || e.Kind == "error" {
			finals++
		}
	}
	if finals != 1 {
		t.Errorf("%d last events, want exactly one: %+v", finals, ev.of(run))
	}
	// The CLI's own child was ended with it.
	var pid int
	for _, c := range child {
		pid = pid*10 + int(c-'0')
	}
	aiWaitFor(t, "the CLI's child to end", func() bool { return syscall.Kill(pid, 0) != nil })
	// The ended run's endpoint refuses further calls.
	req, _ := http.NewRequest("POST", m.URL, strings.NewReader(`{"jsonrpc":"2.0","id":3,"method":"ping"}`))
	for k, v := range m.Headers {
		req.Header.Set(k, v)
	}
	if resp, err := http.DefaultClient.Do(req); err == nil {
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("ended run: status %d, want 404", resp.StatusCode)
		}
	}
}

func TestAIRunWithoutResult(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	script, _ := fakeClaude(t, `echo "Error: not logged in" >&2
exit 1
`)
	x, ev := testAI(t, aiOptions{Path: script})
	run, err := x.Start(AIRequest{Prompt: "hi", Provider: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	evs := ev.waitFinal(t, run)
	if len(evs) != 1 || evs[0].Kind != "error" || !strings.Contains(evs[0].Error, "not logged in") {
		t.Errorf("events %+v", evs)
	}
}
