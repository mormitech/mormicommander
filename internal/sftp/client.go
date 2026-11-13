// This file is the heart of the SFTP client: it dials the server (directly or
// through a proxy), runs the SSH handshake with host key checking and login,
// opens the SFTP subsystem and then offers the file operations a Total
// Commander panel needs on a remote folder: list, stat, make folder, delete,
// rename, change attributes, create links, query free space and run a raw
// command. Two background helpers keep the connection healthy the way Total
// Commander's FTP/SFTP options do: a keep-alive that pings the server so idle
// connections survive NAT routers (and dead ones are noticed), and an idle
// timer that disconnects after a period without activity. Servers that only
// allow SCP (no SFTP subsystem) still connect; the SFTP methods then return
// ErrNoSFTP and the SCP functions in scp.go are used instead.
package sftp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	pkgsftp "github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"mormicommander/internal/remotefs"
)

var (
	// ErrNoSFTP is returned by SFTP operations when the server refused the
	// SFTP subsystem (an SCP-only or shell-only account).
	ErrNoSFTP = errors.New("sftp: server does not offer the SFTP subsystem")
	// ErrClosed is returned after the connection was closed.
	ErrClosed = errors.New("sftp: connection closed")
	// ErrIdleTimeout is what Err reports after the idle timer disconnected.
	ErrIdleTimeout = errors.New("sftp: disconnected after idle timeout")
	// ErrKeepAliveFailed is what Err reports when the server stopped
	// answering keep-alive requests.
	ErrKeepAliveFailed = errors.New("sftp: server stopped answering keep-alive requests")
)

// Client is one live SSH connection with its SFTP session. All methods are
// safe for concurrent use; pkg/sftp pipelines requests over the one channel.
type Client struct {
	cfg  Config
	conn *ssh.Client
	sftp *pkgsftp.Client
	home string

	lastUse atomic.Int64 // unix nanoseconds
	busy    atomic.Int32 // operations in progress

	closeOnce sync.Once
	done      chan struct{}
	mu        sync.Mutex
	reason    error
}

var _ remotefs.FS = (*Client)(nil)

// Dial connects, authenticates and opens the SFTP subsystem. The context
// cancels the dial (including a pending host key prompt's network wait); the
// Config.Timeout bounds connect plus handshake, but the clock is paused while
// HostKeyPrompt waits for the user.
func Dial(ctx context.Context, cfg Config) (*Client, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	sshCfg, cleanup, err := cfg.sshConfig()
	if err != nil {
		return nil, err
	}
	defer cleanup()

	addr := cfg.Addr()
	conn, err := cfg.dialTCP(ctx, addr)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(cfg.timeout())
	conn.SetDeadline(deadline)

	// Pause the handshake deadline while the user looks at the host key
	// dialog; a person may take longer than the network timeout.
	prompt := cfg.HostKeyPrompt
	if prompt != nil {
		cfg.HostKeyPrompt = func(info HostKeyInfo) HostKeyDecision {
			conn.SetDeadline(time.Time{})
			d := prompt(info)
			conn.SetDeadline(time.Now().Add(cfg.timeout()))
			return d
		}
	}
	hostCB, algos, err := cfg.hostKeyCheck()
	if err != nil {
		conn.Close()
		return nil, err
	}
	sshCfg.HostKeyCallback = hostCB
	if len(algos) > 0 {
		sshCfg.HostKeyAlgorithms = algos
	}

	// Abort the handshake when the context is cancelled.
	stop := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-stop:
		}
	}()
	cc, chans, reqs, err := ssh.NewClientConn(conn, addr, sshCfg)
	close(stop)
	if err != nil {
		conn.Close()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("sftp: connecting to %s: %w", addr, err)
	}
	conn.SetDeadline(time.Time{})
	sc := ssh.NewClient(cc, chans, reqs)

	c := &Client{cfg: cfg, conn: sc, done: make(chan struct{})}
	c.touch()
	if s, err := pkgsftp.NewClient(sc); err == nil {
		c.sftp = s
		if wd, err := s.Getwd(); err == nil {
			c.home = wd
		}
	}
	if c.home == "" {
		c.home = "/"
	}

	go func() {
		// Notice the server going away.
		sc.Wait()
		c.shutdown(nil)
	}()
	if cfg.KeepAlive > 0 {
		go c.keepAlive(cfg.KeepAlive, keepAliveMinWait)
	}
	if cfg.IdleTimeout > 0 {
		go c.idleWatch(cfg.IdleTimeout)
	}
	return c, nil
}

