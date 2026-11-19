package main

// Cloud and object-storage remotes in a panel through the user's own rclone (issue #18, F-866): rclone://remote/path
// shows the folder path of a remote defined in the user's rclone.conf (S3, R2, B2, Azure, GCS, WebDAV, Drive, Dropbox,
// OneDrive, SMB and every other rclone backend). netRclone implements vfs.FS and the optional operations on top of
// internal/rclone, which runs the rclone program as a subprocess (lsjson, cat, rcat, copyto, moveto, deletefile,
// rmdir, purge, mkdir), and is registered with vfs.Register for "rclone", so the panels, the router and the network
// transfers (feature_net_xfer.go) treat a remote like a WebDAV share. Copies between two remotes are one rclone copyto,
// which copies on the server when both are on the same provider.
//
// Nothing runs by itself: rclone is found (Options > FTP > "rclone program", else on PATH) and started only when the
// user lists the remotes (Net > Cloud remotes (rclone)…, which only reads rclone's config) or opens one (that list,
// quick connect or the path bar). Only List and Connect, the user opening a folder, may open a remote; every other call
// on a remote that is not open fails without starting rclone. rclone's configuration is never written: rclone reads
// its own config file as usual, and --ask-password=false keeps it from waiting for a config password.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"mormicommander/internal/launch"
	"mormicommander/internal/rclone"
	"mormicommander/internal/vfs"
)

// rcloneCacheFor is how long a listing answers Stat calls for its entries (each rclone call is a new process).
const rcloneCacheFor = 5 * time.Second

// netRclone is the file system of rclone remotes.
type netRclone struct {
	program func() string // the configured rclone program ("" = look on PATH)
	env     []string      // extra environment for rclone (tests)

	mu     sync.Mutex
	open   map[string]*rclone.Client // remote name → client, for the remotes the user opened
	cache  map[string]rcloneDir      // panel URL of a folder → its last listing
	gotten map[string]bool           // folders listed by Connect, so the panel's first List needs no second call
}

type rcloneDir struct {
	at      time.Time
	entries map[string]vfs.Entry
	list    []vfs.Entry
}

func newNetRclone(program func() string) *netRclone {
	return &netRclone{program: program, open: map[string]*rclone.Client{}, cache: map[string]rcloneDir{}, gotten: map[string]bool{}}
}

// rcloneProgram is Options > FTP > "rclone program".
func rcloneProgram() string {
	s, ok := currentSettings()
	if !ok {
		return ""
	}
	return s.FTP.RclonePath
}

// newClient finds rclone and makes a client for it.
func (r *netRclone) newClient() (*rclone.Client, error) {
	prog := ""
	if r.program != nil {
		prog = r.program()
	}
	bin, err := rclone.Find(prog)
	if err != nil {
		return nil, err
	}
	return &rclone.Client{Bin: bin, Env: r.env, Prepare: launch.Hide}, nil
}

// parse splits a panel URL into remote name and path ("/" at the remote's root).
func (r *netRclone) parse(p string) (string, string, error) {
	u, err := netParseURL(p)
	if err != nil {
		return "", "", err
	}
	if u.Scheme != "rclone" {
		return "", "", errors.New("not an rclone address: " + p)
	}
	rp := u.Path
	if rp == "" {
		rp = "/"
	}
	return u.Host, rp, nil
}

func rcloneURL(remote, rp string) string {
	return netURL{Scheme: "rclone", Host: remote, Path: path.Clean("/" + rp)}.String()
}

// client returns the client of an open remote; open lets a user's List or Connect open it.
func (r *netRclone) client(remote string, open bool) (*rclone.Client, error) {
	r.mu.Lock()
	c := r.open[remote]
	r.mu.Unlock()
	if c != nil {
		return c, nil
	}
	if !open {
		return nil, fmt.Errorf("the rclone remote %s is not open: open it in a panel first (Net > Cloud remotes)", remote)
	}
	c, err := r.newClient()
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	if old := r.open[remote]; old != nil {
		c = old
	} else {
		r.open[remote] = c
	}
	r.mu.Unlock()
	return c, nil
}

