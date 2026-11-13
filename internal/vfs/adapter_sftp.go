// This file shows SFTP servers in a panel. Total Commander reaches SSH
// servers through its SFTP plugin: the server appears like a drive at
// sftp://user@host/path, and every panel operation (list, view, copy both
// ways, make folder, delete, rename, change permissions and dates) works on
// it. SFTPFS provides that on top of the internal/sftp engine.
//
// One SSH connection per server (scheme, user, host and port) is enough,
// because SFTP multiplexes many requests over it. When the connection dies
// (idle timeout, keep-alive failure, network change) it is dialled again on
// the next operation and that operation is retried once. Host key checks,
// passwords, private keys, agents and proxies come from the Configure hook,
// which the connection manager fills from its stored site (or which asks
// the user); without it, unknown host keys are rejected, which is the safe
// default of the engine. Paths of the form sftp://host/~/dir are relative to
// the login folder, as in the SSH URI convention.
package vfs

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net"
	"os/user"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	pkgsftp "github.com/pkg/sftp"

	"mormicommander/internal/remotefs"
	"mormicommander/internal/sftp"
)

// SFTPFS is the file system of SFTP servers. Create it with NewSFTPFS; it
// is safe for concurrent use.
type SFTPFS struct {
	// Configure, when set, is called with the configuration parsed from a
	// URL before a session is dialled the first time: it adds host key
	// handling (HostKeyPrompt, KnownHostsFiles), passwords, key files,
	// agent use, proxies and timeouts. An error aborts the connection.
	Configure func(cfg *sftp.Config) error

	mu       sync.Mutex
	sessions map[string]*sftpConn
}

// sftpConn is one server session.
type sftpConn struct {
	base remoteURL
	cfg  sftp.Config
	mu   sync.Mutex
	c    *sftp.Client
	home string
}

// NewSFTPFS returns an SFTP file system without sessions.
func NewSFTPFS() *SFTPFS { return &SFTPFS{sessions: map[string]*sftpConn{}} }

// IsSFTPURL reports whether p is a URL SFTPFS answers for.
func IsSFTPURL(p string) bool {
	l := strings.ToLower(p)
	return strings.HasPrefix(l, "sftp://") || strings.HasPrefix(l, "scp://")
}

// normalize maps scp:// to sftp:// (both are served over SFTP) and fills in
// the local login name when the URL has no user, like the ssh command.
func sftpNormalize(u remoteURL) remoteURL {
	u.Scheme = "sftp"
	if u.User == "" {
		if cur, err := user.Current(); err == nil {
			name := cur.Username
			if i := strings.LastIndexAny(name, `\/`); i >= 0 { // DOMAIN\user on Windows
				name = name[i+1:]
			}
			u.User = name
		}
	}
	return u
}

// session returns the session for p (dialling it when new) and the
// absolute remote path p names.
func (f *SFTPFS) session(ctx context.Context, p string) (*sftpConn, string, error) {
	if !IsSFTPURL(p) {
		return nil, "", errors.New("vfs: not an SFTP URL: " + p)
	}
	u, err := parseRemote(p)
	if err != nil {
		return nil, "", err
	}
	u = sftpNormalize(u)
	key := u.key()
	f.mu.Lock()
	s := f.sessions[key]
	f.mu.Unlock()
	if s == nil {
		cfg := sftp.Config{Host: u.Host, Port: u.Port, User: u.User, Password: u.Password}
		if f.Configure != nil {
			if err := f.Configure(&cfg); err != nil {
				return nil, "", err
			}
		}
		if s, err = f.open(ctx, key, u, cfg); err != nil {
			return nil, "", err
		}
	}
	return s, s.abs(u.Path), nil
}

