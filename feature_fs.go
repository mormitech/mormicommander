package main

// Files and folders for the panels: listing, volumes, free space, opening a file with its app. Everything goes through
// one vfs.Router (internal/vfs/router.go), so a panel shows a local folder, the inside of an archive
// (/x/backup.zip/docs, nested archives too) or an FTP/SFTP folder the same way: Enter on an archive lists it, ".."
// leaves it. The demo (--demo) uses the fictional vfs.Mock instead. Other features reach the router with vfsRouter(a)
// (for example the connection manager, to set Router.FTP.Configure); archive jobs live in feature_vfs_jobs.go.

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"mormicommander/internal/archive"
	"mormicommander/internal/archive/sevenrar"
	"mormicommander/internal/disk"
	"mormicommander/internal/hang"
	"mormicommander/internal/vfs"
)

// FS is what the page calls for files and folders (window.go.main.FS).
type FS struct {
	app    *App
	router *vfs.Router // nil in the demo
	mu     sync.Mutex
	gen    map[string]listing // the listing in progress per panel ("left", "right", or any key)
	seq    int
	stuck  hang.Guard // listings still running per folder, so a dead share can't pile up blocked reads
}

type listing struct {
	id     int
	cancel context.CancelFunc
}

func init() {
	registerFeature("fs", func(a *App) []exposed { return []exposed{expose(newFS(a))} })
	onShutdown(func(a *App) {
		if f := fsOf(a); f != nil && f.router != nil {
			f.router.Close() // disconnects servers, closes archives, removes temp copies
		}
	})
}

var fsByApp sync.Map // *App → *FS

func newFS(a *App) *FS {
	f := &FS{app: a, gen: map[string]listing{}}
	if a.mock == nil {
		r := vfs.NewRouter()
		r.Archives.Password = func(req vfs.PasswordRequest) (string, bool) {
			return vfsAskPassword(a, req.Archive, req.Entry, req.Attempt)
		}
		// The panels' router also answers vfs.For for server URLs, so the connection layer (feature_net.go) puts its
		// Configure hooks (password, saved-site settings, transfer mode, log) on the sessions the panels use.
		r.RegisterSchemes()
		f.router = r
	}
	fsByApp.Store(a, f)
	return f
}

// fsOf is the FS object of an app (nil before the features were bound).
func fsOf(a *App) *FS {
	if v, ok := fsByApp.Load(a); ok {
		return v.(*FS)
	}
	return nil
}

// vfsRouter is the router every panel path goes through (nil in the demo), for features that need archives or servers.
func vfsRouter(a *App) *vfs.Router {
	if f := fsOf(a); f != nil {
		return f.router
	}
	return nil
}

func (f *FS) fsFor(p string) vfs.FS {
	if f.router != nil {
		return f.router
	}
	return vfs.For(p, f.app.fs)
}

// List reads a folder for a panel. key names the panel: a new listing for the same key cancels the one before (a fast
// typist going through folders never waits for a slow one). The answer has the entries as compact rows (see vfs.Rows).
func (f *FS) List(key, path string) (vfs.Listing, error) {
	ctx, cancel := context.WithCancel(context.Background())
	f.mu.Lock()
	if old, ok := f.gen[key]; ok {
		old.cancel()
	}
	f.seq++
	id := f.seq
	f.gen[key] = listing{id, cancel}
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		cancel()
		if f.gen[key].id == id { // (a newer listing for the panel keeps its entry)
			delete(f.gen, key)
		}
		f.mu.Unlock()
	}()
	fsys := f.fsFor(path)
	// The read runs in a goroutine of its own (internal/hang): on a share whose server is gone the system call can block
	// for minutes, and Esc (CancelList) or a newer listing must still bring the panel back at once. At most a few
	// reads of the same folder are left hanging; past that the answer is "not responding" right away.
	l, err := hang.Guarded(&f.stuck, ctx, fsys.Clean(path), func(ctx context.Context) (vfs.Listing, error) {
		return fsys.List(ctx, path)
	})
	if err != nil {
		return vfs.Listing{}, errors.New(listError(fsys.Clean(path), err))
	}
	l.Rows = vfs.Rows(l.Entries)
	return l, nil
}

