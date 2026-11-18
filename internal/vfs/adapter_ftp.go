// This file shows FTP servers in a panel. Total Commander's FTP client
// (Ctrl+F connect, Ctrl+N quick connect, or typing ftp://host in the path
// bar) puts the server into the panel like a drive: folders are listed and
// entered, F3 views a remote file, F5/F6 copy and move, F7 makes folders, F8
// deletes, Shift+F6 renames, and Files > Change Attributes sends SITE CHMOD.
// FTPFS does that on top of the internal/ftp engine.
//
// Each server (scheme, user, host and port) gets one session holding a small
// pool of logged-in connections. A pool is needed because one FTP control
// connection is busy for as long as a file is being read, and a panel must
// still be able to list folders while the viewer reads a file or a job copies
// in the background. Connections that died (server timeout, network change)
// are replaced and the operation is retried once, which is what TC's
// "reconnect automatically" does. Passwords never appear in panel paths: the
// first URL may carry one (ftp://user:pw@host), and the Configure hook lets
// the connection manager fill in stored passwords, TLS mode, proxies and the
// rest of a site's settings before a connection is dialled.
package vfs

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"mormicommander/internal/ftp"
)

// FTPFS is the file system of FTP servers. Create it with NewFTPFS; it is
// safe for concurrent use.
type FTPFS struct {
	// Configure, when set, is called with the configuration parsed from a
	// URL before a new session is dialled. It fills in what the URL cannot
	// carry (stored password, TLS, proxy, passive mode, encoding, time zone)
	// or asks the user; an error aborts the connection.
	Configure func(cfg *ftp.Config) error
	// MaxConns is the number of connections per server (default 2: one for
	// the panel, one for a transfer).
	MaxConns int
	// MaxConnsFunc, when set, gives the number of connections for a new
	// session instead of MaxConns (Options > FTP, read when connecting).
	MaxConnsFunc func() int
	// NoLinkResolve turns off finding out whether symbolic links in a
	// listing point to folders (one CWD per link, at most 64 per listing).
	NoLinkResolve bool

	mu       sync.Mutex
	sessions map[string]*ftpConn
}

// ftpConn is one server session.
type ftpConn struct {
	key  string
	base remoteURL // scheme, user, host, port
	cfg  ftp.Config
	pool *ftp.Pool
	home string
	mfmt bool
}

// NewFTPFS returns an FTP file system without sessions.
func NewFTPFS() *FTPFS { return &FTPFS{sessions: map[string]*ftpConn{}} }

// IsFTPURL reports whether p is a URL FTPFS answers for.
func IsFTPURL(p string) bool {
	l := strings.ToLower(p)
	return strings.HasPrefix(l, "ftp://") || strings.HasPrefix(l, "ftps://") || strings.HasPrefix(l, "ftpes://")
}

// configFor builds the engine configuration for a URL.
func (f *FTPFS) configFor(u remoteURL) (ftp.Config, error) {
	raw := u.Scheme + "://" + u.Host
	if strings.Contains(u.Host, ":") {
		raw = u.Scheme + "://[" + u.Host + "]"
	}
	if u.Port != 0 {
		raw += ":" + strconv.Itoa(u.Port)
	}
	cfg, _, err := ftp.ParseURL(raw)
	if err != nil {
		return ftp.Config{}, err
	}
	cfg.User, cfg.Password = u.User, u.Password
	if f.Configure != nil {
		if err := f.Configure(&cfg); err != nil {
			return ftp.Config{}, err
		}
	}
	return cfg, nil
}

// session returns the session for the URL p, dialling it when needed, and
// the remote path p names ("" when p has no path).
func (f *FTPFS) session(ctx context.Context, p string) (*ftpConn, string, error) {
	u, err := parseRemote(p)
	if err != nil {
		return nil, "", err
	}
	if !IsFTPURL(p) {
		return nil, "", errors.New("vfs: not an FTP URL: " + p)
	}
	key := u.key()
	f.mu.Lock()
	s := f.sessions[key]
	f.mu.Unlock()
	if s == nil {
		cfg, err := f.configFor(u)
		if err != nil {
			return nil, "", err
		}
		s, err = f.open(ctx, key, u, cfg)
		if err != nil {
			return nil, "", err
		}
	}
	rp := u.Path
	if rp == "" {
		rp = s.home
	}
	return s, rp, nil
}

