// This file opens FTP data connections, the second TCP connection every
// listing and file transfer travels over. In passive mode (the default) the
// client asks with EPSV, or PASV for old servers, where to connect; because
// servers behind NAT routers often announce an address nobody can reach, the
// announced host can be replaced with the control connection's host, as
// FileZilla and Total Commander do. In active mode the client listens and
// tells the server where with PORT (IPv4) or EPRT (IPv6), optionally inside
// a fixed port range for firewalls. With FTPS the data connection is wrapped
// in TLS too, reusing the control connection's session cache, and the
// client always plays the TLS client role as RFC 4217 requires.
package ftp

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type dialFunc func(ctx context.Context, network, addr string) (net.Conn, error)

func directDial(ctx context.Context, network, addr string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, network, addr)
}

// idleConn refreshes the deadline before every read and write, so a
// transfer may take hours but a stalled one fails after Config.Timeout.
type idleConn struct {
	net.Conn
	timeout time.Duration
}

func (c *idleConn) Read(p []byte) (int, error) {
	c.Conn.SetDeadline(time.Now().Add(c.timeout))
	return c.Conn.Read(p)
}

func (c *idleConn) Write(p []byte) (int, error) {
	c.Conn.SetDeadline(time.Now().Add(c.timeout))
	return c.Conn.Write(p)
}

// openData prepares a data connection, sends cmd (preceded by REST when
// offset > 0) and returns the connected, possibly TLS-wrapped, data stream.
// A nil connection with a nil error means the server finished at once with
// a 2xx reply and sent no data (some servers do that for empty folders).
func (c *Client) openData(ctx context.Context, cmd string, offset int64) (net.Conn, error) {
	var conn net.Conn
	var ln net.Listener
	var err error
	if c.cfg.DataMode == Active {
		ln, err = c.listenActive()
		if err != nil {
			return nil, err
		}
		defer ln.Close()
	} else {
		addr, err := c.passiveAddr()
		if err != nil {
			return nil, err
		}
		dctx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
		conn, err = c.dial(dctx, "tcp", addr)
		cancel()
		if err != nil {
			return nil, fmt.Errorf("ftp: data connection to %s: %w", addr, err)
		}
	}
	fail := func(err error) (net.Conn, error) {
		if conn != nil {
			conn.Close()
		}
		return nil, err
	}
	if offset > 0 {
		r, err := c.raw("REST " + strconv.FormatInt(offset, 10))
		if err != nil {
			return fail(err)
		}
		if r.Code != 350 {
			return fail(protoErr("REST", r))
		}
	}
	r, err := c.raw(cmd)
	if err != nil {
		return fail(err)
	}
	if r.Code/100 != 1 {
		if r.Code/100 == 2 {
			return fail(nil)
		}
		return fail(protoErr(cmd, r))
	}
	if ln != nil {
		var want net.IP
		if ra, ok := c.conn.RemoteAddr().(*net.TCPAddr); ok {
			want = ra.IP
		}
		conn, err = acceptActive(ctx, ln, want, c.cfg.Timeout, activeGrace)
		if err != nil {
			ln.Close()
			c.readReply() // the server's 425 for the failed transfer
			return fail(err)
		}
	}
	if c.prot {
		tc := tls.Client(conn, c.tlsConf)
		hctx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
		err := tc.HandshakeContext(hctx)
		cancel()
		if err != nil {
			conn.Close()
			c.readReply()
			return nil, fmt.Errorf("ftp: data connection TLS handshake: %w", err)
		}
		conn = tc
	}
	return &idleConn{Conn: conn, timeout: c.cfg.Timeout}, nil
}

// closeUpload ends the data connection of an upload so that all of it
// arrives. Closing a socket that still holds unread data (a TLS 1.3 server
// sends session tickets on the data connection, which an upload never reads)
// makes the system reset the connection instead of closing it, and the
// server then loses whatever it had not read yet: the end of the file. So
// the client says it is done (TLS close_notify, then a TCP half-close),
// reads until the server closes its side (at most timeout), and only then
// closes.
func closeUpload(conn net.Conn, timeout time.Duration) error {
	raw := conn
	if ic, ok := raw.(*idleConn); ok {
		raw = ic.Conn
	}
	var err error
	if tc, ok := raw.(*tls.Conn); ok {
		err = tc.CloseWrite()
		raw = tc.NetConn()
	}
	// An HTTP CONNECT tunnel's connection is wrapped (see bufferedConn), and the wrapper does not offer the
	// half-close; it has to be asked of the TCP connection inside, or the upload ends with a reset after all.
	if bc, ok := raw.(*bufferedConn); ok {
		raw = bc.Conn
	}
	if hc, ok := raw.(interface{ CloseWrite() error }); ok && err == nil {
		if err = hc.CloseWrite(); err == nil {
			if timeout <= 0 || timeout > 30*time.Second {
				timeout = 30 * time.Second
			}
			deadline := time.Now().Add(timeout)
			buf := make([]byte, 4096)
			for {
				conn.SetReadDeadline(deadline)
				if _, rerr := conn.Read(buf); rerr != nil {
					break // the server closed (or went quiet): what it sends here is never needed
				}
			}
		}
	}
	if cerr := conn.Close(); err == nil {
		err = cerr
	}
	return err
}

