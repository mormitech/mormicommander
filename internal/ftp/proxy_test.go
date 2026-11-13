// This file tests connecting through proxies. It starts tiny SOCKS4/4A,
// SOCKS5 (with and without user name and password) and HTTP CONNECT proxies
// on 127.0.0.1 that relay to the in-test FTP server, then browses and
// downloads through them, so both the control connection and the passive
// data connections are proven to go through the tunnel. The FTP-style
// "USER user@host" and "OPEN host" firewalls are tested against the FTP
// test server playing the proxy.
package ftp

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

type testProxy struct {
	ln      net.Listener
	kind    ProxyType
	user    string
	pass    string
	tunnels atomic.Int32
	targets sync.Map // addr -> true
	wg      sync.WaitGroup
}

func newTestProxy(t *testing.T, kind ProxyType, user, pass string) *testProxy {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := &testProxy{ln: ln, kind: kind, user: user, pass: pass}
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			p.wg.Add(1)
			go func() {
				defer p.wg.Done()
				p.handle(c)
			}()
		}
	}()
	t.Cleanup(func() {
		ln.Close()
		p.wg.Wait()
	})
	return p
}

func (p *testProxy) port() int { return p.ln.Addr().(*net.TCPAddr).Port }

func (p *testProxy) handle(c net.Conn) {
	defer c.Close()
	var target string
	var ok bool
	var r io.Reader = c
	switch p.kind {
	case ProxySOCKS5:
		target, ok = p.socks5(c)
	case ProxySOCKS4, ProxySOCKS4A:
		target, ok = p.socks4(c)
	case ProxyHTTP:
		br := bufio.NewReader(c)
		r = br
		target, ok = p.http(c, br)
	}
	if !ok {
		return
	}
	up, err := net.Dial("tcp", target)
	if err != nil {
		return
	}
	defer up.Close()
	p.tunnels.Add(1)
	p.targets.Store(target, true)
	done := make(chan struct{}, 2)
	go func() { io.Copy(up, r); up.(*net.TCPConn).CloseWrite(); done <- struct{}{} }()
	go func() { io.Copy(c, up); c.(*net.TCPConn).CloseWrite(); done <- struct{}{} }()
	<-done
	<-done
}

func (p *testProxy) socks5(c net.Conn) (string, bool) {
	hdr := make([]byte, 2)
	if _, err := io.ReadFull(c, hdr); err != nil || hdr[0] != 5 {
		return "", false
	}
	methods := make([]byte, hdr[1])
	io.ReadFull(c, methods)
	want := byte(0)
	if p.user != "" {
		want = 2
	}
	if !strings.ContainsRune(string(methods), rune(want)) {
		c.Write([]byte{5, 0xff})
		return "", false
	}
	c.Write([]byte{5, want})
	if want == 2 {
		b := make([]byte, 2)
		io.ReadFull(c, b)
		u := make([]byte, b[1])
		io.ReadFull(c, u)
		io.ReadFull(c, b[:1])
		pw := make([]byte, b[0])
		io.ReadFull(c, pw)
		if string(u) != p.user || string(pw) != p.pass {
			c.Write([]byte{1, 1})
			return "", false
		}
		c.Write([]byte{1, 0})
	}
	req := make([]byte, 4)
	if _, err := io.ReadFull(c, req); err != nil || req[1] != 1 {
		return "", false
	}
	var host string
	switch req[3] {
	case 1:
		ip := make([]byte, 4)
		io.ReadFull(c, ip)
		host = net.IP(ip).String()
	case 3:
		l := make([]byte, 1)
		io.ReadFull(c, l)
		h := make([]byte, l[0])
		io.ReadFull(c, h)
		host = string(h)
	case 4:
		ip := make([]byte, 16)
		io.ReadFull(c, ip)
		host = net.IP(ip).String()
	}
	pb := make([]byte, 2)
	io.ReadFull(c, pb)
	c.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0})
	return net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(pb)))), true
}