// CancelList stops the listing in progress for a panel (Esc while a slow folder is read).
func (f *FS) CancelList(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if l, ok := f.gen[key]; ok {
		l.cancel()
	}
}

// listError says in plain words why a folder can't be shown.
func listError(p string, err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "Stopped reading " + p + "."
	case errors.Is(err, hang.ErrNotResponding):
		return p + " is not responding (a network share whose server is gone, or a disk that was removed?)."
	case errors.Is(err, vfs.ErrNotFolder):
		return p + " is a file, not a folder."
	case errors.Is(err, fs.ErrNotExist):
		return "The folder " + p + " is not there."
	case errors.Is(err, archive.ErrBadPassword), errors.Is(err, sevenrar.ErrWrongPassword):
		return "The password for " + p + " is wrong."
	case errors.Is(err, archive.ErrCanceled), errors.Is(err, sevenrar.ErrPasswordCancelled):
		return p + " needs a password to be opened."
	case errors.Is(err, archive.ErrPasswordRequired), errors.Is(err, sevenrar.ErrPasswordRequired):
		return p + " needs a password to be opened."
	case errors.Is(err, archive.ErrMissingVolume), errors.Is(err, sevenrar.ErrMissingVolume):
		return "A part of the multi-volume archive " + p + " is missing."
	case errors.Is(err, archive.ErrFormat):
		return p + " is not a valid archive, or it is damaged."
	case errors.Is(err, vfs.ErrUnsupported):
		return "mormiCommander can't open " + p + ": this kind of address is not supported."
	case errors.Is(err, fs.ErrPermission):
		msg := "You don't have permission to open " + p + "."
		if runtime.GOOS == "darwin" {
			msg += " macOS may need to allow mormiCommander in System Settings > Privacy & Security > Files and Folders (or Full Disk Access)."
		}
		return msg
	}
	return fmt.Sprintf("Can't open %s: %v", p, err)
}

// Entry is one file as Stat returns it.
type Entry struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	MTime  int64  `json:"mtime"`
	Flags  int    `json:"flags"`
	Mode   uint32 `json:"mode"`
	Target string `json:"target"` // where a link points ("" for no link)
}

// Stat describes one file or folder.
func (f *FS) Stat(path string) (Entry, error) {
	fsys := f.fsFor(path)
	p := fsys.Clean(path)
	e, err := fsys.Stat(p)
	if err != nil {
		return Entry{}, errors.New(listError(p, err))
	}
	return Entry{Name: e.Name, Path: p, Size: e.Size, MTime: e.MTime, Flags: e.Flags, Mode: e.Mode, Target: f.LinkTarget(p)}, nil
}

// LinkTarget is the full path a symbolic link points to ("" when path is no link).
func (f *FS) LinkTarget(path string) string {
	if f.app.mock != nil {
		return f.app.mock.LinkTarget(path)
	}
	if f.router != nil && f.router.Kind(path) != vfs.KindLocal {
		return "" // links inside archives and on servers are shown, not followed by path
	}
	t, err := os.Readlink(path)
	if err != nil {
		return ""
	}
	if !filepath.IsAbs(t) {
		t = filepath.Join(filepath.Dir(path), t)
	}
	return filepath.Clean(t)
}

// Clean makes a path canonical ("~" is the home folder).
func (f *FS) Clean(path string) string { return f.fsFor(path).Clean(path) }

// Parent is the folder above path ("" at a root).
func (f *FS) Parent(path string) string { return f.fsFor(path).Parent(path) }

// Root is the top of the volume path is on.
func (f *FS) Root(path string) string { return f.fsFor(path).Root(path) }

// Join puts a name under a folder, with the system's separator.
func (f *FS) Join(dir, name string) string { return f.fsFor(dir).Join(dir, name) }

