// Package ai holds the UI-free half of the AI layer (issue #146): the MCP endpoint an LLM client reaches the app's
// tools through, the reader of the Claude CLI's stream-json output, and the building of the Claude command line. The
// tools themselves are defined and run by the page; this package only describes them to the model and hands the calls
// on. Nothing here starts a program or opens a port, so all of it is tested with plain values and httptest.
package ai

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
)

// ProtocolVersion is the MCP revision the endpoint speaks when the client names none. A client that names its own
// gets that one echoed back: the endpoint uses so little of the protocol (no SSE, no sessions, no resources) that every
// revision since 2025-03-26 looks the same from here.
const ProtocolVersion = "2025-06-18"

// Tool is one tool the page offers the model. InputSchema is a JSON Schema object ({"type":"object",...}).
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// Server answers the MCP requests of the runs. It holds no state of its own: the tool list, the runs that exist and
// the tool calls are all asked from the functions it is given, so the app keeps them in one place.
type Server struct {
	// Name and Version are what initialize reports as serverInfo.
	Name    string
	Version string
	// Token is the bearer token every request must carry.
	Token string
	// Tools returns the page's current tool list.
	Tools func() []Tool
	// HasRun says whether a run with this id is going on; requests for any other run are refused, so a CLI left over
	// from an ended run can't keep calling tools.
	HasRun func(run int) bool
	// Call runs one tool for a run and returns the text the model reads and whether it is an error. It is given the
	// request's context, which ends when the client goes away (the CLI was stopped).
	Call func(ctx context.Context, run int, name string, args map[string]any) (text string, isError bool)
}

// Handler is the HTTP side of the endpoint: POST /mcp/<run>, behind the same guard as the control mode.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp/", s.handleMCP)
	return Guard(s.Token, mux)
}

// Guard lets a request through when it carries the token, is addressed to this computer by its loopback name and was
// not made by a web page (browsers always send Origin on such requests, other programs don't). It is the guard of the
// control mode (control.go), for the same reason: anything on this computer can reach a loopback port, a web page
// included, and the tools act on the user's files.
func Guard(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		if r.Header.Get("Origin") != "" || (host != "127.0.0.1" && host != "localhost") {
			http.Error(w, "refused", http.StatusForbidden)
			return
		}
		if token == "" || r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "token missing or wrong", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// rpcRequest is one JSON-RPC 2.0 message from the client. A message without an id is a notification and gets no
// answer.
type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// The JSON-RPC error codes the endpoint uses.
const (
	codeParse          = -32700
	codeInvalidRequest = -32600
	codeNoMethod       = -32601
	codeInvalidParams  = -32602
)

func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request) {
	run, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/mcp/"))
	if err != nil || run <= 0 {
		http.NotFound(w, r)
		return
	}
	// The streamable HTTP transport lets a client open a GET stream for messages the server starts; this server never
	// starts any, and 405 is how the specification says so.
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.HasRun != nil && !s.HasRun(run) {
		http.Error(w, "no such run", http.StatusNotFound)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		http.Error(w, "the request could not be read", http.StatusBadRequest)
		return
	}
	var req rpcRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeRPC(w, rpcResponse{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{codeParse, "not a JSON-RPC message: " + err.Error()}})
		return
	}
	if len(req.ID) == 0 || string(req.ID) == "null" {
		// A notification (notifications/initialized, notifications/cancelled) or a response to a request we never
		// sent: accepted, no body, as the transport asks.
		w.WriteHeader(http.StatusAccepted)
		return
	}
	writeRPC(w, s.answer(r.Context(), run, req))
}

// answer handles one JSON-RPC request.
func (s *Server) answer(ctx context.Context, run int, req rpcRequest) rpcResponse {
	resp := rpcResponse{JSONRPC: "2.0", ID: req.ID}
	if req.JSONRPC != "2.0" || req.Method == "" {
		resp.Error = &rpcError{codeInvalidRequest, "not a JSON-RPC 2.0 request"}
		return resp
	}
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		json.Unmarshal(req.Params, &p)
		pv := p.ProtocolVersion
		if pv == "" {
			pv = ProtocolVersion
		}
		resp.Result = map[string]any{
			"protocolVersion": pv,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": s.Name, "version": s.Version},
		}
	case "ping":
		resp.Result = map[string]any{}
	case "tools/list":
		resp.Result = map[string]any{"tools": s.toolList()}
	case "tools/call":
		var p struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil || p.Name == "" {
			resp.Error = &rpcError{codeInvalidParams, "tools/call needs a tool name"}
			return resp
		}
		if p.Arguments == nil {
			p.Arguments = map[string]any{}
		}
		text, isErr := "", true
		if s.Call != nil {
			text, isErr = s.Call(ctx, run, p.Name, p.Arguments)
		} else {
			text = "No tools can be run now."
		}
		resp.Result = map[string]any{
			"content": []map[string]any{{"type": "text", "text": text}},
			"isError": isErr,
		}
	default:
		resp.Error = &rpcError{codeNoMethod, "unknown method " + req.Method}
	}
	return resp
}

// toolList is the page's tools as tools/list reports them. A tool without a schema gets an empty object schema,
// because MCP clients reject a tool whose inputSchema is missing.
func (s *Server) toolList() []Tool {
	out := []Tool{}
	if s.Tools == nil {
		return out
	}
	for _, t := range s.Tools() {
		if t.InputSchema == nil {
			t.InputSchema = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		out = append(out, t)
	}
	return out
}

func writeRPC(w http.ResponseWriter, resp rpcResponse) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