func (p *testProxy) socks4(c net.Conn) (string, bool) {
	hdr := make([]byte, 8)
	if _, err := io.ReadFull(c, hdr); err != nil || hdr[0] != 4 || hdr[1] != 1 {
		return "", false
	}
	readZ := func() string {
		var b []byte
		one := make([]byte, 1)
		for {
			if _, err := c.Read(one); err != nil || one[0] == 0 {
				return string(b)
			}
			b = append(b, one[0])
		}
	}
	user := readZ()
	host := net.IP(hdr[4:8]).String()
	if hdr[4] == 0 && hdr[5] == 0 && hdr[6] == 0 && hdr[7] != 0 {
		if p.kind != ProxySOCKS4A {
			c.Write([]byte{0, 0x5b, 0, 0, 0, 0, 0, 0})
			return "", false
		}
		host = readZ()
	}
	if p.user != "" && user != p.user {
		c.Write([]byte{0, 0x5d, 0, 0, 0, 0, 0, 0})
		return "", false
	}
	c.Write([]byte{0, 0x5a, 0, 0, 0, 0, 0, 0})
	return net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(hdr[2:4])))), true
}

func (p *testProxy) http(c net.Conn, br *bufio.Reader) (string, bool) {
	req, err := http.ReadRequest(br)
	if err != nil || req.Method != http.MethodConnect {
		return "", false
	}
	if p.user != "" {
		want := "Basic " + base64.StdEncoding.EncodeToString([]byte(p.user+":"+p.pass))
		if req.Header.Get("Proxy-Authorization") != want {
			io.WriteString(c, "HTTP/1.1 407 Proxy Authentication Required\r\n\r\n")
			return "", false
		}
	}
	io.WriteString(c, "HTTP/1.1 200 Connection established\r\n\r\n")
	return req.Host, true
}

func TestTunnelProxies(t *testing.T) {
	tests := []struct {
		name string
		kind ProxyType
		user string
		pass string
		host string // what the FTP config names as host
	}{
		{"socks5", ProxySOCKS5, "", "", "127.0.0.1"},
		{"socks5 auth", ProxySOCKS5, "pu", "pp", "127.0.0.1"},
		{"socks5 host name", ProxySOCKS5, "", "", "localhost"},
		{"socks4", ProxySOCKS4, "", "", "127.0.0.1"},
		{"socks4a host name", ProxySOCKS4A, "id", "", "localhost"},
		{"http connect", ProxyHTTP, "", "", "127.0.0.1"},
		{"http connect auth", ProxyHTTP, "hu", "hp", "127.0.0.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newTestServer(t)
			srv.write("f.txt", "through the tunnel")
			px := newTestProxy(t, tt.kind, tt.user, tt.pass)
			cfg := srv.cfg()
			cfg.Host = tt.host
			cfg.Proxy = &Proxy{Type: tt.kind, Host: "127.0.0.1", Port: px.port(), User: tt.user, Password: tt.pass}
			c, err := Dial(context.Background(), cfg)
			if err != nil {
				t.Fatalf("Dial through %s: %v", tt.name, err)
			}
			defer c.Close()
			ctx := context.Background()
			es, err := c.List(ctx, "")
			if err != nil || len(es) != 1 {
				t.Fatalf("List = %v, %v", es, err)
			}
			dst := filepath.Join(t.TempDir(), "f")
			if _, err := c.Download(ctx, "f.txt", dst, TransferOptions{}); err != nil {
				t.Fatal(err)
			}
			if b, _ := os.ReadFile(dst); string(b) != "through the tunnel" {
				t.Fatalf("got %q", b)
			}
			if n := px.tunnels.Load(); n < 3 {
				t.Errorf("only %d tunnels; data connections bypassed the proxy", n)
			}
		})
	}
}