// Home is the user's home folder (the demo's fictional one in --demo).
func (f *FS) Home() string {
	if f.app.mock != nil {
		return vfs.MockHome
	}
	h, _ := os.UserHomeDir()
	return h
}

// Separator is the path separator the panels show: "\" on Windows, "/" elsewhere (and in the demo).
func (f *FS) Separator() string {
	if f.app.mock != nil {
		return "/"
	}
	return string(filepath.Separator)
}

// Volumes lists the drives and volumes for the drive bar and the drive popup.
func (f *FS) Volumes() []disk.Volume {
	if f.app.mock != nil {
		return mockVolumes
	}
	return disk.Volumes()
}

var mockVolumes = []disk.Volume{
	{Name: "Macintosh HD", Path: "/", Kind: "fixed", Total: 994_662_584_320, Free: 412_337_123_328},
	{Name: "Home", Path: vfs.MockHome, Kind: "home", Total: 994_662_584_320, Free: 412_337_123_328},
	{Name: "Backup", Path: "/Volumes/Backup", Kind: "removable", Total: 2_000_398_934_016, Free: 1_210_204_430_336},
	{Name: "USB stick", Path: "/Volumes/USB stick", Kind: "removable", Total: 63_999_836_160, Free: 51_330_000_896},
}

// VolumeInfo is the volume a folder is on, for the drive info line above a panel.
type VolumeInfo struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Total uint64 `json:"total"`
	Free  uint64 `json:"free"`
}

// VolumeOf tells which volume path is on, with its free space: the volume with the longest mount path that contains it.
func (f *FS) VolumeOf(path string) VolumeInfo {
	p := f.Clean(path)
	vols := append([]disk.Volume(nil), f.Volumes()...) // a copy: sorting must not change the order of the drive bar
	sort.Slice(vols, func(i, j int) bool { return len(vols[i].Path) > len(vols[j].Path) })
	for _, v := range vols {
		if v.Kind == "home" {
			continue // the home folder is a shortcut in the list, not a volume of its own
		}
		if pathWithin(p, v.Path) {
			info := VolumeInfo{Name: v.Name, Path: v.Path, Total: v.Total, Free: v.Free}
			if f.app.mock == nil {
				if u, err := disk.UsageOfQuick(p); err == nil {
					info.Total, info.Free = u.Total, u.Free
				}
			}
			return info
		}
	}
	info := VolumeInfo{Path: f.Root(p)}
	if f.app.mock == nil {
		if u, err := disk.UsageOfQuick(p); err == nil {
			info.Total, info.Free = u.Total, u.Free
		}
	}
	return info
}

// pathWithin says whether p is root or under it (case-insensitive on Windows drive letters).
func pathWithin(p, root string) bool {
	if runtime.GOOS == "windows" {
		p, root = strings.ToLower(p), strings.ToLower(root)
	}
	if p == root {
		return true
	}
	if !strings.HasSuffix(root, "/") && !strings.HasSuffix(root, `\`) {
		root += string(filepath.Separator)
		if strings.HasPrefix(root, "/") {
			root = strings.TrimSuffix(root, string(filepath.Separator)) + "/"
		}
	}
	return strings.HasPrefix(p, root)
}

// OpenFile opens a file with the app the system uses for it (Enter on a file). A file inside an archive or on a server
// is unpacked or downloaded to a temporary copy first. In the demo nothing is opened.
func (f *FS) OpenFile(path string) error {
	if f.app.mock != nil {
		return errors.New("This is the demo: its files are not real, so they can't be opened.")
	}
	if f.router != nil && !vfsIsLocal(f.router, path) {
		local, err := f.LocalCopy(path)
		if err != nil {
			return err
		}
		path = local
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "windows":
		// not "cmd /c start", which reads & and ^ in the path as its own syntax
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	hideWindow(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("Can't open %s: %v", filepath.Base(path), err)
	}
	go cmd.Wait()
	return nil
}