func (cfg *Config) dialTCP(ctx context.Context, addr string) (net.Conn, error) {
	var conn net.Conn
	var err error
	switch {
	case cfg.Dial != nil:
		conn, err = cfg.Dial("tcp", addr)
	case cfg.Proxy != nil:
		conn, err = cfg.Proxy.Dial(addr, cfg.timeout())
	default:
		d := net.Dialer{Timeout: cfg.timeout()}
		conn, err = d.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("sftp: connecting to %s: %w", addr, err)
	}
	return conn, nil
}

// Close disconnects. It is safe to call more than once.
func (c *Client) Close() error {
	c.shutdown(nil)
	return nil
}

func (c *Client) shutdown(reason error) {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.reason = reason
		c.mu.Unlock()
		if c.sftp != nil {
			c.sftp.Close()
		}
		c.conn.Close()
		close(c.done)
	})
}

// Done is closed when the connection ends for any reason (Close, idle
// timeout, keep-alive failure, server disconnect). The panel uses it to show
// "connection lost" and switch back to a local folder.
func (c *Client) Done() <-chan struct{} { return c.done }

// Err tells why the connection ended: ErrIdleTimeout, ErrKeepAliveFailed, or
// nil for a normal close or a server-side disconnect. It returns nil while
// the connection is open.
func (c *Client) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.reason
}

// Config returns the configuration the client was dialled with.
func (c *Client) Config() Config { return c.cfg }

// Home returns the login folder reported by the server.
func (c *Client) Home() string { return c.home }

// StartDir resolves Config.StartDir against the login folder.
func (c *Client) StartDir() string {
	if c.cfg.StartDir == "" {
		return c.home
	}
	return c.abs(c.cfg.StartDir)
}

// HasSFTP reports whether the SFTP subsystem is available.
func (c *Client) HasSFTP() bool { return c.sftp != nil }

// SSH exposes the underlying SSH connection (port forwarding, extra
// sessions). Most callers never need it.
func (c *Client) SSH() *ssh.Client { return c.conn }

// ServerVersion returns the server's SSH identification string.
func (c *Client) ServerVersion() string { return string(c.conn.ServerVersion()) }

// HasExtension reports whether the SFTP server announced an extension such
// as "posix-rename@openssh.com" or "statvfs@openssh.com".
func (c *Client) HasExtension(name string) bool {
	if c.sftp == nil {
		return false
	}
	_, ok := c.sftp.HasExtension(name)
	return ok
}

// LastActivity returns when the last operation finished or progressed.
func (c *Client) LastActivity() time.Time { return time.Unix(0, c.lastUse.Load()) }

func (c *Client) touch() { c.lastUse.Store(time.Now().UnixNano()) }

// begin marks an operation in progress (so the idle timer never fires in the
// middle of a transfer) and returns the matching end function.
func (c *Client) begin() (func(), error) {
	if c.sftp == nil {
		select {
		case <-c.done:
			return nil, ErrClosed
		default:
		}
		return nil, ErrNoSFTP
	}
	return c.beginAny()
}

// beginAny is begin for operations that do not need the SFTP subsystem
// (SCP transfers and remote commands).
func (c *Client) beginAny() (func(), error) {
	select {
	case <-c.done:
		return nil, ErrClosed
	default:
	}
	c.busy.Add(1)
	c.touch()
	return func() { c.touch(); c.busy.Add(-1) }, nil
}

// keepAliveMinWait is the shortest time a keep-alive reply is waited for
// (a variable so tests can shorten it).
var keepAliveMinWait = 15 * time.Second

// keepAlive sends "keepalive@openssh.com" global requests. A request that is
// not answered within max(interval, 15s) means the link is dead.
func (c *Client) keepAlive(interval, minWait time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	wait := interval
	if wait < minWait {
		wait = minWait
	}
	for {
		select {
		case <-c.done:
			return
		case <-t.C:
		}
		res := make(chan error, 1)
		go func() {
			_, _, err := c.conn.SendRequest("keepalive@openssh.com", true, nil)
			res <- err
		}()
		select {
		case err := <-res:
			if err != nil {
				c.shutdown(ErrKeepAliveFailed)
				return
			}
		case <-time.After(wait):
			c.shutdown(ErrKeepAliveFailed)
			return
		case <-c.done:
			return
		}
	}
}

