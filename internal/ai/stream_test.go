package ai

import (
	"reflect"
	"testing"
)

func TestParseLine(t *testing.T) {
	cases := []struct {
		name  string
		line  string
		want  []Progress
		final bool
	}{
		{"start-up", `{"type":"system","subtype":"init","cwd":"/Users/x","tools":[],"mcp_servers":[{"name":"mormi","status":"connected"}]}`, nil, false},
		{"rate limit note", `{"type":"rate_limit_event","rate_limit_info":{"status":"allowed"}}`, nil, false},
		{"not JSON", `Warning: something`, nil, false},
		{"text", `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Looking for PDFs."}]}}`,
			[]Progress{{Kind: "text", Text: "Looking for PDFs."}}, false},
		{"blank text is left out", `{"type":"assistant","message":{"content":[{"type":"text","text":"  \n"}]}}`, nil, false},
		{"tool call", `{"type":"assistant","message":{"content":[{"type":"text","text":"Searching."},{"type":"tool_use","id":"toolu_1","name":"mcp__mormi__find_files","input":{"namePattern":"*.pdf","depth":-1}}]}}`,
			[]Progress{{Kind: "text", Text: "Searching."}, {Kind: "tool", Tool: "find_files", Args: map[string]any{"namePattern": "*.pdf", "depth": float64(-1)}}}, false},
		{"tool call without input", `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"mcp__mormi__get_state"}]}}`,
			[]Progress{{Kind: "tool", Tool: "get_state", Args: map[string]any{}}}, false},
		{"tool result echoed back", `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"3 files"}]}}`, nil, false},
		{"success", `{"type":"result","subtype":"success","is_error":false,"result":"Found 3 PDFs.","num_turns":2}`,
			[]Progress{{Kind: "done", Text: "Found 3 PDFs."}}, true},
		{"error with text", `{"type":"result","subtype":"success","is_error":true,"result":"Invalid API key"}`,
			[]Progress{{Kind: "error", Error: "Invalid API key"}}, true},
		{"error with a list", `{"type":"result","subtype":"error_during_execution","is_error":true,"errors":["Network down",{"message":"retry later"}]}`,
			[]Progress{{Kind: "error", Error: "Network down retry later"}}, true},
		{"error with nothing said", `{"type":"result","subtype":"error_max_turns","is_error":true}`,
			[]Progress{{Kind: "error", Error: "Claude stopped: it took too many steps."}}, true},
	}
	for _, c := range cases {
		got, final := ParseLine([]byte(c.line))
		if !reflect.DeepEqual(got, c.want) || final != c.final {
			t.Errorf("%s:\n got %#v, %v\nwant %#v, %v", c.name, got, final, c.want, c.final)
		}
	}
}
