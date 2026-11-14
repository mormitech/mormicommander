package main

// The AI layer, Go side (issue #146): the user writes a prompt, an LLM answers by calling mormiCommander's own tools,
// never shell commands. The tools are defined and run by the page (frontend/src/core/aitools.js), where the panels,
// the selection and the Find dialog live; this side only relays. Step 1 has one provider, the Claude Code CLI, which
// reaches the tools through an MCP endpoint served by the app on 127.0.0.1 (feature_ai_mcp.go); the CLI is started per
// run by feature_ai_claude.go. The pure parts (the MCP protocol, the CLI's output, its command line) are in internal/ai.
//
// The page talks to it like this: SetTools at start; Start(request) gives a run id; "ai-progress" events tell what the
// run does and end with exactly one "done" or "error"; each tool call of the model arrives as an "ai-tool" event and is
// answered with ToolReply; Stop ends a run early.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"mormicommander/internal/ai"
)

// AITool is one tool the page offers the model: a name, a description for the model, and a JSON Schema of its input.
type AITool = ai.Tool

// AIRequest is one prompt to run. Context is a short text from the page about where the user is (it goes into the
// system prompt, before the rules); Provider is "claude" (the only one in step 1; empty means the one chosen in
// Options > AI); Model is empty for the CLI's default.
type AIRequest struct {
	Prompt   string `json:"prompt"`
	Context  string `json:"context"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// AIProvider says whether one provider can be used, and where it was found.
type AIProvider struct {
	Available bool   `json:"available"`
	Path      string `json:"path"`
	Version   string `json:"version"`
	Error     string `json:"error"`
}

// AIProviders is the state of every provider, for the AI tab of the Find dialog.
type AIProviders struct {
	Claude AIProvider `json:"claude"`
}

// AI is what the page calls for the AI layer (window.go.main.AI).
type AI struct {
	app *App
	// emit sends an event to the page; tests replace it to see the events.
	emit func(event string, data ...any)
	// replyTimeout is how long a tool call waits for the page.
	replyTimeout time.Duration
	// finder finds the claude program; tests give one with made-up folders.
	finder func() ai.Finder
	// options reads the AI page of the Options dialog; tests replace it.
	options func() aiOptions

	mu      sync.Mutex
	tools   []AITool
	runs    map[int]*aiRun
	runSeq  int
	callSeq int
	waiting map[int]chan string
	mcp     *aiMCP // the MCP endpoint once the first run started it

	shellOnce sync.Once
	shellPath string            // where the login shell found claude ("" when it did not)
	versions  map[string]string // `claude --version` by program path
}

// aiFeature is the one AI object, for the shut-down hook.
var aiFeature *AI

func init() {
	registerFeature("ai", func(a *App) []exposed {
		aiFeature = newAI(a)
		return []exposed{expose(aiFeature)}
	})
	onShutdown(func(a *App) {
		if aiFeature != nil {
			aiFeature.shutdown()
		}
	})
}

func newAI(a *App) *AI {
	x := &AI{app: a, emit: a.emit, replyTimeout: 120 * time.Second, runs: map[int]*aiRun{}, waiting: map[int]chan string{},
		versions: map[string]string{}}
	x.finder = x.systemFinder
	x.options = readAIOptions
	return x
}

// aiOptions is the AI page of the Options dialog.
type aiOptions struct {
	Provider int
	Path     string
	Model    string
}

// readAIOptions reads the AI page of the Options dialog (the defaults when the settings can't be read).
func readAIOptions() aiOptions {
	if configFeature == nil {
		return aiOptions{}
	}
	cfg, err := configFeature.get()
	if err != nil {
		return aiOptions{}
	}
	defer configFeature.mu.Unlock()
	return aiOptions{Provider: cfg.Settings.AI.Provider, Path: cfg.Settings.AI.ClaudePath, Model: cfg.Settings.AI.ClaudeModel}
}

// SetTools registers the page's tools (it may be called again; the latest list counts from the next tools/list on).
func (x *AI) SetTools(tools []AITool) {
	x.mu.Lock()
	defer x.mu.Unlock()
	x.tools = append([]AITool(nil), tools...)
}

// toolList is the current tool list (for tools/list).
func (x *AI) toolList() []ai.Tool {
	x.mu.Lock()
	defer x.mu.Unlock()
	return append([]ai.Tool(nil), x.tools...)
}

// ToolReply is the page's answer to one "ai-tool" event: JSON {"text": ..., "error": ...}.
func (x *AI) ToolReply(id int, result string) {
	x.mu.Lock()
	ch := x.waiting[id]
	delete(x.waiting, id)
	x.mu.Unlock()
	if ch != nil {
		ch <- result
	}
}

// aiToolCall is the "ai-tool" event's second argument.
type aiToolCall struct {
	Run  int            `json:"run"`
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

// callTool hands one tool call of the model to the page and waits for its answer. The wait ends early when the
// request's context does (the CLI went away, as when the run is stopped), so a stopped run leaves nothing waiting.
func (x *AI) callTool(ctx context.Context, run int, name string, args map[string]any) (string, bool) {
	x.mu.Lock()
	x.callSeq++
	id := x.callSeq
	ch := make(chan string, 1)
	x.waiting[id] = ch
	x.mu.Unlock()
	forget := func() {
		x.mu.Lock()
		delete(x.waiting, id)
		x.mu.Unlock()
	}
	x.emit("ai-tool", id, aiToolCall{Run: run, Name: name, Args: args})
	select {
	case s := <-ch:
		return aiReplyText(s)
	case <-ctx.Done():
		forget()
		return "The run was stopped.", true
	case <-time.After(x.replyTimeout):
		forget()
		return fmt.Sprintf("mormiCommander did not answer the %s call in %v.", name, x.replyTimeout), true
	}
}

// aiReplyText turns the page's answer into what the model reads: the error when there is one, else the text. An
// answer that is not the expected JSON is passed on as it is rather than lost.
func aiReplyText(s string) (string, bool) {
	var r struct {
		Text  string `json:"text"`
		Error string `json:"error"`
	}
	if json.Unmarshal([]byte(s), &r) != nil {
		return s, false
	}
	if r.Error != "" {
		return r.Error, true
	}
	return r.Text, false
}

// Providers tells which providers can be used: for Claude Code, whether the claude command was found, where, and
// its version.
func (x *AI) Providers() AIProviders {
	opt := x.options()
	return AIProviders{Claude: x.claudeProvider(opt.Path)}
}

// claudeProvider finds the claude program (configured is the AI.ClaudePath option).
func (x *AI) claudeProvider(configured string) AIProvider {
	p, ok := x.finder().Find(configured)
	if !ok {
		if strings.TrimSpace(configured) != "" {
			return AIProvider{Path: p, Error: fmt.Sprintf("The Claude command set in Options > AI (%s) is not there.", p)}
		}
		return AIProvider{Error: "Claude Code was not found. Install it (https://claude.com/claude-code), or set the path of the claude command in Options > AI."}
	}
	return AIProvider{Available: true, Path: p, Version: x.claudeVersion(p)}
}

// Start begins a run and returns its id. The answer comes as "ai-progress" events.
func (x *AI) Start(req AIRequest) (int, error) {
	if strings.TrimSpace(req.Prompt) == "" {
		return 0, errors.New("Write what the AI should do first.")
	}
	opt := x.options()
	provider := strings.ToLower(strings.TrimSpace(req.Provider))
	if provider == "" {
		provider = aiProviderName(opt.Provider)
	}
	if provider != "claude" {
		return 0, fmt.Errorf("The AI provider %q is not available.", req.Provider)
	}
	prov := x.claudeProvider(opt.Path)
	if !prov.Available {
		return 0, errors.New(prov.Error)
	}
	m, err := x.endpoint()
	if err != nil {
		return 0, fmt.Errorf("The AI tools could not be offered: %v", err)
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = strings.TrimSpace(opt.Model)
	}
	return x.startClaude(prov.Path, m, req, model)
}

// aiProviderName is the provider an AI.Provider value stands for.
func aiProviderName(v int) string {
	switch v {
	case 0:
		return "claude"
	}
	return ""
}

// Stop ends a run: the CLI is ended, and the run's last event is an "error" with "Stopped." (unless it had just
// ended by itself).
func (x *AI) Stop(run int) {
	x.mu.Lock()
	r := x.runs[run]
	x.mu.Unlock()
	if r == nil {
		return
	}
	x.finish(r, ai.Progress{Kind: "error", Error: "Stopped."})
	r.kill()
}

// aiRun is one run of the CLI.
type aiRun struct {
	id      int
	cmd     *exec.Cmd
	once    sync.Once
	ended   chan struct{} // closed when the run's last event was sent
	exited  chan struct{} // closed when the CLI has exited
	cfgFile string
}

// kill ends the CLI and the programs it started, if it is still running.
func (r *aiRun) kill() {
	if r.cmd == nil || r.cmd.Process == nil {
		return
	}
	select {
	case <-r.exited:
		return
	default:
	}
	aiKillTree(r.cmd, r.exited)
}

// finish sends a run's last event, once: whichever comes first of the CLI's result, its exit, and Stop.
func (x *AI) finish(r *aiRun, last ai.Progress) {
	r.once.Do(func() {
		close(r.ended)
		x.emit("ai-progress", r.id, last)
	})
}

// hasRun says whether a run is going on (its last event not sent yet), for the MCP endpoint.
func (x *AI) hasRun(id int) bool {
	x.mu.Lock()
	r := x.runs[id]
	x.mu.Unlock()
	if r == nil {
		return false
	}
	select {
	case <-r.ended:
		return false
	default:
		return true
	}
}

// shutdown stops every run and closes the endpoint, when the app ends.
func (x *AI) shutdown() {
	x.mu.Lock()
	runs := make([]*aiRun, 0, len(x.runs))
	for _, r := range x.runs {
		runs = append(runs, r)
	}
	m := x.mcp
	x.mcp = nil
	x.mu.Unlock()
	for _, r := range runs {
		x.finish(r, ai.Progress{Kind: "error", Error: "Stopped."})
		r.kill()
	}
	if m != nil {
		m.close()
	}
}

// aiHome is the folder the CLI runs in: the user's home, so it never starts inside a project folder whose own
// settings it would pick up.
func aiHome() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return os.TempDir()
	}
	return h
}

// aiServerHandler is the endpoint's HTTP handler for this AI object.
func (x *AI) aiServerHandler(token string) http.Handler {
	s := &ai.Server{Name: "mormicommander", Version: version(), Token: token, Tools: x.toolList, HasRun: x.hasRun, Call: x.callTool}
	return s.Handler()
}