func TestProxyFailures(t *testing.T) {
	srv := newTestServer(t)
	ctx := context.Background()
	t.Run("bad socks5 password", func(t *testing.T) {
		px := newTestProxy(t, ProxySOCKS5, "u", "right")
		cfg := srv.cfg()
		cfg.Proxy = &Proxy{Type: ProxySOCKS5, Host: "127.0.0.1", Port: px.port(), User: "u", Password: "wrong"}
		if _, err := Dial(ctx, cfg); err == nil {
			t.Fatal("wrong proxy password accepted")
		}
	})
	t.Run("bad http credentials", func(t *testing.T) {
		px := newTestProxy(t, ProxyHTTP, "u", "right")
		cfg := srv.cfg()
		cfg.Proxy = &Proxy{Type: ProxyHTTP, Host: "127.0.0.1", Port: px.port()}
		if _, err := Dial(ctx, cfg); err == nil || !strings.Contains(err.Error(), "407") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("socks4 cannot take names", func(t *testing.T) {
		px := newTestProxy(t, ProxySOCKS4, "", "")
		cfg := srv.cfg()
		// SOCKS4 resolves locally, so a name works; but the proxy refuses
		// SOCKS4A-style requests.
		cfg.Proxy = &Proxy{Type: ProxySOCKS4A, Host: "127.0.0.1", Port: px.port()}
		cfg.Host = "localhost"
		if _, err := Dial(ctx, cfg); err == nil {
			t.Fatal("SOCKS4A request accepted by a SOCKS4-only proxy")
		}
	})
	t.Run("active mode through tunnel", func(t *testing.T) {
		px := newTestProxy(t, ProxySOCKS5, "", "")
		cfg := srv.cfg()
		cfg.DataMode = Active
		cfg.Proxy = &Proxy{Type: ProxySOCKS5, Host: "127.0.0.1", Port: px.port()}
		c := dialT(t, cfg)
		if _, err := c.List(ctx, ""); !errors.Is(err, ErrActiveViaProxy) {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestFTPProxies(t *testing.T) {
	ctx := context.Background()
	t.Run("user@host", func(t *testing.T) {
		// The test server plays the proxy: it insists on "USER name@host".
		srv := newTestServer(t, func(s *testServer) { s.proxyAtHost = true; s.user, s.pass = "real", "pw" })
		srv.write("x", "1")
		cfg := Config{Host: "ftp.example.invalid", Port: 2121, User: "real", Password: "pw", Timeout: srv.cfg().Timeout,
			Proxy: &Proxy{Type: ProxyFTPUserAtHost, Host: "127.0.0.1", Port: srv.port}}
		c := dialT(t, cfg)
		if srv.sent("USER real@ftp.example.invalid:2121") != 1 {
			t.Fatalf("commands: %v", srv.commands())
		}
		if es, err := c.List(ctx, ""); err != nil || len(es) != 1 {
			t.Fatalf("List = %v, %v", es, err)
		}
	})
	t.Run("open host", func(t *testing.T) {
		srv := newTestServer(t, func(s *testServer) { s.proxyOpen = true })
		cfg := Config{Host: "ftp.example.invalid", Timeout: srv.cfg().Timeout,
			Proxy: &Proxy{Type: ProxyFTPOpen, Host: "127.0.0.1", Port: srv.port}}
		dialT(t, cfg)
		cmds := srv.commands()
		if len(cmds) < 2 || cmds[0] != "OPEN ftp.example.invalid" || !strings.HasPrefix(cmds[1], "USER anonymous") {
			t.Fatalf("commands: %v", cmds)
		}
	})
}

func TestProxyTypeNames(t *testing.T) {
	for i := range proxyNames {
		pt := ProxyType(i)
		if ParseProxyType(pt.String()) != pt {
			t.Errorf("round trip of %v failed", pt)
		}
	}
	if ParseProxyType("bogus") != ProxyNone || ProxyType(99).String() != "none" {
		t.Error("unknown proxy type handling wrong")
	}
	for _, tc := range []struct {
		p    Proxy
		want string
	}{
		{Proxy{Type: ProxySOCKS5, Host: "h"}, "h:1080"},
		{Proxy{Type: ProxyHTTP, Host: "h"}, "h:8080"},
		{Proxy{Type: ProxyFTPSite, Host: "h"}, "h:21"},
		{Proxy{Type: ProxyHTTP, Host: "::1", Port: 3128}, "[::1]:3128"},
	} {
		if got := tc.p.Address(); got != tc.want {
			t.Errorf("Address = %q, want %q", got, tc.want)
		}
	}
}