// open dials the first connection of a session to check the login and
// learn the start folder, then registers the session.
func (f *FTPFS) open(ctx context.Context, key string, u remoteURL, cfg ftp.Config) (*ftpConn, error) {
	max := f.MaxConns
	if f.MaxConnsFunc != nil {
		max = f.MaxConnsFunc()
	}
	if max <= 0 {
		max = 2
	}
	pool := ftp.NewPool(cfg, max)
	c, err := pool.Get(ctx)
	if err != nil {
		pool.Close()
		return nil, err
	}
	home, err := c.Pwd(ctx)
	if err != nil || home == "" {
		home = "/"
	}
	s := &ftpConn{key: key, base: u, cfg: cfg, pool: pool, home: path.Clean(home),
		mfmt: c.HasFeature("MFMT")}
	pool.Put(c)
	s.base.Path, s.base.Password, s.base.HasPass = "", "", false
	f.mu.Lock()
	defer f.mu.Unlock()
	if old := f.sessions[key]; old != nil {
		pool.Close()
		return old, nil
	}
	f.sessions[key] = s
	return s, nil
}

// Connect opens (or reuses) the session for a typed URL and returns the
// panel path to show: the URL's folder, or the login folder when the URL
// has no path.
func (f *FTPFS) Connect(ctx context.Context, rawURL string) (string, error) {
	s, rp, err := f.session(ctx, rawURL)
	if err != nil {
		return "", err
	}
	return s.url(rp), nil
}

// ConnectConfig opens a session from a connection-manager site. A session
// for the same server and user is replaced. The returned panel path is the
// site's InitialDir, or the login folder.
func (f *FTPFS) ConnectConfig(ctx context.Context, cfg ftp.Config) (string, error) {
	scheme := "ftp"
	switch cfg.Security {
	case ftp.SecurityImplicitTLS:
		scheme = "ftps"
	case ftp.SecurityExplicitTLS:
		scheme = "ftpes"
	}
	u := remoteURL{Scheme: scheme, User: cfg.User, Host: strings.ToLower(cfg.Host)}
	if cfg.Port != 0 && cfg.Port != defaultPorts[scheme] {
		u.Port = cfg.Port
	}
	key := u.key()
	f.Disconnect(u.String())
	s, err := f.open(ctx, key, u, cfg)
	if err != nil {
		return "", err
	}
	return s.url(s.home), nil
}

// Disconnect closes the session the URL p belongs to (TC's Disconnect).
func (f *FTPFS) Disconnect(p string) {
	u, err := parseRemote(p)
	if err != nil {
		return
	}
	f.mu.Lock()
	s := f.sessions[u.key()]
	delete(f.sessions, u.key())
	f.mu.Unlock()
	if s != nil {
		s.pool.Close()
	}
}

// Sessions lists the connected servers as panel URLs of their roots.
func (f *FTPFS) Sessions() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.sessions))
	for _, s := range f.sessions {
		out = append(out, s.url("/"))
	}
	sort.Strings(out)
	return out
}

// CloseIdle closes pooled connections unused for longer than d (TC's
// "disconnect after idle time").
func (f *FTPFS) CloseIdle(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range f.sessions {
		s.pool.CloseIdle(d)
	}
}

// Close disconnects every session.
func (f *FTPFS) Close() error {
	f.mu.Lock()
	ss := f.sessions
	f.sessions = map[string]*ftpConn{}
	f.mu.Unlock()
	for _, s := range ss {
		s.pool.Close()
	}
	return nil
}

func (s *ftpConn) url(rp string) string {
	u := s.base
	u.Path = path.Clean("/" + rp)
	return u.String()
}

// with runs fn on a pooled connection, retrying once on a fresh connection
// when the first one turned out to be dead.
func (f *FTPFS) with(ctx context.Context, p string, fn func(c *ftp.Client, rp string) error) error {
	s, rp, err := f.session(ctx, p)
	if err != nil {
		return err
	}
	for attempt := 0; ; attempt++ {
		c, err := s.pool.Get(ctx)
		if err != nil {
			return err
		}
		err = fn(c, rp)
		broken := c.Broken() != nil
		s.pool.Put(c)
		if err != nil && broken && attempt == 0 && ctx.Err() == nil {
			continue
		}
		return err
	}
}

// ftpEntry converts an engine entry.
func ftpEntry(e ftp.Entry, isDir bool) Entry {
	return modeEntry(e.Name, e.Size, e.ModTime, e.Mode, isDir, e.Type == ftp.TypeLink, false, e.HasMode, false, false, false)
}

