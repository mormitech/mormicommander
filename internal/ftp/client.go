// This file is the heart of the FTP client: opening the control connection,
// the login dance (greeting, optional AUTH TLS, USER/PASS/ACCT, PBSZ/PROT,
// FEAT, OPTS UTF8, SYST), and the simple one-line commands a file panel
// needs (PWD, CWD, CDUP, MKD, RMD, DELE, RNFR/RNTO, SITE CHMOD, SIZE, MDTM,
// MFMT, NOOP and raw "quote" commands). FTP is strictly one command at a
// time on the control connection, so every public method takes the client's
// mutex; a download or upload keeps holding it until its stream is closed,
// which is why parallel transfers use several clients (see Pool). Each
// method also takes a context so the job engine can cancel a slow server.
package ftp

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/textproto"
	"path"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Reply is one complete server reply. Multi-line replies have their lines
// joined with "\n".
type Reply struct {
	Code int
	Msg  string
}

// Client is one logged-in FTP connection.
type Client struct {
	cfg  Config
	dial dialFunc

	mu       sync.Mutex // serialises use of the control connection
	conn     net.Conn   // control connection (a *tls.Conn once secured)
	tp       *textproto.Conn
	tlsConf  *tls.Config // non-nil once the control connection uses TLS
	prot     bool        // data connections are protected (PROT P)
	features map[string]string
	welcome  string
	system   string
	latin1   bool
	curType  byte // last TYPE sent: 'A', 'I' or 0
	noEPSV   bool // EPSV failed once; use PASV from now on
	noMLSD   bool // MLSD failed once; use LIST from now on
	broken   error

	lastUse   atomic.Int64 // unix nanos of the last command
	kaMu      sync.Mutex
	kaStop    chan struct{}
	closeOnce sync.Once
}

// Dial connects to the server described by cfg and logs in.
func Dial(ctx context.Context, cfg Config) (*Client, error) {
	cfg = cfg.withDefaults()
	if strings.TrimSpace(cfg.Host) == "" {
		return nil, errors.New("ftp: no host given")
	}
	c := &Client{cfg: cfg, features: map[string]string{}}
	c.dial = directDial
	target := cfg.Address()
	if p := cfg.Proxy; p != nil && p.Type != ProxyNone {
		if p.isFTPProxy() {
			target = p.Address()
		} else {
			c.dial = p.DialContext
		}
	}
	dctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	conn, err := c.dial(dctx, "tcp", target)
	cancel()
	if err != nil {
		return nil, fmt.Errorf("ftp: connect to %s: %w", target, err)
	}
	if cfg.Security == SecurityImplicitTLS {
		conf := c.newTLSConfig()
		tc := tls.Client(conn, conf)
		hctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
		err := tc.HandshakeContext(hctx)
		cancel()
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("ftp: TLS handshake: %w", err)
		}
		conn = tc
		c.tlsConf = conf
	}
	c.setConn(conn)
	stop := context.AfterFunc(ctx, func() { conn.SetDeadline(time.Unix(1, 0)) })
	err = c.login(ctx)
	stop()
	if err != nil {
		c.conn.Close()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	c.conn.SetDeadline(time.Time{})
	c.touch()
	if cfg.Keepalive > 0 {
		c.StartKeepalive(cfg.Keepalive)
	}
	return c, nil
}

func (c *Client) setConn(conn net.Conn) {
	c.conn = conn
	c.tp = textproto.NewConn(conn)
}

func (c *Client) newTLSConfig() *tls.Config {
	var conf *tls.Config
	if c.cfg.TLSConfig != nil {
		conf = c.cfg.TLSConfig.Clone()
	} else {
		conf = &tls.Config{}
	}
	if conf.ServerName == "" {
		conf.ServerName = c.cfg.Host
	}
	if conf.ClientSessionCache == nil {
		// Many servers insist that the data connection resumes the control
		// connection's TLS session; sharing a cache makes that happen.
		conf.ClientSessionCache = tls.NewLRUClientSessionCache(16)
	}
	return conf
}

func (c *Client) logf(format string, args ...any) {
	if c.cfg.Logf != nil {
		c.cfg.Logf(format, args...)
	}
}