// open dials a session and registers it.
func (f *SFTPFS) open(ctx context.Context, key string, u remoteURL, cfg sftp.Config) (*sftpConn, error) {
	c, err := sftp.Dial(ctx, cfg)
	if err != nil {
		return nil, err
	}
	s := &sftpConn{base: u, cfg: cfg, c: c, home: c.Home()}
	if s.home == "" {
		s.home = "/"
	}
	s.base.Path, s.base.Password, s.base.HasPass = "", "", false
	f.mu.Lock()
	defer f.mu.Unlock()
	if old := f.sessions[key]; old != nil {
		c.Close()
		return old, nil
	}
	f.sessions[key] = s
	return s, nil
}

// abs resolves a URL path: "" and "/~" are the login folder, "/~/x" is
// below it.
func (s *sftpConn) abs(p string) string {
	switch {
	case p == "" || p == "/~":
		return s.home
	case strings.HasPrefix(p, "/~/"):
		return path.Join(s.home, p[3:])
	}
	return path.Clean(p)
}

func (s *sftpConn) url(rp string) string {
	u := s.base
	u.Path = path.Clean("/" + rp)
	return u.String()
}

// client returns a live client, redialling a dead connection.
func (s *sftpConn) client(ctx context.Context) (*sftp.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.c != nil && !sftpClosed(s.c) {
		return s.c, nil
	}
	if s.c != nil {
		s.c.Close()
		s.c = nil
	}
	c, err := sftp.Dial(ctx, s.cfg)
	if err != nil {
		return nil, err
	}
	s.c = c
	return c, nil
}

// drop closes c if it is still the session's client, so the next call
// dials a new connection.
func (s *sftpConn) drop(c *sftp.Client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.c == c {
		c.Close()
		s.c = nil
	}
}

// sftpClosed reports whether the connection has ended.
func sftpClosed(c *sftp.Client) bool {
	select {
	case <-c.Done():
		return true
	default:
		return false
	}
}

// sftpConnLost reports whether err means the connection is gone (rather
// than an ordinary error such as a missing file).
func sftpConnLost(c *sftp.Client, err error) bool {
	return sftpClosed(c) || errors.Is(err, sftp.ErrClosed) || errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, pkgsftp.ErrSSHFxConnectionLost) ||
		errors.Is(err, net.ErrClosed)
}

// with runs fn with a live client, retrying once after a reconnect when the
// connection turned out to be dead.
func (f *SFTPFS) with(ctx context.Context, p string, fn func(c *sftp.Client, rp string) error) error {
	s, rp, err := f.session(ctx, p)
	if err != nil {
		return err
	}
	for attempt := 0; ; attempt++ {
		c, err := s.client(ctx)
		if err != nil {
			return err
		}
		err = fn(c, rp)
		if err != nil && sftpConnLost(c, err) {
			s.drop(c)
			if attempt == 0 && ctx.Err() == nil {
				continue
			}
		}
		return err
	}
}

// Connect opens (or reuses) the session for a typed URL and returns the
// panel path to show (the login folder when the URL has no path).
func (f *SFTPFS) Connect(ctx context.Context, rawURL string) (string, error) {
	s, rp, err := f.session(ctx, rawURL)
	if err != nil {
		return "", err
	}
	return s.url(rp), nil
}

// ConnectConfig opens a session from a connection-manager site, replacing
// an existing session for the same server and user. It returns the panel
// path of the site's StartDir or the login folder.
func (f *SFTPFS) ConnectConfig(ctx context.Context, cfg sftp.Config) (string, error) {
	u := sftpNormalize(remoteURL{Scheme: "sftp", User: cfg.User, Host: strings.ToLower(cfg.Host)})
	if cfg.User == "" {
		cfg.User = u.User
	}
	if cfg.Port != 0 && cfg.Port != defaultPorts["sftp"] {
		u.Port = cfg.Port
	}
	f.Disconnect(u.String())
	s, err := f.open(ctx, u.key(), u, cfg)
	if err != nil {
		return "", err
	}
	start := s.home
	if s.c != nil {
		start = s.c.StartDir()
	}
	return s.url(start), nil
}