// forget drops the cached listings (after any change).
func (r *netRclone) forget() {
	r.mu.Lock()
	r.cache = map[string]rcloneDir{}
	r.gotten = map[string]bool{}
	r.mu.Unlock()
}

func rcloneEntry(it rclone.Item) vfs.Entry {
	name := it.Name
	if name == "" {
		name = path.Base(it.Path)
	}
	e := vfs.Entry{Name: name}
	if !it.ModTime.IsZero() && it.ModTime.Year() > 1970 {
		e.MTime = it.ModTime.UnixMilli()
	}
	if it.IsDir {
		e.Flags |= vfs.FlagDir
		e.Mode = uint32(fs.ModeDir)
	} else if it.Size > 0 {
		e.Size = it.Size
	}
	if strings.HasPrefix(name, ".") {
		e.Flags |= vfs.FlagHidden
	}
	return e
}

// Connect opens the remote of u (the user chose it) and returns the panel path to show: the folder, or the folder of
// a file.
func (r *netRclone) Connect(ctx context.Context, u netURL) (string, error) {
	if !rclone.ValidRemote(u.Host) {
		return "", fmt.Errorf("%q is not the name of an rclone remote.", u.Host)
	}
	c, err := r.client(u.Host, true)
	if err != nil {
		return "", err
	}
	rp := u.Path
	if rp == "" {
		rp = "/"
	}
	if rp != "/" {
		it, err := c.Stat(ctx, u.Host, rp)
		if err != nil {
			r.drop(u.Host)
			return "", err
		}
		if !it.IsDir {
			rp = path.Dir(rp)
		}
	}
	// one listing checks that rclone can reach the remote; the panel's first List uses it
	lp := rcloneURL(u.Host, rp)
	if _, err := r.list(ctx, c, u.Host, rp, lp); err != nil {
		r.drop(u.Host)
		return "", err
	}
	r.mu.Lock()
	r.gotten[lp] = true
	r.mu.Unlock()
	return lp, nil
}

// drop closes a remote that failed to open.
func (r *netRclone) drop(remote string) {
	r.mu.Lock()
	delete(r.open, remote)
	r.mu.Unlock()
}

// Disconnect closes the remote of a session key ("rclone://name").
func (r *netRclone) Disconnect(key string) {
	remote, _, err := r.parse(key)
	if err != nil {
		return
	}
	r.drop(remote)
	r.forget()
}

// Sessions lists the open remotes as panel URLs of their roots.
func (r *netRclone) Sessions() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.open))
	for name := range r.open {
		out = append(out, rcloneURL(name, "/"))
	}
	sort.Strings(out)
	return out
}

// Close forgets every remote.
func (r *netRclone) Close() error {
	r.mu.Lock()
	r.open = map[string]*rclone.Client{}
	r.mu.Unlock()
	r.forget()
	return nil
}

// Remotes lists the remotes of the user's rclone.conf (rclone listremotes reads the config only).
func (r *netRclone) Remotes(ctx context.Context) ([]rclone.Remote, error) {
	c, err := r.newClient()
	if err != nil {
		return nil, err
	}
	return c.Remotes(ctx)
}

func (r *netRclone) list(ctx context.Context, c *rclone.Client, remote, rp, lp string) ([]vfs.Entry, error) {
	items, err := c.List(ctx, remote, rp)
	if err != nil {
		return nil, err
	}
	// rclone lists a file as itself: tell that from a folder holding one entry of the same name
	if len(items) == 1 && !items[0].IsDir && rp != "/" && items[0].Path == path.Base(rp) {
		if it, err := c.Stat(ctx, remote, rp); err == nil && !it.IsDir {
			return nil, &fs.PathError{Op: "list", Path: lp, Err: vfs.ErrNotFolder}
		}
	}
	out := make([]vfs.Entry, 0, len(items))
	m := make(map[string]vfs.Entry, len(items))
	for _, it := range items {
		e := rcloneEntry(it)
		if !netPlainName(e.Name) {
			continue
		}
		out = append(out, e)
		m[e.Name] = e
	}
	r.mu.Lock()
	r.cache[lp] = rcloneDir{at: time.Now(), entries: m, list: out}
	r.mu.Unlock()
	return out, nil
}