func (c *Client) touch() { c.lastUse.Store(time.Now().UnixNano()) }

// IdleFor tells how long the connection has been unused, so the UI can
// disconnect idle sessions (Total Commander's idle timeout).
func (c *Client) IdleFor() time.Duration {
	return time.Since(time.Unix(0, c.lastUse.Load()))
}

// login runs the greeting, TLS upgrade, authentication and feature
// negotiation. The caller holds no lock; nobody else knows the client yet.
func (c *Client) login(ctx context.Context) error {
	r, err := c.readReply()
	for err == nil && r.Code == 120 { // "service ready in nnn minutes"
		r, err = c.readReply()
	}
	if err != nil {
		return err
	}
	if r.Code != 220 {
		return protoErr("connect", r)
	}
	c.welcome = r.Msg

	if s := c.cfg.Security; s == SecurityExplicitTLS || s == SecurityExplicitTLSIfAvailable {
		r, err := c.raw("AUTH TLS")
		if err != nil {
			return err
		}
		if r.Code != 234 {
			if r, err = c.raw("AUTH SSL"); err != nil {
				return err
			}
		}
		switch {
		case r.Code == 234 || r.Code == 334:
			conf := c.newTLSConfig()
			tc := tls.Client(c.conn, conf)
			hctx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
			err := tc.HandshakeContext(hctx)
			cancel()
			if err != nil {
				return fmt.Errorf("ftp: TLS handshake: %w", err)
			}
			c.setConn(tc)
			c.tlsConf = conf
		case s == SecurityExplicitTLS:
			return fmt.Errorf("%w: %d %s", ErrTLSRefused, r.Code, r.Msg)
		}
	}

	if err := c.authenticate(); err != nil {
		return err
	}

	if c.tlsConf != nil {
		if _, err := c.raw("PBSZ 0"); err != nil {
			return err
		}
		r, err := c.raw("PROT P")
		if err != nil {
			return err
		}
		if r.Code/100 != 2 {
			return protoErr("PROT P", r)
		}
		c.prot = true
	}

	if r, err := c.raw("FEAT"); err != nil {
		return err
	} else if r.Code/100 == 2 {
		c.features = parseFeatures(r.Msg)
	}
	switch c.cfg.Encoding {
	case EncodingLatin1:
		c.latin1 = true
	default:
		if c.hasFeature("UTF8") {
			if _, err := c.raw("OPTS UTF8 ON"); err != nil {
				return err
			}
		}
	}
	if r, err := c.raw("SYST"); err != nil {
		return err
	} else if r.Code/100 == 2 {
		c.system = r.Msg
	}
	if d := c.cfg.InitialDir; d != "" {
		if err := c.expect2("CWD " + c.enc(d)); err != nil {
			return err
		}
	}
	return nil
}

// authenticate sends USER/PASS (and ACCT), adapted for FTP-style proxies.
func (c *Client) authenticate() error {
	user, pass := c.cfg.loginUser()
	if p := c.cfg.Proxy; p != nil {
		hostPort := c.cfg.Host
		if c.cfg.EffectivePort() != DefaultPort {
			hostPort = c.cfg.Address()
		}
		switch p.Type {
		case ProxyFTPUserAtHost:
			if p.User != "" {
				if err := c.userPass(p.User, p.Password); err != nil {
					return err
				}
			}
			user = user + "@" + hostPort
		case ProxyFTPSite, ProxyFTPOpen:
			if p.User != "" {
				if err := c.userPass(p.User, p.Password); err != nil {
					return err
				}
			}
			verb := "SITE "
			if p.Type == ProxyFTPOpen {
				verb = "OPEN "
			}
			r, err := c.raw(verb + hostPort)
			if err != nil {
				return err
			}
			if r.Code/100 != 2 {
				return protoErr(verb+hostPort, r)
			}
		}
	}
	return c.userPass(user, pass)
}

func (c *Client) userPass(user, pass string) error {
	r, err := c.raw("USER " + c.enc(user))
	if err != nil {
		return err
	}
	if r.Code == 331 {
		if r, err = c.raw("PASS " + pass); err != nil {
			return err
		}
	}
	if r.Code == 332 {
		if r, err = c.raw("ACCT " + c.cfg.Account); err != nil {
			return err
		}
	}
	if r.Code/100 != 2 {
		return fmt.Errorf("%w: %w", ErrLoginFailed, protoErr("USER "+user, r))
	}
	return nil
}