// finishData reads the final reply of a transfer after the data connection
// was closed.
func (c *Client) finishData(cmd string) error {
	c.armDeadline()
	r, err := c.readReply()
	if err != nil {
		return err
	}
	if r.Code/100 != 2 {
		return protoErr(cmd, r)
	}
	return nil
}

var (
	pasvRe = regexp.MustCompile(`(\d{1,3}),(\d{1,3}),(\d{1,3}),(\d{1,3}),(\d{1,3}),(\d{1,3})`)
	epsvRe = regexp.MustCompile(`\((.)(.?)(.?)(\d+)(.)\)`)
)

// passiveAddr asks for a passive port (EPSV first, PASV as fallback) and
// returns the address to dial.
func (c *Client) passiveAddr() (string, error) {
	if !c.cfg.DisableEPSV && !c.noEPSV {
		r, err := c.raw("EPSV")
		if err != nil {
			return "", err
		}
		if r.Code == 229 {
			if m := epsvRe.FindStringSubmatch(r.Msg); m != nil {
				port, _ := strconv.Atoi(m[4])
				if port > 0 && port < 65536 {
					return net.JoinHostPort(c.controlHost(), strconv.Itoa(port)), nil
				}
			}
		}
		c.noEPSV = true
	}
	r, err := c.raw("PASV")
	if err != nil {
		return "", err
	}
	if r.Code != 227 {
		return "", protoErr("PASV", r)
	}
	host, port, err := parsePASV(r.Msg)
	if err != nil {
		return "", err
	}
	if c.replacePassiveHost(net.ParseIP(host)) {
		host = c.controlHost()
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), nil
}

func parsePASV(msg string) (string, int, error) {
	m := pasvRe.FindStringSubmatch(msg)
	if m == nil {
		return "", 0, fmt.Errorf("ftp: cannot parse PASV reply %q", msg)
	}
	var n [6]int
	for i := range n {
		n[i], _ = strconv.Atoi(m[i+1])
		if n[i] > 255 {
			return "", 0, fmt.Errorf("ftp: cannot parse PASV reply %q", msg)
		}
	}
	host := fmt.Sprintf("%d.%d.%d.%d", n[0], n[1], n[2], n[3])
	return host, n[4]<<8 | n[5], nil
}

func (c *Client) replacePassiveHost(ip net.IP) bool {
	switch c.cfg.PassiveHost {
	case PassiveHostServer:
		return false
	case PassiveHostControl:
		return true
	}
	if ip == nil || ip.IsUnspecified() {
		return true
	}
	ctrl := net.ParseIP(c.controlHost())
	// A loopback address is only right when the server runs on this computer too. From a server on the LAN it would
	// make the client connect to one of its own local services (an upload sent there, a "listing" read from it).
	if ip.IsLoopback() && ctrl != nil && !ctrl.IsLoopback() {
		return true
	}
	return nonRoutable(ip) && ctrl != nil && !nonRoutable(ctrl)
}

func nonRoutable(ip net.IP) bool {
	return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast()
}

// controlHost is the host data connections go to when the PASV address is
// not trusted: the control connection's peer, or the configured host name
// when a SOCKS/HTTP proxy hides the real peer.
func (c *Client) controlHost() string {
	if p := c.cfg.Proxy; p != nil && p.Type != ProxyNone && !p.isFTPProxy() {
		return c.cfg.Host
	}
	if a, ok := c.conn.RemoteAddr().(*net.TCPAddr); ok {
		return a.IP.String()
	}
	host, _, err := net.SplitHostPort(c.conn.RemoteAddr().String())
	if err != nil {
		return c.cfg.Host
	}
	return host
}

