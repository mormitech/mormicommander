package main

// The AI layer, Go side (issue #146): the user writes a prompt, an LLM answers by calling mormiCommander's own tools,
// never shell commands. The tools are defined and run by the page (frontend/src/core/aitools.js), where the panels,
// the selection and the Find dialog live; this side only relays. There are two providers. The Claude Code CLI (step 1)
// reaches the tools through an MCP endpoint served by the app on 127.0.0.1 (feature_ai_mcp.go); the CLI is started per
// run by feature_ai_claude.go. OpenRouter, or any OpenAI-compatible chat completions API (step 2), is talked to by the
// app itself, which runs the tool calls through the same page relay (feature_ai_openrouter.go); its API key is kept in
// the system keychain. The pure parts (the MCP protocol, the CLI's output, its command line, the chat completions
// JSON and loop) are in internal/ai.
//
// The page talks to it like this: SetTools at start; Start(request) gives a run id; "ai-progress" events tell what the
// run does and end with exactly one "done" or "error"; each tool call of the model arrives as an "ai-tool" event and is
// answered with ToolReply; Stop ends a run early.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"mormicommander/internal/ai"
	"mormicommander/internal/keychain"
)

// AITool is one tool the page offers the model: a name, a description for the model, and a JSON Schema of its input.
type AITool = ai.Tool

// AIRequest is one prompt to run. Context is a short text from the page about where the user is (it goes into the
// system prompt, before the rules); Provider is "claude" or "openrouter" (empty means the one chosen in Options > AI);
// Model is empty for the model chosen in Options > AI (for Claude, empty there too means the CLI's default).
type AIRequest struct {
	Prompt   string `json:"prompt"`
	Context  string `json:"context"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// AIProvider says whether one provider can be used, and where it was found: Path and Version for the Claude CLI,
// Model and URL for OpenRouter. Error says what is missing when it can't be used.
type AIProvider struct {
	Available bool   `json:"available"`
	Path      string `json:"path"`
	Version   string `json:"version"`
	Model     string `json:"model,omitempty"`
	URL       string `json:"url,omitempty"`
	Error     string `json:"error"`
}

// AIProviders is the state of every provider, for the AI tab of the Find dialog and the AI settings. Selected is the
// provider chosen in Options > AI ("claude" or "openrouter"), the one a run with no provider of its own uses.
type AIProviders struct {
	Selected   string     `json:"selected"`
	Claude     AIProvider `json:"claude"`
	OpenRouter AIProvider `json:"openrouter"`
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
	// openKeychain opens the system keychain, where the OpenRouter API key is kept; tests put a fake one here, so
	// they never touch the user's real keychain.
	openKeychain func() (keychain.Keychain, error)

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

	kcOnce sync.Once
	kc     keychain.Keychain
	kcErr  error
	keys   map[string]string    // API keys read from the keychain this session, by provider ("" = none saved)
	models map[string][]AIModel // model lists by API address, for the session
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
	x.openKeychain = keychain.Open
	x.keys = map[string]string{}
	x.models = map[string][]AIModel{}
	return x
}

// aiOptions is the AI page of the Options dialog.
type aiOptions struct {
	Provider int
	Path     string
	Model    string
	ORModel  string // OpenRouter's model
	ORURL    string // OpenRouter's API address ("" = OpenRouter's own)
	// LocalURL and LocalModel are the local model of the Multi-Rename tool's AI rename plan ("" = off; issue #33).
	LocalURL   string
	LocalModel string
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
	opt := cfg.Settings.AI
	return aiOptions{Provider: opt.Provider, Path: opt.ClaudePath, Model: opt.ClaudeModel, ORModel: opt.OpenRouterModel, ORURL: opt.OpenRouterURL,
		LocalURL: opt.LocalRenameURL, LocalModel: opt.LocalRenameModel}
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
// its version; for OpenRouter, whether a key is saved and a model chosen.
func (x *AI) Providers() AIProviders {
	opt := x.options()
	sel := aiProviderName(opt.Provider)
	return AIProviders{Selected: sel, Claude: x.claudeProvider(opt.Path), OpenRouter: x.openRouterProvider(opt, sel == "openrouter")}
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
	if provider == "openrouter" {
		return x.startOpenRouter(opt, req)
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
	case 1:
		return "openrouter"
	}
	return ""
}

// Stop ends a run: the CLI is ended (or the request to the API cancelled), and the run's last event is an "error" with "Stopped." (unless it had just
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

// aiRun is one run: of the CLI (cmd), or of the chat completions loop (cancel).
type aiRun struct {
	id      int
	cmd     *exec.Cmd
	cancel  context.CancelFunc
	once    sync.Once
	ended   chan struct{} // closed when the run's last event was sent
	exited  chan struct{} // closed when the CLI has exited
	cfgFile string
}

// kill ends the CLI and the programs it started, if it is still running, or cancels the loop's request.
func (r *aiRun) kill() {
	if r.cancel != nil {
		r.cancel()
	}
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

// aiHome is the folder the CLI runs in: an empty folder of mormiCommander's own (mormicommander-ai in the temporary
// folder, readable by the user only), not the home folder or a project. Claude Code reads CLAUDE.md, skills and
// settings from the folder it starts in, and the user's own ones there (rules for unrelated work) must not steer a file
// search. The folder is made again when it is missing; the temporary folder itself is the fallback.
//
// On Linux the temporary folder is /tmp, which every user shares, so another user can make mormicommander-ai there
// first (writable for all, or a link to a folder of theirs) and put a .claude/settings.json with hooks in it, which
// the CLI would run as this user. A folder that is not a real folder of this user's, closed to others, is therefore
// not used; a new private folder (made once per run of the app) takes its place.
func aiHome() string {
	d := filepath.Join(os.TempDir(), "mormicommander-ai")
	if err := os.Mkdir(d, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
		return os.TempDir()
	}
	if aiPrivateDir(d) {
		return d
	}
	aiHomeMu.Lock()
	defer aiHomeMu.Unlock()
	if aiHomeOwn == "" || !aiPrivateDir(aiHomeOwn) {
		own, err := os.MkdirTemp("", "mormicommander-ai-")
		if err != nil {
			return os.TempDir()
		}
		aiHomeOwn = own
	}
	return aiHomeOwn
}

// aiHomeOwn is the private folder aiHome made because the usual one could not be trusted.
var (
	aiHomeMu  sync.Mutex
	aiHomeOwn string
)

// aiServerHandler is the endpoint's HTTP handler for this AI object.
func (x *AI) aiServerHandler(token string) http.Handler {
	s := &ai.Server{Name: "mormicommander", Version: version(), Token: token, Tools: x.toolList, HasRun: x.hasRun, Call: x.callTool}
	return s.Handler()
}