// List reads a remote folder.
func (f *FTPFS) List(ctx context.Context, p string) (Listing, error) {
	var out []Entry
	var canon string
	err := f.with(ctx, p, func(c *ftp.Client, rp string) error {
		es, err := c.List(ctx, rp)
		if err != nil {
			if e, serr := c.Stat(ctx, rp); serr == nil && !e.IsDir() && e.Type != ftp.TypeLink {
				return ErrNotFolder
			} else if serr != nil && errors.Is(serr, fs.ErrNotExist) {
				return &fs.PathError{Op: "list", Path: rp, Err: fs.ErrNotExist}
			}
			return err
		}
		// Many servers answer LIST/MLSD of a file with that file's own
		// line, so a short answer is double-checked.
		if len(es) <= 1 && rp != "/" {
			if e, serr := c.Stat(ctx, rp); serr == nil && !e.IsDir() && e.Type != ftp.TypeLink {
				return ErrNotFolder
			}
		}
		out = make([]Entry, 0, len(es))
		links := 0
		for _, e := range es {
			if e.Name == "" || e.Name == "." || e.Name == ".." || strings.ContainsRune(e.Name, '/') {
				continue
			}
			isDir := e.IsDir() || e.Mode&fs.ModeDir != 0
			if e.Type == ftp.TypeLink && !f.NoLinkResolve && links < 64 {
				links++
				if strings.HasSuffix(e.Target, "/") {
					isDir = true
				} else if c.Cd(ctx, path.Join(rp, e.Name)) == nil {
					isDir = true
				}
			}
			out = append(out, ftpEntry(e, isDir))
		}
		canon = rp
		return ctx.Err()
	})
	if err != nil {
		return Listing{}, err
	}
	s, _, _ := f.session(ctx, p)
	lp := s.url(canon)
	return Listing{Path: lp, Parent: urlParent(lp), Entries: out}, nil
}

// Stat describes one remote path.
func (f *FTPFS) Stat(p string) (Entry, error) {
	var out Entry
	ctx := context.Background()
	err := f.with(ctx, p, func(c *ftp.Client, rp string) error {
		if rp == "/" {
			out = Entry{Name: "/", Flags: FlagDir, Mode: uint32(fs.ModeDir | 0o755)}
			return nil
		}
		e, err := c.Stat(ctx, rp)
		if err != nil {
			return err
		}
		isDir := e.IsDir()
		if e.Type == ftp.TypeLink && c.Cd(ctx, rp) == nil {
			isDir = true
		}
		e.Name = path.Base(rp)
		out = ftpEntry(e, isDir)
		return nil
	})
	return out, err
}

// Clean returns the canonical panel URL (without password).
func (f *FTPFS) Clean(p string) string { return cleanURL(p) }

// Parent is the folder above p, "" at the server root.
func (f *FTPFS) Parent(p string) string { return urlParent(p) }

// Root is the server root.
func (f *FTPFS) Root(p string) string { return urlRoot(p) }

// Join puts name under dir.
func (f *FTPFS) Join(dir, name string) string { return urlJoin(dir, name) }

// Caps reports what works on an FTP server.
func (f *FTPFS) Caps(p string) Caps {
	c := Caps{Read: true, Write: true, Mkdir: true, Delete: true, Rename: true, Chmod: true, Remote: true}
	if u, err := parseRemote(p); err == nil {
		f.mu.Lock()
		if s := f.sessions[u.key()]; s != nil {
			c.Chtimes = s.mfmt
		}
		f.mu.Unlock()
	}
	return c
}

// ftpReader returns its connection to the pool when closed.
type ftpReader struct {
	io.ReadCloser
	pool *ftp.Pool
	c    *ftp.Client
	once sync.Once
}

func (r *ftpReader) Close() error {
	err := r.ReadCloser.Close()
	r.once.Do(func() { r.pool.Put(r.c) })
	return err
}

// Open streams a remote file (binary mode). The connection is reserved
// until the reader is closed.
func (f *FTPFS) Open(ctx context.Context, p string) (io.ReadCloser, error) {
	s, rp, err := f.session(ctx, p)
	if err != nil {
		return nil, err
	}
	for attempt := 0; ; attempt++ {
		c, err := s.pool.Get(ctx)
		if err != nil {
			return nil, err
		}
		rc, err := c.Retrieve(ctx, rp, 0, ftp.TransferBinary)
		if err != nil {
			broken := c.Broken() != nil
			if !broken {
				if e, serr := c.Stat(ctx, rp); serr == nil && e.IsDir() {
					err = ErrIsFolder
				}
			}
			s.pool.Put(c)
			if broken && attempt == 0 && ctx.Err() == nil {
				continue
			}
			return nil, opErr("open", p, err)
		}
		return &ftpReader{ReadCloser: rc, pool: s.pool, c: c}, nil
	}
}

// ftpWriter feeds a STOR running in the background.
type ftpWriter struct {
	pw   *io.PipeWriter
	done chan error
	once sync.Once
	err  error
}

func (w *ftpWriter) Write(b []byte) (int, error) { return w.pw.Write(b) }

func (w *ftpWriter) Close() error {
	w.once.Do(func() {
		w.pw.Close()
		w.err = <-w.done
	})
	return w.err
}

