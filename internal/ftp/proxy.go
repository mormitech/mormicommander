// This file implements Total Commander's firewall/proxy settings for FTP.
// Two families exist. Tunnelling proxies (SOCKS4, SOCKS4A, SOCKS5 and HTTP
// CONNECT) carry raw TCP, so both the control connection and every passive
// data connection are dialled through them; active mode is impossible there
// because the server cannot connect back through the tunnel. FTP proxies
// (the classic "USER user@host", "SITE host" and "OPEN host" firewalls) are
// themselves FTP servers that relay to the real one; for those the client
// connects to the proxy and only the login sequence changes (see
// Client.authenticate). Everything here uses only the standard library.
package ftp

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"
)

// ProxyType selects the kind of proxy or firewall.
type ProxyType int

const (
	ProxyNone ProxyType = iota
	ProxySOCKS4
	ProxySOCKS4A
	ProxySOCKS5
	ProxyHTTP          // HTTP CONNECT tunnel
	ProxyFTPUserAtHost // USER user@host[:port], PASS pass
	ProxyFTPSite       // [USER/PASS proxy login], SITE host, USER, PASS
	ProxyFTPOpen       // [USER/PASS proxy login], OPEN host, USER, PASS
)

var proxyNames = []string{"none", "socks4", "socks4a", "socks5", "http", "ftp-user@host", "ftp-site", "ftp-open"}

// String returns the settings-file name of the proxy type.
func (t ProxyType) String() string {
	if int(t) < len(proxyNames) && t >= 0 {
		return proxyNames[t]
	}
	return "none"
}

// ParseProxyType is the inverse of ProxyType.String.
func ParseProxyType(s string) ProxyType {
	for i, n := range proxyNames {
		if n == s {
			return ProxyType(i)
		}
	}
	return ProxyNone
}

// Proxy describes one proxy server. In the connection manager the password
// is stored sealed like site passwords.
type Proxy struct {
	Type     ProxyType `json:"type"`
	Host     string    `json:"host"`
	Port     int       `json:"port"`
	User     string    `json:"user,omitempty"`
	Password string    `json:"-"`
}

// Address is "host:port" of the proxy, with each type's usual default port.
func (p *Proxy) Address() string {
	port := p.Port
	if port <= 0 {
		switch p.Type {
		case ProxySOCKS4, ProxySOCKS4A, ProxySOCKS5:
			port = 1080
		case ProxyHTTP:
			port = 8080
		default:
			port = DefaultPort
		}
	}
	return net.JoinHostPort(p.Host, strconv.Itoa(port))
}

func (p *Proxy) isFTPProxy() bool {
	return p.Type == ProxyFTPUserAtHost || p.Type == ProxyFTPSite || p.Type == ProxyFTPOpen
}

// DialContext connects to addr through a tunnelling proxy.
func (p *Proxy) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	conn, err := directDial(ctx, "tcp", p.Address())
	if err != nil {
		return nil, fmt.Errorf("proxy %s: %w", p.Address(), err)
	}
	if dl, ok := ctx.Deadline(); ok {
		conn.SetDeadline(dl)
	} else {
		conn.SetDeadline(time.Now().Add(defaultTimeout))
	}
	stop := context.AfterFunc(ctx, func() { conn.SetDeadline(time.Unix(1, 0)) })
	var out net.Conn
	switch p.Type {
	case ProxySOCKS5:
		err = p.socks5(conn, addr)
		out = conn
	case ProxySOCKS4, ProxySOCKS4A:
		err = p.socks4(ctx, conn, addr)
		out = conn
	case ProxyHTTP:
		out, err = p.httpConnect(conn, addr)
	default:
		err = fmt.Errorf("proxy type %s cannot tunnel", p.Type)
	}
	stop()
	if err != nil {
		conn.Close()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("proxy %s: %w", p.Address(), err)
	}
	conn.SetDeadline(time.Time{})
	return out, nil
}

func splitHostPort(addr string) (string, uint16, error) {
	host, ps, err := net.SplitHostPort(addr)
	if err != nil {
		return "", 0, err
	}
	port, err := strconv.ParseUint(ps, 10, 16)
	if err != nil {
		return "", 0, err
	}
	return host, uint16(port), nil
}