// idleWatch disconnects after timeout without activity, but never while an
// operation is running.
func (c *Client) idleWatch(timeout time.Duration) {
	step := timeout / 4
	if step < 5*time.Millisecond {
		step = 5 * time.Millisecond
	}
	t := time.NewTicker(step)
	defer t.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-t.C:
		}
		if c.busy.Load() > 0 {
			continue
		}
		if time.Since(c.LastActivity()) >= timeout {
			c.shutdown(ErrIdleTimeout)
			return
		}
	}
}

// abs makes a path absolute against the login folder.
func (c *Client) abs(p string) string {
	if p == "" {
		return c.home
	}
	if path.IsAbs(p) {
		return path.Clean(p)
	}
	return path.Join(c.home, p)
}

// entryFrom converts an fs.FileInfo from pkg/sftp into a remotefs.Entry.
func entryFrom(dir string, fi fs.FileInfo) remotefs.Entry {
	e := remotefs.Entry{
		Name:    fi.Name(),
		Path:    path.Join(dir, fi.Name()),
		Size:    fi.Size(),
		Mode:    fi.Mode(),
		ModTime: fi.ModTime(),
		IsDir:   fi.IsDir(),
		IsLink:  fi.Mode()&fs.ModeSymlink != 0,
	}
	if st, ok := fi.Sys().(*pkgsftp.FileStat); ok {
		e.UID, e.GID = st.UID, st.GID
		if st.Atime != 0 {
			e.ATime = time.Unix(int64(st.Atime), 0)
		}
	}
	return e
}

// ReadDir lists a folder. Symbolic links are resolved: LinkTarget holds the
// link text, IsDir tells whether the target is a folder (so the panel can
// enter it) and Size is the target's size; a dangling link has LinkBroken set.
func (c *Client) ReadDir(dir string) ([]remotefs.Entry, error) {
	end, err := c.begin()
	if err != nil {
		return nil, err
	}
	defer end()
	dir = c.abs(dir)
	infos, err := c.sftp.ReadDir(dir)
	if err != nil {
		return nil, pathErr("readdir", dir, err)
	}
	out := make([]remotefs.Entry, 0, len(infos))
	for _, fi := range infos {
		if fi.Name() == "." || fi.Name() == ".." {
			continue
		}
		e := entryFrom(dir, fi)
		if e.IsLink {
			c.resolveLink(&e)
		}
		out = append(out, e)
	}
	return out, nil
}

func (c *Client) resolveLink(e *remotefs.Entry) {
	if t, err := c.sftp.ReadLink(e.Path); err == nil {
		e.LinkTarget = t
	}
	if fi, err := c.sftp.Stat(e.Path); err == nil {
		e.IsDir = fi.IsDir()
		if !fi.IsDir() {
			e.Size = fi.Size()
		}
	} else {
		e.LinkBroken = true
	}
}

// Stat returns information about a path, following symbolic links.
func (c *Client) Stat(p string) (remotefs.Entry, error) {
	end, err := c.begin()
	if err != nil {
		return remotefs.Entry{}, err
	}
	defer end()
	p = c.abs(p)
	fi, err := c.sftp.Stat(p)
	if err != nil {
		return remotefs.Entry{}, pathErr("stat", p, err)
	}
	e := entryFrom(path.Dir(p), fi)
	e.Name, e.Path = path.Base(p), p
	return e, nil
}

// Lstat is Stat without following a final symbolic link; for a link the
// target text is filled in and IsDir tells whether it points to a folder.
// This backs the remote "Properties" dialog.
func (c *Client) Lstat(p string) (remotefs.Entry, error) {
	end, err := c.begin()
	if err != nil {
		return remotefs.Entry{}, err
	}
	defer end()
	p = c.abs(p)
	fi, err := c.sftp.Lstat(p)
	if err != nil {
		return remotefs.Entry{}, pathErr("lstat", p, err)
	}
	e := entryFrom(path.Dir(p), fi)
	e.Name, e.Path = path.Base(p), p
	if e.IsLink {
		c.resolveLink(&e)
	}
	return e, nil
}

// Exists reports whether a path exists (without following a final link).
func (c *Client) Exists(p string) (bool, error) {
	_, err := c.Lstat(p)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}