// Create uploads what is written to the returned writer as p (binary
// mode); the upload completes when it is closed.
func (f *FTPFS) Create(ctx context.Context, p string) (io.WriteCloser, error) {
	s, rp, err := f.session(ctx, p)
	if err != nil {
		return nil, err
	}
	c, err := s.pool.Get(ctx)
	if err != nil {
		return nil, err
	}
	pr, pw := io.Pipe()
	w := &ftpWriter{pw: pw, done: make(chan error, 1)}
	go func() {
		_, err := c.Store(ctx, rp, pr, 0, ftp.TransferBinary)
		pr.CloseWithError(errOr(err, io.ErrClosedPipe))
		s.pool.Put(c)
		if err != nil {
			err = opErr("create", p, err)
		}
		w.done <- err
	}()
	return w, nil
}

// errOr returns err, or alt when err is nil.
func errOr(err, alt error) error {
	if err != nil {
		return err
	}
	return alt
}

// Mkdir creates a folder, with missing parents (F7 accepts "a/b/c").
func (f *FTPFS) Mkdir(ctx context.Context, p string) error {
	return f.with(ctx, p, func(c *ftp.Client, rp string) error {
		if _, err := c.Stat(ctx, rp); err == nil {
			return &fs.PathError{Op: "mkdir", Path: rp, Err: fs.ErrExist}
		}
		return c.MkdirAll(ctx, rp)
	})
}

// Remove deletes a file or an empty folder.
func (f *FTPFS) Remove(ctx context.Context, p string) error {
	return f.with(ctx, p, func(c *ftp.Client, rp string) error {
		e, err := c.Stat(ctx, rp)
		if err != nil {
			return err
		}
		if e.IsDir() {
			return c.Rmdir(ctx, rp)
		}
		return c.Delete(ctx, rp)
	})
}

// RemoveAll deletes a file or a folder tree.
func (f *FTPFS) RemoveAll(ctx context.Context, p string) error {
	return f.with(ctx, p, func(c *ftp.Client, rp string) error {
		if _, err := c.Stat(ctx, rp); err != nil {
			return err
		}
		return c.RemoveAll(ctx, rp)
	})
}

// Rename moves within one server. The target must not exist.
func (f *FTPFS) Rename(ctx context.Context, from, to string) error {
	uf, err1 := parseRemote(from)
	ut, err2 := parseRemote(to)
	if err1 != nil || err2 != nil || uf.key() != ut.key() || !IsFTPURL(to) {
		return opErr("rename", from, ErrCrossFS)
	}
	return f.with(ctx, from, func(c *ftp.Client, rp string) error {
		tp := ut.Path
		if _, err := c.Stat(ctx, tp); err == nil {
			return &fs.PathError{Op: "rename", Path: tp, Err: fs.ErrExist}
		}
		return c.Rename(ctx, rp, tp)
	})
}

// Chmod sends SITE CHMOD.
func (f *FTPFS) Chmod(ctx context.Context, p string, mode fs.FileMode) error {
	return f.with(ctx, p, func(c *ftp.Client, rp string) error { return c.Chmod(ctx, rp, mode) })
}

// Chtimes sets the modification time with MFMT.
func (f *FTPFS) Chtimes(ctx context.Context, p string, mtime time.Time) error {
	return f.with(ctx, p, func(c *ftp.Client, rp string) error { return c.SetModTime(ctx, rp, mtime) })
}

// transferOpts maps the shared options onto the engine's.
func (s *ftpConn) transferOpts(opt TransferOptions) ftp.TransferOptions {
	return ftp.TransferOptions{Mode: s.cfg.TransferMode, Resume: opt.Resume, PreserveTime: opt.PreserveTime, Progress: opt.Progress}
}

// Download copies a remote file to the local disk, in the session's
// transfer mode (binary, ASCII or automatic by file mask), with resume.
func (f *FTPFS) Download(ctx context.Context, p, local string, opt TransferOptions) (int64, error) {
	s, _, err := f.session(ctx, p)
	if err != nil {
		return 0, err
	}
	var n int64
	err = f.with(ctx, p, func(c *ftp.Client, rp string) error {
		var err error
		n, err = c.Download(ctx, rp, local, s.transferOpts(opt))
		return err
	})
	return n, err
}

// Upload copies a local file to the server, with resume.
func (f *FTPFS) Upload(ctx context.Context, local, p string, opt TransferOptions) (int64, error) {
	s, _, err := f.session(ctx, p)
	if err != nil {
		return 0, err
	}
	var n int64
	err = f.with(ctx, p, func(c *ftp.Client, rp string) error {
		var err error
		n, err = c.Upload(ctx, local, rp, s.transferOpts(opt))
		return err
	})
	return n, err
}