// Disconnect closes the session p belongs to.
func (f *SFTPFS) Disconnect(p string) {
	u, err := parseRemote(p)
	if err != nil {
		return
	}
	key := sftpNormalize(u).key()
	f.mu.Lock()
	s := f.sessions[key]
	delete(f.sessions, key)
	f.mu.Unlock()
	if s != nil {
		s.mu.Lock()
		if s.c != nil {
			s.c.Close()
		}
		s.mu.Unlock()
	}
}

// Sessions lists the connected servers as panel URLs of their roots.
func (f *SFTPFS) Sessions() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.sessions))
	for _, s := range f.sessions {
		out = append(out, s.url("/"))
	}
	sort.Strings(out)
	return out
}

// Close disconnects every session.
func (f *SFTPFS) Close() error {
	f.mu.Lock()
	ss := f.sessions
	f.sessions = map[string]*sftpConn{}
	f.mu.Unlock()
	for _, s := range ss {
		s.mu.Lock()
		if s.c != nil {
			s.c.Close()
		}
		s.mu.Unlock()
	}
	return nil
}

// sftpEntry converts an engine entry.
func sftpEntry(e remotefs.Entry) Entry {
	return modeEntry(e.Name, e.Size, e.ModTime, e.Mode, e.IsDir, e.IsLink, e.LinkBroken, true, false, false, false)
}

// List reads a remote folder; links are resolved by the engine.
func (f *SFTPFS) List(ctx context.Context, p string) (Listing, error) {
	var out []Entry
	var canon string
	var s *sftpConn
	err := f.with(ctx, p, func(c *sftp.Client, rp string) error {
		es, err := c.ReadDir(rp)
		if err != nil {
			if e, serr := c.Stat(rp); serr == nil && !e.IsDir {
				return ErrNotFolder
			}
			return err
		}
		out = make([]Entry, len(es))
		for i, e := range es {
			out[i] = sftpEntry(e)
		}
		canon = rp
		return ctx.Err()
	})
	if err != nil {
		return Listing{}, err
	}
	s, _, _ = f.session(ctx, p)
	lp := s.url(canon)
	return Listing{Path: lp, Parent: urlParent(lp), Entries: out}, nil
}

// Stat describes one path; a link is shown with its target's type and size
// and marked as a link (broken when the target is missing).
func (f *SFTPFS) Stat(p string) (Entry, error) {
	var out Entry
	err := f.with(context.Background(), p, func(c *sftp.Client, rp string) error {
		e, err := c.Lstat(rp)
		if err != nil {
			return err
		}
		if e.IsLink {
			if t, err := c.Stat(rp); err == nil {
				e.Size, e.IsDir, e.ModTime = t.Size, t.IsDir, t.ModTime
			} else {
				e.LinkBroken = true
			}
		}
		if rp == "/" {
			e.Name = "/"
		}
		out = sftpEntry(e)
		return nil
	})
	return out, err
}

// Clean returns the canonical panel URL (scp:// becomes sftp://, the local
// user name is filled in, the password is dropped).
func (f *SFTPFS) Clean(p string) string {
	u, err := parseRemote(p)
	if err != nil {
		return p
	}
	return sftpNormalize(u).String()
}

// Parent is the folder above p, "" at the server root.
func (f *SFTPFS) Parent(p string) string { return urlParent(f.Clean(p)) }

// Root is the server root.
func (f *SFTPFS) Root(p string) string { return urlRoot(f.Clean(p)) }

// Join puts name under dir.
func (f *SFTPFS) Join(dir, name string) string { return urlJoin(f.Clean(dir), name) }

// Caps: SFTP can do everything a panel asks for.
func (f *SFTPFS) Caps(string) Caps {
	return Caps{Read: true, Write: true, Mkdir: true, Delete: true, Rename: true, Chmod: true, Chtimes: true, Remote: true}
}

// Open opens a remote file; the reader also implements io.Seeker and
// io.ReaderAt, so the lister can page through big files.
func (f *SFTPFS) Open(ctx context.Context, p string) (io.ReadCloser, error) {
	var rc io.ReadCloser
	err := f.with(ctx, p, func(c *sftp.Client, rp string) error {
		e, err := c.Stat(rp)
		if err != nil {
			return err
		}
		if e.IsDir {
			return opErr("open", p, ErrIsFolder)
		}
		rc, err = c.Open(rp)
		return err
	})
	return rc, err
}