func parseFeatures(msg string) map[string]string {
	feats := map[string]string{}
	lines := strings.Split(msg, "\n")
	for i, l := range lines {
		if i == 0 || i == len(lines)-1 && !strings.HasPrefix(l, " ") {
			continue // "Features:" header and "End" trailer
		}
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		name, val, _ := strings.Cut(l, " ")
		feats[strings.ToUpper(name)] = strings.TrimSpace(val)
	}
	return feats
}

func (c *Client) hasFeature(name string) bool {
	_, ok := c.features[strings.ToUpper(name)]
	return ok
}

// readReply reads one complete (possibly multi-line) reply.
func (c *Client) readReply() (Reply, error) {
	code, msg, err := c.tp.ReadResponse(0)
	if err != nil {
		var te *textproto.Error
		if !errors.As(err, &te) {
			c.broken = fmt.Errorf("%w: %w", ErrClosed, err)
			return Reply{}, c.broken
		}
	}
	r := Reply{Code: code, Msg: c.dec(msg)}
	c.logf("< %d %s", r.Code, r.Msg)
	return r, nil
}

// raw sends one command line and reads its reply, with a fresh per-reply
// deadline. The caller holds c.mu (or owns the client exclusively).
func (c *Client) raw(line string) (Reply, error) {
	if c.broken != nil {
		return Reply{}, c.broken
	}
	if strings.ContainsAny(line, "\r\n") {
		return Reply{}, ErrBadLine
	}
	c.armDeadline()
	c.logf("> %s", maskCmd(line))
	if err := c.tp.PrintfLine("%s", line); err != nil {
		c.broken = fmt.Errorf("%w: %w", ErrClosed, err)
		return Reply{}, c.broken
	}
	return c.readReply()
}

// armDeadline gives the next exchange Config.Timeout to complete. A context
// cancellation (see begin) overrides it with a deadline in the past.
func (c *Client) armDeadline() {
	c.conn.SetDeadline(time.Now().Add(c.cfg.Timeout))
}

func (c *Client) expect2(line string) error {
	r, err := c.raw(line)
	if err != nil {
		return err
	}
	if r.Code/100 != 2 {
		return protoErr(line, r)
	}
	return nil
}

// begin takes the control lock for one public operation and ties the
// context to the connection: cancelling it makes the pending read fail.
func (c *Client) begin(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.mu.Lock()
	if c.broken != nil {
		err := c.broken
		c.mu.Unlock()
		return nil, err
	}
	conn := c.conn
	stop := context.AfterFunc(ctx, func() { conn.SetDeadline(time.Unix(1, 0)) })
	return func() {
		// A read interrupted by the cancellation already marked the
		// connection broken in readReply; otherwise it is still in sync.
		stop()
		c.touch()
		c.mu.Unlock()
	}, nil
}

// ctxErr prefers the context's error over the I/O error it caused.
func ctxErr(ctx context.Context, err error) error {
	if err != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

// Features returns the server's FEAT list (upper-case names to arguments).
func (c *Client) Features() map[string]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	m := make(map[string]string, len(c.features))
	for k, v := range c.features {
		m[k] = v
	}
	return m
}

// HasFeature reports whether FEAT listed name (e.g. "MLST", "UTF8").
func (c *Client) HasFeature(name string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hasFeature(name)
}

// Welcome is the server's greeting banner.
func (c *Client) Welcome() string { return c.welcome }

// System is the SYST reply, e.g. "UNIX Type: L8".
func (c *Client) System() string { return c.system }

// Secure reports whether the control connection uses TLS.
func (c *Client) Secure() bool { return c.tlsConf != nil }

// Config returns the configuration the client was dialled with.
func (c *Client) Config() Config { return c.cfg }

// Broken returns the error that made the connection unusable, or nil.
func (c *Client) Broken() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.broken
}