// RealPath asks the server to canonicalise a path (resolving "..", links
// and relative paths); used when the user types a path into the path bar.
func (c *Client) RealPath(p string) (string, error) {
	end, err := c.begin()
	if err != nil {
		return "", err
	}
	defer end()
	r, err := c.sftp.RealPath(c.abs(p))
	if err != nil {
		return "", pathErr("realpath", p, err)
	}
	return r, nil
}

// ReadLink returns the target text of a symbolic link.
func (c *Client) ReadLink(p string) (string, error) {
	end, err := c.begin()
	if err != nil {
		return "", err
	}
	defer end()
	p = c.abs(p)
	t, err := c.sftp.ReadLink(p)
	if err != nil {
		return "", pathErr("readlink", p, err)
	}
	return t, nil
}

// Mkdir creates one folder (F7).
func (c *Client) Mkdir(p string) error {
	end, err := c.begin()
	if err != nil {
		return err
	}
	defer end()
	p = c.abs(p)
	return pathErr("mkdir", p, c.sftp.Mkdir(p))
}

// MkdirAll creates a folder and any missing parents; existing folders are
// fine. Total Commander's F7 accepts "a/b/c" and creates the whole chain.
func (c *Client) MkdirAll(p string) error {
	end, err := c.begin()
	if err != nil {
		return err
	}
	defer end()
	p = c.abs(p)
	return pathErr("mkdir", p, c.sftp.MkdirAll(p))
}

// Remove deletes a file, a symbolic link (not its target) or an empty
// folder.
func (c *Client) Remove(p string) error {
	end, err := c.begin()
	if err != nil {
		return err
	}
	defer end()
	p = c.abs(p)
	fi, err := c.sftp.Lstat(p)
	if err != nil {
		return pathErr("remove", p, err)
	}
	if fi.IsDir() {
		return pathErr("remove", p, c.sftp.RemoveDirectory(p))
	}
	return pathErr("remove", p, c.sftp.Remove(p))
}

// RemoveAll deletes a path and, for a folder, everything below it (F8 on a
// folder). Links are deleted, never followed. onItem, when not nil, is called
// with each path just before it is deleted so the progress dialog can show
// it. The context is checked between items.
func (c *Client) RemoveAll(ctx context.Context, p string, onItem func(path string)) error {
	end, err := c.begin()
	if err != nil {
		return err
	}
	defer end()
	return c.removeAll(ctx, c.abs(p), onItem)
}

func (c *Client) removeAll(ctx context.Context, p string, onItem func(string)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	fi, err := c.sftp.Lstat(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return pathErr("remove", p, err)
	}
	if fi.IsDir() {
		children, err := c.sftp.ReadDir(p)
		if err != nil {
			return pathErr("readdir", p, err)
		}
		for _, ch := range children {
			if ch.Name() == "." || ch.Name() == ".." {
				continue
			}
			if err := c.removeAll(ctx, path.Join(p, ch.Name()), onItem); err != nil {
				return err
			}
			c.touch()
		}
		if onItem != nil {
			onItem(p)
		}
		return pathErr("rmdir", p, c.sftp.RemoveDirectory(p))
	}
	if onItem != nil {
		onItem(p)
	}
	return pathErr("remove", p, c.sftp.Remove(p))
}

// Rename renames or moves a path within the server (F6 / Shift+F6). It
// fails with an error matching fs.ErrExist when the destination exists, so
// the UI can ask "overwrite?" first; SFTP servers differ here (OpenSSH
// refuses, others silently replace), so the check is made on the client
// side. Use RenameOverwrite to replace the destination.
func (c *Client) Rename(oldPath, newPath string) error {
	end, err := c.begin()
	if err != nil {
		return err
	}
	defer end()
	oldPath, newPath = c.abs(oldPath), c.abs(newPath)
	if _, err := c.sftp.Lstat(newPath); err == nil && oldPath != newPath && !c.caseOnlyRename(oldPath, newPath) {
		return &os.LinkError{Op: "rename", Old: oldPath, New: newPath, Err: fs.ErrExist}
	}
	if err := c.sftp.Rename(oldPath, newPath); err != nil {
		return &os.LinkError{Op: "rename", Old: oldPath, New: newPath, Err: err}
	}
	return nil
}