// listenActive opens a listener for active mode and announces it.
func (c *Client) listenActive() (net.Listener, error) {
	if p := c.cfg.Proxy; p != nil && p.Type != ProxyNone && !p.isFTPProxy() {
		return nil, ErrActiveViaProxy
	}
	local, ok := c.conn.LocalAddr().(*net.TCPAddr)
	if !ok {
		return nil, errors.New("ftp: active mode needs a TCP control connection")
	}
	ln, err := listenInRange(local.IP, c.cfg.ActivePortMin, c.cfg.ActivePortMax)
	if err != nil {
		return nil, fmt.Errorf("ftp: active mode listener: %w", err)
	}
	adv := local.IP
	if c.cfg.ActiveIP != "" {
		ip := net.ParseIP(c.cfg.ActiveIP)
		if ip == nil {
			ips, err := net.LookupIP(c.cfg.ActiveIP)
			if err != nil || len(ips) == 0 {
				ln.Close()
				return nil, fmt.Errorf("ftp: cannot resolve active mode address %q", c.cfg.ActiveIP)
			}
			ip = ips[0]
		}
		adv = ip
	}
	port := ln.Addr().(*net.TCPAddr).Port
	var cmd string
	if v4 := adv.To4(); v4 != nil {
		cmd = fmt.Sprintf("PORT %d,%d,%d,%d,%d,%d", v4[0], v4[1], v4[2], v4[3], port>>8, port&0xff)
	} else {
		cmd = fmt.Sprintf("EPRT |2|%s|%d|", adv.String(), port)
	}
	if err := c.expect2(cmd); err != nil {
		ln.Close()
		return nil, err
	}
	return ln, nil
}

func listenInRange(ip net.IP, lo, hi int) (net.Listener, error) {
	if lo <= 0 || hi < lo || hi > 65535 {
		return net.Listen("tcp", net.JoinHostPort(ip.String(), "0"))
	}
	ports := make([]int, 0, hi-lo+1)
	for p := lo; p <= hi; p++ {
		ports = append(ports, p)
	}
	rand.Shuffle(len(ports), func(i, j int) { ports[i], ports[j] = ports[j], ports[i] })
	var last error
	for _, p := range ports {
		ln, err := net.Listen("tcp", net.JoinHostPort(ip.String(), strconv.Itoa(p)))
		if err == nil {
			return ln, nil
		}
		last = err
	}
	return nil, fmt.Errorf("no free port in %d-%d: %w", lo, hi, last)
}

// readDataLines runs a listing command and returns its lines.
func (c *Client) readDataLines(ctx context.Context, cmd string) ([]string, error) {
	conn, err := c.openData(ctx, cmd, 0)
	if err != nil {
		return nil, err
	}
	if conn == nil {
		return nil, nil
	}
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	var b strings.Builder
	buf := make([]byte, 32*1024)
	var rerr error
	for {
		n, err := conn.Read(buf)
		b.Write(buf[:n])
		if err != nil {
			if !isEOF(err) {
				rerr = err
			}
			break
		}
	}
	stop()
	conn.Close()
	if rerr != nil {
		c.readReply()
		return nil, rerr
	}
	if err := c.finishData(cmd); err != nil {
		return nil, err
	}
	var lines []string
	for _, l := range strings.Split(b.String(), "\n") {
		l = strings.TrimRight(l, "\r")
		if strings.TrimSpace(l) != "" {
			lines = append(lines, c.dec(l))
		}
	}
	return lines, nil
}

// activeGrace is how long an active-mode listener keeps waiting for the server's own address after a connection
// from another address arrived.
const activeGrace = 2 * time.Second

// acceptActive takes the server's data connection in active mode. Someone else on the network could connect to the
// announced port first and take the transfer (port stealing), so a connection from the control connection's address
// (want) is preferred: one from another address is held back and used only when nothing from want arrives within
// grace, which keeps servers working that connect back from another address (several addresses, NAT). FTPS checks the
// peer through TLS as well.
func acceptActive(ctx context.Context, ln net.Listener, want net.IP, timeout, grace time.Duration) (net.Conn, error) {
	type result struct {
		conn net.Conn
		err  error
	}
	ch := make(chan result, 4)
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for {
			cn, err := ln.Accept()
			select {
			case ch <- result{cn, err}:
			case <-stop:
				if cn != nil {
					cn.Close()
				}
				return
			}
			if err != nil {
				return
			}
		}
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	var held net.Conn
	var graceC <-chan time.Time
	release := func() {
		if held != nil {
			held.Close()
		}
	}
	for {
		select {
		case res := <-ch:
			if res.err != nil {
				if held != nil {
					return held, nil
				}
				return nil, res.err
			}
			ip := net.IP(nil)
			if ra, ok := res.conn.RemoteAddr().(*net.TCPAddr); ok {
				ip = ra.IP
			}
			if want == nil || ip.Equal(want) {
				release()
				return res.conn, nil
			}
			if held != nil {
				res.conn.Close() // only the first stranger is held back
				continue
			}
			held = res.conn
			g := time.NewTimer(grace)
			defer g.Stop()
			graceC = g.C
		case <-graceC:
			return held, nil
		case <-timer.C:
			if held != nil {
				return held, nil
			}
			return nil, errors.New("ftp: server did not connect back (active mode)")
		case <-ctx.Done():
			release()
			return nil, ctx.Err()
		}
	}
}
