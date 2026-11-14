package ai

import (
	"encoding/json"
	"strings"
)

// Progress is one step of a run as the page sees it ("ai-progress"): Kind is "text" (the model's words, maybe in
// pieces), "tool" (the model called Tool with Args), "done" (the run is over, Text is the final answer) or "error"
// (the run failed, Error says why).
type Progress struct {
	Kind  string         `json:"kind"`
	Text  string         `json:"text"`
	Tool  string         `json:"tool"`
	Args  map[string]any `json:"args,omitempty"`
	Error string         `json:"error"`
}

// ToolPrefix is how the Claude CLI names the tools of the "mormi" MCP server: mcp__<server>__<tool>.
const ToolPrefix = "mcp__mormi__"

// streamLine is the part of a stream-json line the reader looks at. The CLI writes one JSON object per line: "system"
// lines (start-up, summaries), "assistant" and "user" messages of the conversation, rate limit notes, and one "result"
// at the end. Only the assistant messages and the result matter to the page.
type streamLine struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype"`
	Message struct {
		Content []struct {
			Type  string         `json:"type"`
			Text  string         `json:"text"`
			Name  string         `json:"name"`
			Input map[string]any `json:"input"`
		} `json:"content"`
	} `json:"message"`
	Result  json.RawMessage `json:"result"`
	IsError bool            `json:"is_error"`
	Errors  []any           `json:"errors"`
}

// ParseLine turns one line of the CLI's stream-json output into the steps it shows: none for the lines the page does
// not care about (and for anything that is not JSON, such as a warning the CLI printed), a "text" or "tool" step per
// block of an assistant message, and a "done" or "error" step for the final result. final says the line was the
// result, after which the run has nothing more to say.
func ParseLine(line []byte) (steps []Progress, final bool) {
	var l streamLine
	if json.Unmarshal(line, &l) != nil {
		return nil, false
	}
	switch l.Type {
	case "assistant":
		for _, b := range l.Message.Content {
			switch b.Type {
			case "text":
				if strings.TrimSpace(b.Text) != "" {
					steps = append(steps, Progress{Kind: "text", Text: b.Text})
				}
			case "tool_use":
				args := b.Input
				if args == nil {
					args = map[string]any{}
				}
				steps = append(steps, Progress{Kind: "tool", Tool: strings.TrimPrefix(b.Name, ToolPrefix), Args: args})
			}
		}
		return steps, false
	case "result":
		text := resultText(l.Result)
		if l.IsError || (l.Subtype != "" && l.Subtype != "success") {
			msg := text
			if msg == "" {
				msg = errorsText(l.Errors)
			}
			if msg == "" {
				msg = resultErrorText(l.Subtype)
			}
			return []Progress{{Kind: "error", Error: msg}}, true
		}
		return []Progress{{Kind: "done", Text: text}}, true
	}
	return nil, false
}

// resultText reads the result field, which is a string in every result seen so far; anything else is shown as JSON
// rather than lost.
func resultText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return strings.TrimSpace(s)
	}
	return string(raw)
}

// errorsText joins the "errors" list some failed results carry instead of a result text.
func errorsText(list []any) string {
	var parts []string
	for _, e := range list {
		switch v := e.(type) {
		case string:
			parts = append(parts, v)
		case map[string]any:
			if m, ok := v["message"].(string); ok {
				parts = append(parts, m)
			}
		}
	}
	return strings.Join(parts, " ")
}

// resultErrorText says in words what an error subtype of the result means.
func resultErrorText(subtype string) string {
	switch subtype {
	case "error_max_turns":
		return "Claude stopped: it took too many steps."
	case "error_during_execution":
		return "Claude stopped with an error."
	}
	return "Claude ended with an error."
}