// caseOnlyRename reports whether renaming oldPath to newPath only changes
// the letter case and newPath "exists" merely because the server's file
// system ignores case (Windows OpenSSH, macOS). Then Lstat(newPath) finds
// the old file itself, which must neither block the rename (Total
// Commander allows Shift+F6 to change only the case) nor be deleted as an
// "existing destination". The parent listing decides: when no entry has
// exactly the new name, the hit was the old file under another spelling.
func (c *Client) caseOnlyRename(oldPath, newPath string) bool {
	if oldPath == newPath || !strings.EqualFold(oldPath, newPath) {
		return false
	}
	list, err := c.sftp.ReadDir(path.Dir(newPath))
	if err != nil {
		return false
	}
	base := path.Base(newPath)
	for _, fi := range list {
		if fi.Name() == base {
			return false // a real, distinct file with exactly that name
		}
	}
	return true
}

// RenameOverwrite renames, replacing an existing destination file. It uses
// the atomic posix-rename@openssh.com extension when the server has it and
// otherwise deletes the destination first.
func (c *Client) RenameOverwrite(oldPath, newPath string) error {
	end, err := c.begin()
	if err != nil {
		return err
	}
	defer end()
	oldPath, newPath = c.abs(oldPath), c.abs(newPath)
	if _, ok := c.sftp.HasExtension("posix-rename@openssh.com"); ok {
		if err := c.sftp.PosixRename(oldPath, newPath); err != nil {
			return &os.LinkError{Op: "rename", Old: oldPath, New: newPath, Err: err}
		}
		return nil
	}
	if fi, err := c.sftp.Lstat(newPath); err == nil && !fi.IsDir() && !c.caseOnlyRename(oldPath, newPath) {
		if err := c.sftp.Remove(newPath); err != nil {
			return pathErr("remove", newPath, err)
		}
	}
	if err := c.sftp.Rename(oldPath, newPath); err != nil {
		return &os.LinkError{Op: "rename", Old: oldPath, New: newPath, Err: err}
	}
	return nil
}

// Chmod changes permission bits (Total Commander's "Change attributes" on a
// remote file, which shows the Unix rwx grid).
func (c *Client) Chmod(p string, mode fs.FileMode) error {
	end, err := c.begin()
	if err != nil {
		return err
	}
	defer end()
	p = c.abs(p)
	return pathErr("chmod", p, c.sftp.Chmod(p, mode))
}

// ChmodAll applies Chmod to a folder tree. fileMode is used for files and
// dirMode for folders (the usual 644/755 split); links are skipped because
// changing a link's mode would change its target.
func (c *Client) ChmodAll(ctx context.Context, root string, fileMode, dirMode fs.FileMode) error {
	return c.Walk(ctx, root, func(e remotefs.Entry, err error) error {
		if err != nil {
			return err
		}
		switch {
		case e.IsLink:
			return nil
		case e.IsDir:
			return c.Chmod(e.Path, dirMode)
		default:
			return c.Chmod(e.Path, fileMode)
		}
	})
}

// Chown changes the numeric owner and group.
func (c *Client) Chown(p string, uid, gid int) error {
	end, err := c.begin()
	if err != nil {
		return err
	}
	defer end()
	p = c.abs(p)
	return pathErr("chown", p, c.sftp.Chown(p, uid, gid))
}

// Chtimes sets access and modification times (used by "preserve date" on
// upload and by the Change Attributes dialog's date field).
func (c *Client) Chtimes(p string, atime, mtime time.Time) error {
	end, err := c.begin()
	if err != nil {
		return err
	}
	defer end()
	p = c.abs(p)
	return pathErr("chtimes", p, c.sftp.Chtimes(p, atime, mtime))
}

// Truncate changes a file's size.
func (c *Client) Truncate(p string, size int64) error {
	end, err := c.begin()
	if err != nil {
		return err
	}
	defer end()
	p = c.abs(p)
	return pathErr("truncate", p, c.sftp.Truncate(p, size))
}

// Symlink creates a symbolic link at linkPath pointing to target (Total
// Commander's Ctrl+Shift+F5 "create symbolic link" on a remote panel). The
// target is stored as given, so it may be relative to the link's folder.
func (c *Client) Symlink(target, linkPath string) error {
	end, err := c.begin()
	if err != nil {
		return err
	}
	defer end()
	linkPath = c.abs(linkPath)
	if err := c.sftp.Symlink(target, linkPath); err != nil {
		return &os.LinkError{Op: "symlink", Old: target, New: linkPath, Err: err}
	}
	return nil
}

