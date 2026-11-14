package ftp

import (
	"net"
	"testing"
)

// addrConn is a control connection that only knows its peer address.
type addrConn struct {
	net.Conn
	remote net.Addr
}

func (c addrConn) RemoteAddr() net.Addr { return c.remote }

// TestPassiveLoopbackNeedsLoopbackServer: in the automatic mode a PASV reply that points at 127.0.0.1 while the server
// itself is another machine (here one on the LAN) is replaced with the server's address. Dialling it would connect to a
// service on this computer: a hostile server could make the client send an upload to, or read a "listing" from, any
// local port.
func TestPassiveLoopbackNeedsLoopbackServer(t *testing.T) {
	tests := []struct {
		ctrl, pasv string
		replace    bool
	}{
		{"192.168.1.10", "127.0.0.1", true},
		{"203.0.113.5", "127.0.0.1", true},
		{"127.0.0.1", "127.0.0.1", false},
		{"192.168.1.10", "192.168.1.10", false},
		{"192.168.1.10", "192.168.1.11", false}, // a multi-homed LAN server: left as it is
		{"203.0.113.5", "10.0.0.1", true},
		{"203.0.113.5", "0.0.0.0", true},
	}
	for _, tc := range tests {
		c := &Client{conn: addrConn{remote: &net.TCPAddr{IP: net.ParseIP(tc.ctrl), Port: 21}}}
		if got := c.replacePassiveHost(net.ParseIP(tc.pasv)); got != tc.replace {
			t.Errorf("control %s, PASV %s: replace = %v, want %v", tc.ctrl, tc.pasv, got, tc.replace)
		}
	}
}
