package main

// The AI layer's MCP endpoint (issue #146): the Claude CLI reaches the page's tools through it. It listens on
// 127.0.0.1 only, on a free port, from the first run on (nothing listens before the user starts one), and every request
// needs a random token that only the run's MCP config file holds, behind the control mode's guard (internal/ai.Guard).
// The protocol is in internal/ai; this file only opens and closes the port.

import (
	"fmt"
	"net"
	"net/http"
	"time"
)

// aiMCP is the running endpoint.
type aiMCP struct {
	addr  string // 127.0.0.1:port
	token string
	srv   *http.Server
}

// url is the endpoint of one run.
func (m *aiMCP) url(run int) string { return fmt.Sprintf("http://%s/mcp/%d", m.addr, run) }

func (m *aiMCP) close() { m.srv.Close() }

// endpoint returns the MCP endpoint, starting it the first time.
func (x *AI) endpoint() (*aiMCP, error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.mcp != nil {
		return x.mcp, nil
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	m := &aiMCP{addr: ln.Addr().String(), token: newToken()}
	// No write timeout: a tool call may wait up to two minutes for the page (a long search).
	m.srv = &http.Server{Handler: x.aiServerHandler(m.token), ReadHeaderTimeout: 10 * time.Second}
	go m.srv.Serve(ln)
	x.mcp = m
	return m, nil
}