// List reads a folder of a remote (opening the remote: the user went there).
func (r *netRclone) List(ctx context.Context, p string) (vfs.Listing, error) {
	remote, rp, err := r.parse(p)
	if err != nil {
		return vfs.Listing{}, err
	}
	if !rclone.ValidRemote(remote) {
		return vfs.Listing{}, fmt.Errorf("%q is not the name of an rclone remote.", remote)
	}
	c, err := r.client(remote, true)
	if err != nil {
		return vfs.Listing{}, err
	}
	lp := rcloneURL(remote, rp)
	r.mu.Lock()
	d, ok := r.cache[lp]
	fresh := ok && r.gotten[lp] && time.Since(d.at) < rcloneCacheFor
	delete(r.gotten, lp)
	r.mu.Unlock()
	var es []vfs.Entry
	if fresh {
		es = append([]vfs.Entry(nil), d.list...)
	} else if es, err = r.list(ctx, c, remote, rp, lp); err != nil {
		return vfs.Listing{}, err
	}
	if err := ctx.Err(); err != nil {
		return vfs.Listing{}, err
	}
	return vfs.Listing{Path: lp, Parent: r.Parent(lp), Entries: es}, nil
}

// Stat describes one path of an open remote; a fresh listing of its folder answers without starting rclone.
func (r *netRclone) Stat(p string) (vfs.Entry, error) {
	remote, rp, err := r.parse(p)
	if err != nil {
		return vfs.Entry{}, err
	}
	if rp == "/" {
		return vfs.Entry{Name: "/", Flags: vfs.FlagDir, Mode: uint32(fs.ModeDir)}, nil
	}
	c, err := r.client(remote, false)
	if err != nil {
		return vfs.Entry{}, err
	}
	r.mu.Lock()
	d, ok := r.cache[rcloneURL(remote, path.Dir(rp))]
	r.mu.Unlock()
	if ok && time.Since(d.at) < rcloneCacheFor {
		if e, ok := d.entries[path.Base(rp)]; ok {
			return e, nil
		}
		return vfs.Entry{}, &fs.PathError{Op: "stat", Path: p, Err: fs.ErrNotExist}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	it, err := c.Stat(ctx, remote, rp)
	if err != nil {
		return vfs.Entry{}, err
	}
	e := rcloneEntry(it)
	e.Name = path.Base(rp)
	return e, nil
}

// Clean makes a remote URL canonical.
func (r *netRclone) Clean(p string) string {
	u, err := netParseURL(p)
	if err != nil {
		return p
	}
	return u.String()
}

// Parent is the folder above p, "" at the remote's root.
func (r *netRclone) Parent(p string) string {
	u, err := netParseURL(p)
	if err != nil || u.Path == "" || u.Path == "/" {
		return ""
	}
	u.Path = path.Dir(u.Path)
	return u.String()
}

// Root is the root of p's remote.
func (r *netRclone) Root(p string) string {
	u, err := netParseURL(p)
	if err != nil {
		return p
	}
	u.Path = "/"
	return u.String()
}

// Join puts a name under a folder of a remote.
func (r *netRclone) Join(dir, name string) string {
	u, err := netParseURL(dir)
	if err != nil {
		return strings.TrimSuffix(dir, "/") + "/" + name
	}
	p := u.Path
	if p == "" {
		p = "/"
	}
	u.Path = path.Join(p, name)
	return u.String()
}

// Caps tells what works on a remote: everything but permissions and setting times.
func (r *netRclone) Caps(string) vfs.Caps {
	return vfs.Caps{Read: true, Write: true, Mkdir: true, Delete: true, Rename: true, Remote: true}
}

// Open reads a file of a remote (rclone cat).
func (r *netRclone) Open(ctx context.Context, p string) (io.ReadCloser, error) {
	remote, rp, err := r.parse(p)
	if err != nil {
		return nil, err
	}
	c, err := r.client(remote, false)
	if err != nil {
		return nil, err
	}
	return c.Cat(ctx, remote, rp)
}

type rcloneWriter struct {
	io.WriteCloser
	r *netRclone
}

func (w rcloneWriter) Close() error {
	err := w.WriteCloser.Close()
	w.r.forget()
	return err
}

// Create writes a file of a remote (rclone rcat; complete when closed).
func (r *netRclone) Create(ctx context.Context, p string) (io.WriteCloser, error) {
	remote, rp, err := r.parse(p)
	if err != nil {
		return nil, err
	}
	c, err := r.client(remote, false)
	if err != nil {
		return nil, err
	}
	r.forget()
	w, err := c.Rcat(ctx, remote, rp)
	if err != nil {
		return nil, err
	}
	return rcloneWriter{w, r}, nil
}

// Mkdir makes a folder (and the folders above it); an existing one is fs.ErrExist.
func (r *netRclone) Mkdir(ctx context.Context, p string) error {
	remote, rp, err := r.parse(p)
	if err != nil {
		return err
	}
	c, err := r.client(remote, false)
	if err != nil {
		return err
	}
	if _, err := r.Stat(p); err == nil {
		return &fs.PathError{Op: "mkdir", Path: p, Err: fs.ErrExist}
	}
	defer r.forget()
	return c.Mkdir(ctx, remote, rp)
}

func (r *netRclone) remove(ctx context.Context, p string, all bool) error {
	remote, rp, err := r.parse(p)
	if err != nil {
		return err
	}
	if rp == "/" {
		return &fs.PathError{Op: "remove", Path: p, Err: errors.New("the root of a remote can't be deleted")}
	}
	c, err := r.client(remote, false)
	if err != nil {
		return err
	}
	e, err := r.Stat(p)
	if err != nil {
		return err
	}
	defer r.forget()
	switch {
	case e.Flags&vfs.FlagDir == 0:
		return c.DeleteFile(ctx, remote, rp)
	case all:
		return c.Purge(ctx, remote, rp)
	default:
		return c.Rmdir(ctx, remote, rp)
	}
}

// Remove deletes a file or an empty folder.
func (r *netRclone) Remove(ctx context.Context, p string) error { return r.remove(ctx, p, false) }

// RemoveAll deletes a file or a folder with everything in it (rclone purge).
func (r *netRclone) RemoveAll(ctx context.Context, p string) error { return r.remove(ctx, p, true) }

// Rename moves a file or folder between two remote paths (rclone moveto: on the server when the provider can); an
// existing target is fs.ErrExist.
func (r *netRclone) Rename(ctx context.Context, from, to string) error {
	r1, a, err := r.parse(from)
	if err != nil {
		return err
	}
	r2, b, err := r.parse(to)
	if err != nil {
		return &fs.PathError{Op: "rename", Path: from, Err: vfs.ErrCrossFS}
	}
	c, err := r.client(r1, false)
	if err != nil {
		return err
	}
	if _, err := r.client(r2, false); err != nil {
		return err
	}
	if _, err := r.Stat(to); err == nil {
		return &fs.PathError{Op: "rename", Path: to, Err: fs.ErrExist}
	}
	defer r.forget()
	return c.MoveTo(ctx, rclone.Arg(r1, a), rclone.Arg(r2, b))
}

// Download copies a file of a remote to the local disk (rclone copyto keeps the modification time).
func (r *netRclone) Download(ctx context.Context, p, local string, opt vfs.TransferOptions) (int64, error) {
	remote, rp, err := r.parse(p)
	if err != nil {
		return 0, err
	}
	c, err := r.client(remote, false)
	if err != nil {
		return 0, err
	}
	e, _ := r.Stat(p)
	if err := c.CopyTo(ctx, rclone.Arg(remote, rp), local, opt.Progress); err != nil {
		return 0, err
	}
	return e.Size, nil
}

// Upload copies a local file to a remote.
func (r *netRclone) Upload(ctx context.Context, local, p string, opt vfs.TransferOptions) (int64, error) {
	remote, rp, err := r.parse(p)
	if err != nil {
		return 0, err
	}
	c, err := r.client(remote, false)
	if err != nil {
		return 0, err
	}
	defer r.forget()
	if err := c.CopyTo(ctx, local, rclone.Arg(remote, rp), opt.Progress); err != nil {
		return 0, err
	}
	e, _ := r.Stat(p)
	return e.Size, nil
}

// CopyRemote copies a file from one remote path to another with one rclone copyto, which copies on the server when
// both are on the same provider.
func (r *netRclone) CopyRemote(ctx context.Context, src, dst string, opt vfs.TransferOptions) error {
	r1, a, err := r.parse(src)
	if err != nil {
		return err
	}
	r2, b, err := r.parse(dst)
	if err != nil {
		return err
	}
	c, err := r.client(r1, false)
	if err != nil {
		return err
	}
	if _, err := r.client(r2, false); err != nil {
		return err
	}
	defer r.forget()
	return c.CopyTo(ctx, rclone.Arg(r1, a), rclone.Arg(r2, b), opt.Progress)
}

// Chtimes is not offered (rclone copies keep the source's time).
func (r *netRclone) Chtimes(ctx context.Context, p string, mtime time.Time) error {
	return &fs.PathError{Op: "chtimes", Path: p, Err: vfs.ErrUnsupported}
}

// Chmod is not possible on cloud remotes.
func (r *netRclone) Chmod(ctx context.Context, p string, mode fs.FileMode) error {
	return &fs.PathError{Op: "chmod", Path: p, Err: vfs.ErrUnsupported}
}

// NetRcloneRemote is one remote of the user's rclone.conf, for the page.
type NetRcloneRemote struct {
	Name string `json:"name"`
	Type string `json:"type"`
	URL  string `json:"url"` // the panel address of its root
}

// RcloneRemotes lists the remotes of the user's rclone.conf (Net > Cloud remotes (rclone)…). It runs rclone
// listremotes, which reads rclone's config file only and goes nowhere.
func (n *Net) RcloneRemotes() ([]NetRcloneRemote, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	rs, err := n.rclone.Remotes(ctx)
	if err != nil {
		return nil, rcloneErr(err)
	}
	out := make([]NetRcloneRemote, 0, len(rs))
	for _, r := range rs {
		out = append(out, NetRcloneRemote{Name: r.Name, Type: r.Type, URL: rcloneURL(r.Name, "/")})
	}
	return out, nil
}

// rcloneErr says in plain words why rclone could not be used.
func rcloneErr(err error) error {
	if errors.Is(err, rclone.ErrNotFound) {
		return errors.New("rclone was not found. Install rclone (rclone.org) and set up your remotes with \"rclone config\", or set the rclone program in Options > FTP.")
	}
	return err
}

// rcloneConnectErr says in plain words why a remote could not be opened.
func rcloneConnectErr(u netURL, err error) error {
	err = rcloneErr(err)
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("The rclone remote %s did not answer in time.", u.Host)
	}
	msg := strings.TrimPrefix(err.Error(), "rclone: ")
	if strings.Contains(msg, "didn't find section in config file") {
		return fmt.Errorf("There is no remote %s in your rclone configuration (rclone config).", u.Host)
	}
	return fmt.Errorf("Can't open the rclone remote %s: %s", u.Host, msg)
}