// Link creates a hard link (needs the hardlink@openssh.com extension).
func (c *Client) Link(oldPath, newPath string) error {
	end, err := c.begin()
	if err != nil {
		return err
	}
	defer end()
	oldPath, newPath = c.abs(oldPath), c.abs(newPath)
	if err := c.sftp.Link(oldPath, newPath); err != nil {
		return &os.LinkError{Op: "link", Old: oldPath, New: newPath, Err: err}
	}
	return nil
}

// SpaceInfo is the free space of the remote file system that holds a path,
// shown in the panel's status line like a local drive's free space.
type SpaceInfo struct {
	Total     uint64 // bytes
	Free      uint64 // bytes free for root
	Available uint64 // bytes free for the user
}

// Space queries free space using the statvfs@openssh.com extension. Servers
// without it return an error, and the panel then simply shows no free space.
func (c *Client) Space(p string) (SpaceInfo, error) {
	end, err := c.begin()
	if err != nil {
		return SpaceInfo{}, err
	}
	defer end()
	p = c.abs(p)
	if _, ok := c.sftp.HasExtension("statvfs@openssh.com"); !ok {
		return SpaceInfo{}, fmt.Errorf("sftp: server cannot report free space")
	}
	st, err := c.sftp.StatVFS(p)
	if err != nil {
		return SpaceInfo{}, pathErr("statvfs", p, err)
	}
	return SpaceInfo{
		Total:     st.TotalSpace(),
		Free:      st.FreeSpace(),
		Available: st.Bavail * st.Frsize,
	}, nil
}

// Open opens a remote file for reading. The returned value also implements
// io.Seeker and io.ReaderAt, so the Lister can page through a large remote
// file without downloading it all.
func (c *Client) Open(p string) (io.ReadCloser, error) {
	end, err := c.begin()
	if err != nil {
		return nil, err
	}
	defer end()
	p = c.abs(p)
	f, err := c.sftp.Open(p)
	if err != nil {
		return nil, pathErr("open", p, err)
	}
	return f, nil
}

// Create creates or truncates a remote file for writing.
func (c *Client) Create(p string) (io.WriteCloser, error) {
	end, err := c.begin()
	if err != nil {
		return nil, err
	}
	defer end()
	p = c.abs(p)
	f, err := c.sftp.Create(p)
	if err != nil {
		return nil, pathErr("create", p, err)
	}
	return f, nil
}

// CommandResult is the outcome of RunCommand.
type CommandResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int // -1 when the command ended without an exit status
}

// RunCommand runs a shell command on the server in its own SSH session, like
// Total Commander's "send command to server" (the FTP "quote" equivalent on
// SFTP connections). A non-zero exit code is not an error; it is reported in
// the result. Cancelling the context closes the session.
func (c *Client) RunCommand(ctx context.Context, cmd string) (CommandResult, error) {
	end, err := c.beginAny()
	if err != nil {
		return CommandResult{}, err
	}
	defer end()
	s, err := c.conn.NewSession()
	if err != nil {
		return CommandResult{}, fmt.Errorf("sftp: opening session: %w", err)
	}
	defer s.Close()
	var out, errb bytes.Buffer
	s.Stdout, s.Stderr = &out, &errb
	res := make(chan error, 1)
	go func() { res <- s.Run(cmd) }()
	select {
	case <-ctx.Done():
		s.Signal(ssh.SIGTERM)
		s.Close()
		return CommandResult{}, ctx.Err()
	case err = <-res:
	}
	r := CommandResult{Stdout: out.Bytes(), Stderr: errb.Bytes()}
	var exit *ssh.ExitError
	var missing *ssh.ExitMissingError
	switch {
	case err == nil:
		r.ExitCode = 0
	case errors.As(err, &exit):
		r.ExitCode = exit.ExitStatus()
	case errors.As(err, &missing):
		r.ExitCode = -1
	default:
		return r, fmt.Errorf("sftp: running command: %w", err)
	}
	return r, nil
}

// pathErr wraps an error with the operation and path while keeping
// errors.Is(err, fs.ErrNotExist) and friends working. A nil error stays nil.
func pathErr(op, p string, err error) error {
	if err == nil {
		return nil
	}
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return err
	}
	return &fs.PathError{Op: op, Path: p, Err: err}
}
