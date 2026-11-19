package ai

// OpenRouter, or any other server with an OpenAI-compatible chat completions API, as the second provider (issue #146,
// step 2). Unlike the Claude CLI, which runs the conversation itself and reaches the tools over MCP, here the app runs
// the conversation: it sends the prompt and the tool list, runs each tool call the model answers with through the
// page, sends the results back, and asks again until the model gives a final answer (Loop). This file builds the JSON
// requests and reads the answers; the one function that sends them over the network is in chat_http.go, so the
// zero-network test (privacy_test.go) can allow exactly that function.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// DefaultBaseURL is OpenRouter's API, the default of the AI.OpenRouterURL option.
const DefaultBaseURL = "https://openrouter.ai/api/v1"

// ProjectURL and ProjectTitle are what OpenRouter's optional HTTP-Referer and X-Title headers say: the app, never the
// user. OpenRouter shows them in its usage lists; other servers are not sent them.
const (
	ProjectURL   = "https://github.com/mormitech/mormicommander"
	ProjectTitle = "mormiCommander"
)

// MaxRounds is how many answers with tool calls a run may have before it is ended: a model that keeps calling tools
// without ever answering would otherwise run (and cost money) without end.
const MaxRounds = 25

// ChatMessage is one message of the conversation, as the chat completions API has it. Content is sent even when it
// is empty, since a "tool" message needs it and an assistant message with tool calls may have it empty.
type ChatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolCall is one call the model asks for. Arguments is a JSON object written as a string, as the API sends it.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall is the function part of a ToolCall.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ChatTool is one tool in the request's "tools" list (the OpenAI function format).
type ChatTool struct {
	Type     string       `json:"type"`
	Function ChatFunction `json:"function"`
}

// ChatFunction describes a tool to the model: the same name, description and JSON Schema the MCP path offers.
type ChatFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters"`
}

// ChatTools turns the page's tools into the request's "tools" list. A tool without a schema gets an empty object
// schema, since the API wants one for every function.
func ChatTools(tools []Tool) []ChatTool {
	out := make([]ChatTool, 0, len(tools))
	for _, t := range tools {
		params := t.InputSchema
		if params == nil {
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		out = append(out, ChatTool{Type: "function", Function: ChatFunction{Name: t.Name, Description: t.Description, Parameters: params}})
	}
	return out
}

// chatRequest is the body of POST {url}/chat/completions. Non-streaming: the page shows each step when it is
// complete, so streaming would only add work.
type chatRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Tools    []ChatTool    `json:"tools,omitempty"`
}

// apiErrorBody is the "error" object the API answers with when something went wrong (also, with OpenRouter, inside a
// 200 answer when the model's provider failed halfway).
type apiErrorBody struct {
	Code    any    `json:"code"`
	Message string `json:"message"`
}

