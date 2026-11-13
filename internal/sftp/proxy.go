// This file implements the proxy tunnels Total Commander offers in its
// firewall settings: SOCKS4a, SOCKS5 (optionally with user name and
// password) and HTTP CONNECT (optionally with basic authentication). In
// corporate networks the SSH port is often reachable only through such a
// proxy. All three are small handshakes done before the SSH protocol starts,
// so they are written here with the standard library instead of pulling in a
// proxy package; after the handshake the returned connection is a plain byte
// pipe to the target host.
package sftp

import (
	"bufio"
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

// ProxyType selects the proxy protocol.
type ProxyType int

const (
	ProxySOCKS5 ProxyType = iota
	ProxySOCKS4
	ProxyHTTP
)

func (t ProxyType) String() string {
	switch t {
	case ProxySOCKS5:
		return "socks5"
	case ProxySOCKS4:
		return "socks4"
	case ProxyHTTP:
		return "http"
	}
	return "unknown"
}

// Proxy describes a proxy server to tunnel through.
type Proxy struct {
	Type     ProxyType
	Addr     string // host:port of the proxy
	User     string // optional
	Password string // optional (SOCKS5 and HTTP only)
}

// Dial connects to the proxy and asks it to open a tunnel to addr. The
// timeout covers both the TCP connect and the proxy handshake.
func (p *Proxy) Dial(addr string, timeout time.Duration) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", p.Addr, timeout)
	if err != nil {
		return nil, fmt.Errorf("sftp: connecting to proxy %s: %w", p.Addr, err)
	}
	if timeout > 0 {
		conn.SetDeadline(time.Now().Add(timeout))
	}
	switch p.Type {
	case ProxySOCKS5:
		err = p.socks5(conn, addr)
	case ProxySOCKS4:
		err = p.socks4(conn, addr)
	case ProxyHTTP:
		conn, err = p.httpConnect(conn, addr)
	default:
		err = fmt.Errorf("unknown proxy type %d", p.Type)
	}
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("sftp: proxy %s: %w", p.Addr, err)
	}
	conn.SetDeadline(time.Time{})
	return conn, nil
}

func splitHostPort(addr string) (string, uint16, error) {
	host, ps, err := net.SplitHostPort(addr)
	if err != nil {
		return "", 0, err
	}
	port, err := strconv.ParseUint(ps, 10, 16)
	if err != nil {
		return "", 0, fmt.Errorf("bad port %q", ps)
	}
	return host, uint16(port), nil
}

// socks5 performs the RFC 1928 handshake, with RFC 1929 user/password
// authentication when credentials are configured. The host name is sent to
// the proxy unresolved so that DNS happens on the proxy's side of the
// firewall.
func (p *Proxy) socks5(conn net.Conn, addr string) error {
	host, port, err := splitHostPort(addr)
	if err != nil {
		return err
	}
	methods := []byte{0x00}
	if p.User != "" {
		methods = []byte{0x02, 0x00}
	}
	if _, err := conn.Write(append([]byte{0x05, byte(len(methods))}, methods...)); err != nil {
		return err
	}
	var resp [2]byte
	if _, err := io.ReadFull(conn, resp[:]); err != nil {
		return err
	}
	if resp[0] != 0x05 {
		return errors.New("not a SOCKS5 proxy")
	}
	switch resp[1] {
	case 0x00:
	case 0x02:
		if len(p.User) > 255 || len(p.Password) > 255 {
			return errors.New("SOCKS5 user name or password too long")
		}
		msg := []byte{0x01, byte(len(p.User))}
		msg = append(msg, p.User...)
		msg = append(msg, byte(len(p.Password)))
		msg = append(msg, p.Password...)
		if _, err := conn.Write(msg); err != nil {
			return err
		}
		if _, err := io.ReadFull(conn, resp[:]); err != nil {
			return err
		}
		if resp[1] != 0x00 {
			return errors.New("SOCKS5 authentication failed")
		}
	default:
		return errors.New("SOCKS5 proxy accepts none of our authentication methods")
	}

	req := []byte{0x05, 0x01, 0x00}
	if ip := net.ParseIP(host); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			req = append(req, 0x01)
			req = append(req, ip4...)
		} else {
			req = append(req, 0x04)
			req = append(req, ip.To16()...)
		}
	} else {
		if len(host) > 255 {
			return errors.New("host name too long")
		}
		req = append(req, 0x03, byte(len(host)))
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
	if hdr[1] != 0x00 {
		return fmt.Errorf("SOCKS5 connect failed (code %d)", hdr[1])
	}
	// Skip the bound address in the reply.
	var skip int
	switch hdr[3] {
	case 0x01:
		skip = 4
	case 0x04:
		skip = 16
	case 0x03:
		var l [1]byte
		if _, err := io.ReadFull(conn, l[:]); err != nil {
			return err
		}
		skip = int(l[0])
	default:
		return errors.New("SOCKS5 reply has unknown address type")
	}
	_, err = io.ReadFull(conn, make([]byte, skip+2))
	return err
}

// socks4 performs a SOCKS4a CONNECT, which lets the proxy resolve the host
// name. SOCKS4 has no passwords; User is sent as the user id.
func (p *Proxy) socks4(conn net.Conn, addr string) error {
	host, port, err := splitHostPort(addr)
	if err != nil {
		return err
	}
	req := []byte{0x04, 0x01}
	req = binary.BigEndian.AppendUint16(req, port)
	ip := net.ParseIP(host).To4()
	if ip != nil {
		req = append(req, ip...)
	} else {
		req = append(req, 0, 0, 0, 1) // SOCKS4a marker
	}
	req = append(req, p.User...)
	req = append(req, 0)
	if ip == nil {
		req = append(req, host...)
		req = append(req, 0)
	}
	if _, err := conn.Write(req); err != nil {
		return err
	}
	var resp [8]byte
	if _, err := io.ReadFull(conn, resp[:]); err != nil {
		return err
	}
	if resp[1] != 0x5a {
		return fmt.Errorf("SOCKS4 connect rejected (code %d)", resp[1])
	}
	return nil
}

// httpConnect asks an HTTP proxy for a CONNECT tunnel. Any bytes the proxy
// sent after its response headers are preserved by returning a connection
// that reads through the buffered reader first.
func (p *Proxy) httpConnect(conn net.Conn, addr string) (net.Conn, error) {
	req := "CONNECT " + addr + " HTTP/1.1\r\nHost: " + addr + "\r\n"
	if p.User != "" {
		cred := base64.StdEncoding.EncodeToString([]byte(p.User + ":" + p.Password))
		req += "Proxy-Authorization: Basic " + cred + "\r\n"
	}
	req += "\r\n"
	if _, err := io.WriteString(conn, req); err != nil {
		return conn, err
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodConnect})
	if err != nil {
		return conn, err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return conn, fmt.Errorf("HTTP CONNECT failed: %s", resp.Status)
	}
	if br.Buffered() == 0 {
		return conn, nil
	}
	return &bufferedConn{Conn: conn, r: br}, nil
}

type bufferedConn struct {
	net.Conn
	r *bufio.Reader
}

func (b *bufferedConn) Read(p []byte) (int, error) { return b.r.Read(p) }