// Create creates or truncates a remote file.
func (f *SFTPFS) Create(ctx context.Context, p string) (io.WriteCloser, error) {
	var wc io.WriteCloser
	err := f.with(ctx, p, func(c *sftp.Client, rp string) error {
		var err error
		wc, err = c.Create(rp)
		return err
	})
	return wc, err
}

// Mkdir creates a folder with missing parents; an existing path is
// fs.ErrExist.
func (f *SFTPFS) Mkdir(ctx context.Context, p string) error {
	return f.with(ctx, p, func(c *sftp.Client, rp string) error {
		if ok, _ := c.Exists(rp); ok {
			return &fs.PathError{Op: "mkdir", Path: rp, Err: fs.ErrExist}
		}
		return c.MkdirAll(rp)
	})
}

// Remove deletes a file, a link or an empty folder.
func (f *SFTPFS) Remove(ctx context.Context, p string) error {
	return f.with(ctx, p, func(c *sftp.Client, rp string) error { return c.Remove(rp) })
}

// RemoveAll deletes a folder tree (links are removed, not followed).
func (f *SFTPFS) RemoveAll(ctx context.Context, p string) error {
	return f.with(ctx, p, func(c *sftp.Client, rp string) error {
		if _, err := c.Lstat(rp); err != nil {
			return err
		}
		return c.RemoveAll(ctx, rp, nil)
	})
}

// Rename moves within one server; an existing target is fs.ErrExist.
func (f *SFTPFS) Rename(ctx context.Context, from, to string) error {
	if !IsSFTPURL(to) {
		return opErr("rename", from, ErrCrossFS)
	}
	uf, err1 := parseRemote(from)
	ut, err2 := parseRemote(to)
	if err1 != nil || err2 != nil || sftpNormalize(uf).key() != sftpNormalize(ut).key() {
		return opErr("rename", from, ErrCrossFS)
	}
	_, tp, err := f.session(ctx, to)
	if err != nil {
		return err
	}
	return f.with(ctx, from, func(c *sftp.Client, rp string) error { return c.Rename(rp, tp) })
}

// Chmod changes permission bits.
func (f *SFTPFS) Chmod(ctx context.Context, p string, mode fs.FileMode) error {
	return f.with(ctx, p, func(c *sftp.Client, rp string) error { return c.Chmod(rp, mode) })
}

// Chtimes changes the modification (and access) time.
func (f *SFTPFS) Chtimes(ctx context.Context, p string, mtime time.Time) error {
	return f.with(ctx, p, func(c *sftp.Client, rp string) error { return c.Chtimes(rp, mtime, mtime) })
}

func sftpTransferOpts(opt TransferOptions) remotefs.TransferOptions {
	o := remotefs.TransferOptions{Resume: opt.Resume, PreserveTimes: opt.PreserveTime}
	if opt.Progress != nil {
		o.Progress = func(p remotefs.Progress) { opt.Progress(p.Done, p.Total) }
	}
	return o
}

// Download copies a remote file to the local disk, with resume.
func (f *SFTPFS) Download(ctx context.Context, p, local string, opt TransferOptions) (int64, error) {
	var n int64
	err := f.with(ctx, p, func(c *sftp.Client, rp string) error {
		var err error
		n, err = c.Download(ctx, rp, local, sftpTransferOpts(opt))
		return err
	})
	return n, err
}

// Upload copies a local file to the server, with resume.
func (f *SFTPFS) Upload(ctx context.Context, local, p string, opt TransferOptions) (int64, error) {
	var n int64
	err := f.with(ctx, p, func(c *sftp.Client, rp string) error {
		var err error
		n, err = c.Upload(ctx, local, rp, sftpTransferOpts(opt))
		return err
	})
	return n, err
}