func (p *Proxy) socks5(conn net.Conn, addr string) error {
	host, port, err := splitHostPort(addr)
	if err != nil {
		return err
	}
	methods := []byte{0x00}
	if p.User != "" {
		methods = []byte{0x00, 0x02}
	}
	if _, err := conn.Write(append([]byte{5, byte(len(methods))}, methods...)); err != nil {
		return err
	}
	var sel [2]byte
	if _, err := io.ReadFull(conn, sel[:]); err != nil {
		return err
	}
	if sel[0] != 5 {
		return errors.New("not a SOCKS5 proxy")
	}
	switch sel[1] {
	case 0x00:
	case 0x02:
		if len(p.User) > 255 || len(p.Password) > 255 {
			return errors.New("SOCKS5 user name or password too long")
		}
		req := []byte{1, byte(len(p.User))}
		req = append(req, p.User...)
		req = append(req, byte(len(p.Password)))
		req = append(req, p.Password...)
		if _, err := conn.Write(req); err != nil {
			return err
		}
		var rep [2]byte
		if _, err := io.ReadFull(conn, rep[:]); err != nil {
			return err
		}
		if rep[1] != 0 {
			return errors.New("SOCKS5 authentication failed")
		}
	default:
		return errors.New("SOCKS5 proxy accepts none of our authentication methods")
	}
	req := []byte{5, 1, 0}
	if ip := net.ParseIP(host); ip != nil {
		if v4 := ip.To4(); v4 != nil {
			req = append(req, 1)
			req = append(req, v4...)
		} else {
			req = append(req, 4)
			req = append(req, ip.To16()...)
		}
	} else {
		if len(host) > 255 {
			return errors.New("host name too long")
		}
		req = append(req, 3, byte(len(host)))
		req = append(req, host...)
	}
	req = binary.BigEndian.AppendUint16(req, port)
	if _, err := conn.Write(req); err != nil {
		return err
	}
	var hdr [4]byte
	if _, err := io.ReadFull(conn, hdr[:]); err != nil {
		return err
	}
	if hdr[1] != 0 {
		return fmt.Errorf("SOCKS5 connect failed (code %d)", hdr[1])
	}
	var skip int
	switch hdr[3] {
	case 1:
		skip = 4
	case 4:
		skip = 16
	case 3:
		var l [1]byte
		if _, err := io.ReadFull(conn, l[:]); err != nil {
			return err
		}
		skip = int(l[0])
	default:
		return errors.New("SOCKS5: bad reply address type")
	}
	_, err = io.ReadFull(conn, make([]byte, skip+2))
	return err
}

func (p *Proxy) socks4(ctx context.Context, conn net.Conn, addr string) error {
	host, port, err := splitHostPort(addr)
	if err != nil {
		return err
	}
	req := []byte{4, 1}
	req = binary.BigEndian.AppendUint16(req, port)
	var v4 net.IP
	if ip := net.ParseIP(host); ip != nil {
		v4 = ip.To4()
		if v4 == nil {
			return errors.New("SOCKS4 cannot reach IPv6 addresses")
		}
	} else if p.Type == ProxySOCKS4 {
		ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", host)
		if err != nil || len(ips) == 0 {
			return fmt.Errorf("cannot resolve %s for SOCKS4", host)
		}
		v4 = ips[0].To4()
	}
	if v4 == nil { // SOCKS4A: let the proxy resolve the name
		req = append(req, 0, 0, 0, 1)
		req = append(req, p.User...)
		req = append(req, 0)
		req = append(req, host...)
		req = append(req, 0)
	} else {
		req = append(req, v4...)
		req = append(req, p.User...)
		req = append(req, 0)
	}
	if _, err := conn.Write(req); err != nil {
		return err
	}
	var rep [8]byte
	if _, err := io.ReadFull(conn, rep[:]); err != nil {
		return err
	}
	if rep[1] != 0x5a {
		return fmt.Errorf("SOCKS4 connect refused (code %d)", rep[1])
	}
	return nil
}

// bufferedConn returns bytes the HTTP response reader already buffered
// (an FTP greeting may arrive right behind the proxy's "200") before
// reading from the connection again.
type bufferedConn struct {
	net.Conn
	r *bufio.Reader
}

func (b *bufferedConn) Read(p []byte) (int, error) { return b.r.Read(p) }

func (p *Proxy) httpConnect(conn net.Conn, addr string) (net.Conn, error) {
	req := "CONNECT " + addr + " HTTP/1.1\r\nHost: " + addr + "\r\n"
	if p.User != "" {
		cred := base64.StdEncoding.EncodeToString([]byte(p.User + ":" + p.Password))
		req += "Proxy-Authorization: Basic " + cred + "\r\n"
	}
	req += "\r\n"
	if _, err := io.WriteString(conn, req); err != nil {
		return nil, err
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodConnect})
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP proxy refused CONNECT: %s", resp.Status)
	}
	return &bufferedConn{Conn: conn, r: br}, nil
}