// Quote sends a raw command and returns the reply without judging the code
// (Total Commander's "Send command" for SITE and friends).
func (c *Client) Quote(ctx context.Context, line string) (Reply, error) {
	done, err := c.begin(ctx)
	if err != nil {
		return Reply{}, err
	}
	defer done()
	r, err := c.raw(c.enc(line))
	return r, ctxErr(ctx, err)
}

// NoOp sends NOOP, which keeps the connection alive and checks it works.
func (c *Client) NoOp(ctx context.Context) error {
	return c.simple(ctx, "NOOP")
}

func (c *Client) simple(ctx context.Context, line string) error {
	done, err := c.begin(ctx)
	if err != nil {
		return err
	}
	defer done()
	return ctxErr(ctx, c.expect2(line))
}

// Pwd returns the current remote folder.
func (c *Client) Pwd(ctx context.Context) (string, error) {
	done, err := c.begin(ctx)
	if err != nil {
		return "", err
	}
	defer done()
	p, err := c.pwd()
	return p, ctxErr(ctx, err)
}

func (c *Client) pwd() (string, error) {
	r, err := c.raw("PWD")
	if err != nil {
		return "", err
	}
	if r.Code != 257 {
		return "", protoErr("PWD", r)
	}
	return parseQuoted(r.Msg), nil
}

// parseQuoted extracts the path from a 257 reply: `"/a ""b""" is cwd`.
func parseQuoted(msg string) string {
	i := strings.IndexByte(msg, '"')
	if i < 0 {
		return strings.TrimSpace(strings.SplitN(msg, " ", 2)[0])
	}
	var b strings.Builder
	for j := i + 1; j < len(msg); j++ {
		if msg[j] == '"' {
			if j+1 < len(msg) && msg[j+1] == '"' {
				b.WriteByte('"')
				j++
				continue
			}
			break
		}
		b.WriteByte(msg[j])
	}
	return b.String()
}

// Cd changes the remote folder.
func (c *Client) Cd(ctx context.Context, dir string) error {
	return c.simple(ctx, "CWD "+c.enc(dir))
}

// CdUp goes to the parent folder (CDUP, falling back to "CWD ..").
func (c *Client) CdUp(ctx context.Context) error {
	done, err := c.begin(ctx)
	if err != nil {
		return err
	}
	defer done()
	r, err := c.raw("CDUP")
	if err != nil {
		return ctxErr(ctx, err)
	}
	if r.Code/100 == 2 {
		return nil
	}
	return ctxErr(ctx, c.expect2("CWD .."))
}

// Mkdir creates one folder.
func (c *Client) Mkdir(ctx context.Context, p string) error {
	return c.simple(ctx, "MKD "+c.enc(p))
}

// Rmdir removes an empty folder.
func (c *Client) Rmdir(ctx context.Context, p string) error {
	return c.simple(ctx, "RMD "+c.enc(p))
}

// Delete removes a file (or a symbolic link).
func (c *Client) Delete(ctx context.Context, p string) error {
	return c.simple(ctx, "DELE "+c.enc(p))
}

// Rename renames or moves an item within the server; this is also Total
// Commander's server-side move.
func (c *Client) Rename(ctx context.Context, from, to string) error {
	done, err := c.begin(ctx)
	if err != nil {
		return err
	}
	defer done()
	r, err := c.raw("RNFR " + c.enc(from))
	if err != nil {
		return ctxErr(ctx, err)
	}
	if r.Code != 350 {
		return protoErr("RNFR "+from, r)
	}
	return ctxErr(ctx, c.expect2("RNTO "+c.enc(to)))
}

// Chmod changes Unix permissions with SITE CHMOD (only the permission
// bits, including setuid/setgid/sticky, are sent).
func (c *Client) Chmod(ctx context.Context, p string, mode fs.FileMode) error {
	bits := uint32(mode.Perm())
	if mode&fs.ModeSetuid != 0 {
		bits |= 0o4000
	}
	if mode&fs.ModeSetgid != 0 {
		bits |= 0o2000
	}
	if mode&fs.ModeSticky != 0 {
		bits |= 0o1000
	}
	return c.simple(ctx, fmt.Sprintf("SITE CHMOD %03o %s", bits, c.enc(p)))
}