// chatResponse is the part of the answer the loop reads.
type chatResponse struct {
	Choices []struct {
		Message struct {
			Content   json.RawMessage `json:"content"`
			Refusal   string          `json:"refusal"`
			ToolCalls []ToolCall      `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *apiErrorBody `json:"error"`
}

// Reply is one answer of the model: its text and the tool calls it asks for.
type Reply struct {
	Text         string
	ToolCalls    []ToolCall
	FinishReason string
}

// contentText reads a message's content: a string in most answers, null next to tool calls, and a list of parts
// ({"type": "text", "text": ...}) with some servers.
func contentText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) == nil {
		var b strings.Builder
		for _, p := range parts {
			if p.Type == "text" || p.Type == "" {
				b.WriteString(p.Text)
			}
		}
		return b.String()
	}
	return ""
}

// APIError is a failed request, with a sentence for the user.
type APIError struct {
	Status  int    // the HTTP status (0 when the error came inside a 200 answer without a code)
	Message string // the sentence shown to the user
}

func (e *APIError) Error() string { return e.Message }

// providerMessage is the error text the server gave: the "error" object's message when the body is the usual JSON,
// else the start of the body as text (a proxy's HTML page says little, but something).
func providerMessage(body []byte) string {
	var r struct {
		Error *apiErrorBody `json:"error"`
	}
	if json.Unmarshal(body, &r) == nil && r.Error != nil && strings.TrimSpace(r.Error.Message) != "" {
		return strings.TrimSpace(r.Error.Message)
	}
	s := strings.TrimSpace(string(body))
	if strings.HasPrefix(s, "<") {
		return ""
	}
	if len(s) > 300 {
		s = s[:300] + "…"
	}
	return s
}

// MapError turns an HTTP status and the error text of the server into a sentence that says what to do. The common
// ones are named (a refused key, no credits left, a rate limit, a model that does not exist); the rest keep the
// server's own words.
func MapError(status int, msg string) *APIError {
	with := func(s string) string {
		if msg == "" {
			return s + "."
		}
		return s + ": " + msg
	}
	lower := strings.ToLower(msg)
	var m string
	switch {
	case status == 401:
		m = with("The API key was refused (401). Check it in Configuration > AI settings")
	case status == 402:
		m = with("The account has no credits left for this request (402). Add credits at the provider, or choose a free or cheaper model")
	case status == 403:
		m = with("The provider refused the request (403)")
	case status == 429:
		m = with("Rate limit reached (429): wait a moment and try again, or choose another model")
	case status == 404 || (status == 400 && strings.Contains(lower, "model") &&
		(strings.Contains(lower, "not a valid model") || strings.Contains(lower, "not found") || strings.Contains(lower, "does not exist") || strings.Contains(lower, "no endpoints"))):
		m = with(fmt.Sprintf("The model or the API address was not found (%d). Check both in Configuration > AI settings", status))
	case status == 400:
		m = with("The provider refused the request (400)")
	case status == 408 || status == 504 || status == 524:
		m = with(fmt.Sprintf("The provider took too long to answer (%d). Try again", status))
	case status >= 500:
		m = with(fmt.Sprintf("The provider had an error (%d). Try again in a moment", status))
	case status == 0:
		m = with("The provider ended with an error")
	default:
		m = with(fmt.Sprintf("The request failed (HTTP %d)", status))
	}
	return &APIError{Status: status, Message: m}
}

// errorCode reads the "code" of an error object, which is a number with OpenRouter and a string with others.
func errorCode(c any) int {
	if f, ok := c.(float64); ok {
		return int(f)
	}
	return 0
}

// Client talks to one OpenAI-compatible API.
type Client struct {
	BaseURL string // like https://openrouter.ai/api/v1
	Key     string // the API key; "" sends no Authorization header
	// LocalOnly limits the client to this computer: the address must be a localhost one (CheckLocalURL), and the
	// connection is refused when it would reach any other address, after name resolution or a redirect (the rename
	// plan of issue #33, whose file names must never leave localhost).
	LocalOnly bool
}

// endpoint is BaseURL with a path added.
func (c *Client) endpoint(path string) string {
	return strings.TrimRight(strings.TrimSpace(c.BaseURL), "/") + path
}

// CheckBaseURL says what is wrong with an API address, or nil. It must be https, except on this computer (a local
// server such as Ollama or LM Studio listens on http://localhost): over plain http to another machine the API key
// would cross the network readable by anyone on the way.
func CheckBaseURL(s string) error {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return errors.New("The API address must be a web address starting with https://.")
	}
	if u.User != nil {
		return errors.New("The API address must not hold a user name or password; the API key is kept in the system keychain.")
	}
	if u.Scheme == "http" {
		switch u.Hostname() {
		case "localhost", "127.0.0.1", "::1":
		default:
			return errors.New("The API address must start with https:// (plain http only for a server on this computer, localhost).")
		}
	}
	return nil
}

// isOpenRouter says whether the API is OpenRouter's, which alone is sent the app's name headers.
func isOpenRouter(base string) bool {
	u, err := url.Parse(strings.TrimSpace(base))
	return err == nil && (u.Hostname() == "openrouter.ai" || strings.HasSuffix(u.Hostname(), ".openrouter.ai"))
}

// do sends one request and returns the body of a successful answer, or an APIError / a network error.
func (c *Client) do(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	if err := CheckBaseURL(c.BaseURL); err != nil {
		return nil, err
	}
	if c.LocalOnly {
		if err := CheckLocalURL(c.BaseURL); err != nil {
			return nil, err
		}
	}
	status, resp, err := send(ctx, method, c.endpoint(path), c.Key, isOpenRouter(c.BaseURL), c.LocalOnly, body)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		host := c.BaseURL
		if u, perr := url.Parse(c.BaseURL); perr == nil && u.Host != "" {
			host = u.Host
		}
		return nil, fmt.Errorf("The AI provider (%s) could not be reached: %v", host, err)
	}
	if status < 200 || status > 299 {
		return nil, MapError(status, providerMessage(resp))
	}
	return resp, nil
}

// Complete asks the model for its next answer to the conversation.
func (c *Client) Complete(ctx context.Context, model string, messages []ChatMessage, tools []ChatTool) (Reply, error) {
	body, err := json.Marshal(chatRequest{Model: model, Messages: messages, Tools: tools})
	if err != nil {
		return Reply{}, err
	}
	resp, err := c.do(ctx, "POST", "/chat/completions", body)
	if err != nil {
		return Reply{}, err
	}
	return parseReply(resp)
}

// parseReply reads a chat completions answer.
func parseReply(resp []byte) (Reply, error) {
	var r chatResponse
	if err := json.Unmarshal(resp, &r); err != nil {
		return Reply{}, &APIError{Message: "The provider's answer could not be read (it is not the expected JSON)."}
	}
	if r.Error != nil {
		return Reply{}, MapError(errorCode(r.Error.Code), strings.TrimSpace(r.Error.Message))
	}
	if len(r.Choices) == 0 {
		return Reply{}, &APIError{Message: "The provider answered without a message."}
	}
	ch := r.Choices[0]
	text := contentText(ch.Message.Content)
	if strings.TrimSpace(text) == "" && ch.Message.Refusal != "" {
		text = ch.Message.Refusal
	}
	return Reply{Text: text, ToolCalls: ch.Message.ToolCalls, FinishReason: ch.FinishReason}, nil
}

// Model is one model the API offers, for the model list of the AI settings. ContextLength and Pricing are given by
// OpenRouter (and some others); a plain OpenAI-compatible server lists ids only. Prices are per token, in US dollars,
// as the API writes them (the page shows them per million tokens). NoTools says the API lists the parameters the
// model supports and "tools" is not among them, so it can't search with the app's tools.
type Model struct {
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	ContextLength int           `json:"contextLength"`
	Pricing       *ModelPricing `json:"pricing,omitempty"`
	NoTools       bool          `json:"noTools"`
}

// ModelPricing is the price of a model's input (prompt) and output (completion) tokens.
type ModelPricing struct {
	Prompt     string `json:"prompt"`
	Completion string `json:"completion"`
}

// Models lists the models of GET {url}/models.
func (c *Client) Models(ctx context.Context) ([]Model, error) {
	resp, err := c.do(ctx, "GET", "/models", nil)
	if err != nil {
		return nil, err
	}
	return parseModels(resp)
}

// parseModels reads the model list: {"data": [{id, name, context_length, pricing, supported_parameters}]}.
func parseModels(resp []byte) ([]Model, error) {
	var r struct {
		Data []struct {
			ID            string `json:"id"`
			Name          string `json:"name"`
			ContextLength int    `json:"context_length"`
			Pricing       *struct {
				Prompt     any `json:"prompt"`
				Completion any `json:"completion"`
			} `json:"pricing"`
			SupportedParameters []string `json:"supported_parameters"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp, &r); err != nil {
		return nil, &APIError{Message: "The model list could not be read (it is not the expected JSON)."}
	}
	out := make([]Model, 0, len(r.Data))
	for _, d := range r.Data {
		if d.ID == "" {
			continue
		}
		m := Model{ID: d.ID, Name: strings.TrimSpace(d.Name), ContextLength: d.ContextLength}
		if m.Name == "" {
			m.Name = d.ID
		}
		if d.Pricing != nil {
			m.Pricing = &ModelPricing{Prompt: priceText(d.Pricing.Prompt), Completion: priceText(d.Pricing.Completion)}
		}
		if d.SupportedParameters != nil {
			m.NoTools = true
			for _, p := range d.SupportedParameters {
				if p == "tools" {
					m.NoTools = false
				}
			}
		}
		out = append(out, m)
	}
	return out, nil
}

// priceText reads a price, which OpenRouter writes as a string ("0.000003") and others as a number.
func priceText(v any) string {
	switch p := v.(type) {
	case string:
		return p
	case float64:
		return fmt.Sprint(p)
	}
	return ""
}

// Loop runs one prompt to its end: it asks the model, runs the tool calls of its answer, sends the results back, and
// asks again, until the model answers without tool calls (the final answer) or MaxRounds answers with tool calls have
// gone by.
type Loop struct {
	Client *Client
	Model  string
	Tools  []Tool
	// MaxRounds overrides the package's MaxRounds when it is above 0 (tests use a small one).
	MaxRounds int
	// Call runs one tool through the page and returns its text, and whether it is an error.
	Call func(ctx context.Context, name string, args map[string]any) (string, bool)
	// Emit shows one step of the run ("text" or "tool") to the page.
	Emit func(Progress)
}

// Run runs the conversation and returns its last step: "done" with the final answer, or "error" with what went wrong
// ("Stopped." when ctx was cancelled). The steps before it go to Emit.
func (l Loop) Run(ctx context.Context, systemPrompt, prompt string) Progress {
	rounds := l.MaxRounds
	if rounds <= 0 {
		rounds = MaxRounds
	}
	known := map[string]bool{}
	for _, t := range l.Tools {
		known[t.Name] = true
	}
	tools := ChatTools(l.Tools)
	messages := []ChatMessage{{Role: "system", Content: systemPrompt}, {Role: "user", Content: prompt}}
	stopped := Progress{Kind: "error", Error: "Stopped."}
	for round := 0; ; round++ {
		if ctx.Err() != nil {
			return stopped
		}
		reply, err := l.Client.Complete(ctx, l.Model, messages, tools)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, context.Canceled) {
				return stopped
			}
			return Progress{Kind: "error", Error: err.Error()}
		}
		if len(reply.ToolCalls) == 0 {
			if reply.FinishReason == "length" && strings.TrimSpace(reply.Text) == "" {
				return Progress{Kind: "error", Error: "The answer was cut off: the model reached its output limit."}
			}
			return Progress{Kind: "done", Text: strings.TrimSpace(reply.Text)}
		}
		if round >= rounds {
			return Progress{Kind: "error", Error: fmt.Sprintf("The model called tools %d times without giving an answer, so the run was ended. Try a more precise prompt.", rounds)}
		}
		if strings.TrimSpace(reply.Text) != "" {
			l.Emit(Progress{Kind: "text", Text: reply.Text})
		}
		messages = append(messages, ChatMessage{Role: "assistant", Content: reply.Text, ToolCalls: reply.ToolCalls})
		for i, tc := range reply.ToolCalls {
			if tc.ID == "" {
				// (some servers leave the id out; the tool message must still name the call it answers)
				tc.ID = fmt.Sprintf("call_%d_%d", round, i)
				reply.ToolCalls[i].ID = tc.ID
			}
			messages = append(messages, ChatMessage{Role: "tool", ToolCallID: tc.ID, Content: l.runCall(ctx, tc, known)})
			if ctx.Err() != nil {
				return stopped
			}
		}
	}
}

// runCall runs one tool call and returns what the model reads back. Arguments that are not a JSON object and names
// that are not among the page's tools are answered with an error at once, without reaching the page.
func (l Loop) runCall(ctx context.Context, tc ToolCall, known map[string]bool) string {
	name := tc.Function.Name
	args := map[string]any{}
	raw := strings.TrimSpace(tc.Function.Arguments)
	var bad error
	if raw != "" {
		bad = json.Unmarshal([]byte(raw), &args)
		if args == nil {
			args = map[string]any{}
		}
	}
	l.Emit(Progress{Kind: "tool", Tool: name, Args: args})
	if !known[name] {
		return fmt.Sprintf("Error: there is no tool named %q.", name)
	}
	if bad != nil {
		return fmt.Sprintf("Error: the arguments of %s are not a JSON object: %v", name, bad)
	}
	text, isErr := l.Call(ctx, name, args)
	if isErr {
		return "Error: " + text
	}
	return text
}
