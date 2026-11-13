// Tests for the proxy tunnels: tiny SOCKS4a, SOCKS5 and HTTP CONNECT proxies
// run on 127.0.0.1 inside the test and forward to the in-test SSH server, so
// a full SFTP login is made through each one. Authentication success and
// failure are both checked.
package sftp

import (
	"bufio"
	"encoding/base64"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

type testProxy struct {
	ln     net.Listener
	addr   string
	used   atomic.Int32
	target string // every tunnel goes here (the SSH test server)
}

func startProxy(t *testing.T, target string, handshake func(c net.Conn, br *bufio.Reader) (string, bool)) *testProxy {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := &testProxy{ln: ln, addr: ln.Addr().String(), target: target}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				br := bufio.NewReader(c)
				requested, ok := handshake(c, br)
				if !ok {
					return
				}
				p.used.Add(1)
				// The client asked for "localhost"-style names in some
				// tests; always forward to the real test server.
				_ = requested
				up, err := net.Dial("tcp", target)
				if err != nil {
					return
				}
				defer up.Close()
				go io.Copy(up, br)
				io.Copy(c, up)
			}()
		}
	}()
	return p
}

func socks5Handshake(user, pass string) func(c net.Conn, br *bufio.Reader) (string, bool) {
	return func(c net.Conn, br *bufio.Reader) (string, bool) {
		hdr := make([]byte, 2)
		io.ReadFull(br, hdr)
		methods := make([]byte, hdr[1])
		io.ReadFull(br, methods)
		if user != "" {
			c.Write([]byte{5, 2})
			v := make([]byte, 2)
			io.ReadFull(br, v)
			u := make([]byte, v[1])
			io.ReadFull(br, u)
			pl, _ := br.ReadByte()
			pw := make([]byte, pl)
			io.ReadFull(br, pw)
			if string(u) != user || string(pw) != pass {
				c.Write([]byte{1, 1})
				return "", false
			}
			c.Write([]byte{1, 0})
		} else {
			c.Write([]byte{5, 0})
		}
		req := make([]byte, 4)
		io.ReadFull(br, req)
		var host string
		switch req[3] {
		case 1:
			b := make([]byte, 4)
			io.ReadFull(br, b)
			host = net.IP(b).String()
		case 3:
			l, _ := br.ReadByte()
			b := make([]byte, l)
			io.ReadFull(br, b)
			host = string(b)
		case 4:
			b := make([]byte, 16)
			io.ReadFull(br, b)
			host = net.IP(b).String()
		}
		pb := make([]byte, 2)
		io.ReadFull(br, pb)
		port := binary.BigEndian.Uint16(pb)
		// Reply with a domain-type bound address to exercise that path.
		c.Write([]byte{5, 0, 0, 3, 4, 'h', 'o', 's', 't', 0, 0})
		return net.JoinHostPort(host, strconv.Itoa(int(port))), true
	}
}

func socks4Handshake(c net.Conn, br *bufio.Reader) (string, bool) {
	hdr := make([]byte, 8)
	io.ReadFull(br, hdr)
	br.ReadString(0) // user id
	host := net.IP(hdr[4:8]).String()
	if hdr[4] == 0 && hdr[5] == 0 && hdr[6] == 0 && hdr[7] != 0 {
		h, _ := br.ReadString(0)
		host = strings.TrimSuffix(h, "\x00")
	}
	c.Write([]byte{0, 0x5a, 0, 0, 0, 0, 0, 0})
	return net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(hdr[2:4])))), true
}

func httpHandshake(user, pass string) func(c net.Conn, br *bufio.Reader) (string, bool) {
	return func(c net.Conn, br *bufio.Reader) (string, bool) {
		req, err := http.ReadRequest(br)
		if err != nil || req.Method != http.MethodConnect {
			return "", false
		}
		if user != "" {
			want := "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
			if req.Header.Get("Proxy-Authorization") != want {
				io.WriteString(c, "HTTP/1.1 407 Proxy Authentication Required\r\n\r\n")
				return "", false
			}
		}
		io.WriteString(c, "HTTP/1.1 200 Connection established\r\n\r\n")
		return req.Host, true
	}
}

func TestProxies(t *testing.T) {
	s := newTestServer(t)
	tests := []struct {
		name      string
		handshake func(net.Conn, *bufio.Reader) (string, bool)
		proxy     Proxy
		host      string
		wantErr   bool
	}{
		{"socks5", socks5Handshake("", ""), Proxy{Type: ProxySOCKS5}, "127.0.0.1", false},
		{"socks5 hostname", socks5Handshake("", ""), Proxy{Type: ProxySOCKS5}, "localhost", false},
		{"socks5 auth", socks5Handshake("pu", "pp"), Proxy{Type: ProxySOCKS5, User: "pu", Password: "pp"}, "127.0.0.1", false},
		{"socks5 bad auth", socks5Handshake("pu", "pp"), Proxy{Type: ProxySOCKS5, User: "pu", Password: "no"}, "127.0.0.1", true},
		{"socks5 auth required but none given", socks5Handshake("pu", "pp"), Proxy{Type: ProxySOCKS5}, "127.0.0.1", true},
		{"socks4", socks4Handshake, Proxy{Type: ProxySOCKS4, User: "me"}, "127.0.0.1", false},
		{"socks4a hostname", socks4Handshake, Proxy{Type: ProxySOCKS4}, "localhost", false},
		{"http", httpHandshake("", ""), Proxy{Type: ProxyHTTP}, "127.0.0.1", false},
		{"http auth", httpHandshake("hu", "hp"), Proxy{Type: ProxyHTTP, User: "hu", Password: "hp"}, "127.0.0.1", false},
		{"http bad auth", httpHandshake("hu", "hp"), Proxy{Type: ProxyHTTP, User: "hu", Password: "x"}, "127.0.0.1", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := startProxy(t, s.Addr, tc.handshake)
			cfg := s.baseConfig(t)
			cfg.InsecureIgnoreHostKey = true // host name may differ from the known_hosts entry
			cfg.Host = tc.host
			px := tc.proxy
			px.Addr = p.addr
			cfg.Proxy = &px
			c, err := Dial(t.Context(), cfg)
			if tc.wantErr {
				if err == nil {
					c.Close()
					t.Fatal("expected failure")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			if _, err := c.ReadDir(s.R); err != nil {
				t.Fatal(err)
			}
			if p.used.Load() != 1 {
				t.Errorf("proxy used %d times", p.used.Load())
			}
		})
	}
}

func TestProxyUnreachable(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	ln.Close()
	p := &Proxy{Type: ProxySOCKS5, Addr: addr}
	if _, err := p.Dial("example.org:22", 0); err == nil {
		t.Fatal("dial through closed proxy succeeded")
	}
	if _, err := (&Proxy{Type: ProxyType(99), Addr: addr}).Dial("x:1", 0); err == nil {
		t.Fatal("unknown type accepted")
	}
}

func TestProxyTypeString(t *testing.T) {
	for typ, want := range map[ProxyType]string{ProxySOCKS5: "socks5", ProxySOCKS4: "socks4", ProxyHTTP: "http", 42: "unknown"} {
		if typ.String() != want {
			t.Errorf("%d = %q", typ, typ.String())
		}
	}
}