// ServerCopy copies a file within the server using SITE CPFR/CPTO
// (ProFTPD mod_copy). Servers without it give ErrNotSupported; the caller
// then falls back to download plus upload.
func (c *Client) ServerCopy(ctx context.Context, from, to string) error {
	done, err := c.begin(ctx)
	if err != nil {
		return err
	}
	defer done()
	r, err := c.raw("SITE CPFR " + c.enc(from))
	if err != nil {
		return ctxErr(ctx, err)
	}
	switch {
	case r.Code == 350:
	case r.Code == 500 || r.Code == 502 || r.Code == 504:
		return ErrNotSupported
	default:
		return protoErr("SITE CPFR "+from, r)
	}
	return ctxErr(ctx, c.expect2("SITE CPTO "+c.enc(to)))
}

// Size returns a file's size in bytes (SIZE, asked in binary mode so the
// number matches what a download will deliver).
func (c *Client) Size(ctx context.Context, p string) (int64, error) {
	done, err := c.begin(ctx)
	if err != nil {
		return 0, err
	}
	defer done()
	n, err := c.size(p)
	return n, ctxErr(ctx, err)
}

func (c *Client) size(p string) (int64, error) {
	if err := c.setType('I'); err != nil {
		return 0, err
	}
	r, err := c.raw("SIZE " + c.enc(p))
	if err != nil {
		return 0, err
	}
	if r.Code != 213 {
		return 0, protoErr("SIZE "+p, r)
	}
	n, err := strconv.ParseInt(strings.TrimSpace(r.Msg), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("ftp: SIZE %s: bad reply %q", p, r.Msg)
	}
	return n, nil
}

// ModTime returns a file's modification time (MDTM, always UTC).
func (c *Client) ModTime(ctx context.Context, p string) (time.Time, error) {
	done, err := c.begin(ctx)
	if err != nil {
		return time.Time{}, err
	}
	defer done()
	r, err := c.raw("MDTM " + c.enc(p))
	if err != nil {
		return time.Time{}, ctxErr(ctx, err)
	}
	if r.Code != 213 {
		return time.Time{}, protoErr("MDTM "+p, r)
	}
	t, ok := parseFactTime(strings.TrimSpace(r.Msg))
	if !ok {
		return time.Time{}, fmt.Errorf("ftp: MDTM %s: bad reply %q", p, r.Msg)
	}
	return t, nil
}

// SetModTime sets a file's modification time with MFMT, or with the
// two-argument MDTM some servers accept instead.
func (c *Client) SetModTime(ctx context.Context, p string, t time.Time) error {
	done, err := c.begin(ctx)
	if err != nil {
		return err
	}
	defer done()
	stamp := t.UTC().Format("20060102150405")
	if c.hasFeature("MFMT") {
		return ctxErr(ctx, c.expect2("MFMT "+stamp+" "+c.enc(p)))
	}
	r, err := c.raw("MDTM " + stamp + " " + c.enc(p))
	if err != nil {
		return ctxErr(ctx, err)
	}
	if r.Code/100 == 2 {
		return nil
	}
	return ErrNotSupported
}

// setType sends TYPE when the wanted mode differs from the current one.
func (c *Client) setType(t byte) error {
	if c.curType == t {
		return nil
	}
	if err := c.expect2("TYPE " + string(t)); err != nil {
		return err
	}
	c.curType = t
	return nil
}

// Close logs out with QUIT and closes the connection. If a transfer is
// still running in another goroutine, the connection is simply cut.
func (c *Client) Close() error {
	c.StopKeepalive()
	var err error
	c.closeOnce.Do(func() {
		if c.mu.TryLock() {
			if c.broken == nil {
				c.conn.SetDeadline(time.Now().Add(min(c.cfg.Timeout, 5*time.Second)))
				c.raw("QUIT")
			}
			c.broken = ErrClosed
			err = c.conn.Close()
			c.mu.Unlock()
			return
		}
		err = c.conn.Close()
	})
	return err
}

// Join joins remote path elements with forward slashes.
func Join(elem ...string) string {
	p := path.Join(elem...)
	if p == "." {
		return ""
	}
	return p
}
