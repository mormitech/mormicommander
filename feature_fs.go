package main

// Files and folders for the panels: listing, volumes, free space, opening a file with its app. Everything goes through
// a vfs.FS, so the demo's fictional files (--demo) and future archive or FTP file systems work the same way.

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

	"mormicommander/internal/disk"
	"mormicommander/internal/vfs"
)

// FS is what the page calls for files and folders (window.go.main.FS).
type FS struct {
	app *App
	mu  sync.Mutex
	gen map[string]listing // the listing in progress per panel ("left", "right", or any key)
	seq int
}

type listing struct {
	id     int
	cancel context.CancelFunc
}

func init() {
	registerFeature("fs", func(a *App) []any { return []any{newFS(a)} })
}

func newFS(a *App) *FS { return &FS{app: a, gen: map[string]listing{}} }

func (f *FS) fsFor(p string) vfs.FS { return vfs.For(p, f.app.fs) }

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
	l, err := fsys.List(ctx, path)
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
	case errors.Is(err, vfs.ErrNotFolder):
		return p + " is a file, not a folder."
	case errors.Is(err, fs.ErrNotExist):
		return "The folder " + p + " is not there."
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
				if u, err := disk.UsageOf(p); err == nil {
					info.Total, info.Free = u.Total, u.Free
				}
			}
			return info
		}
	}
	info := VolumeInfo{Path: f.Root(p)}
	if f.app.mock == nil {
		if u, err := disk.UsageOf(p); err == nil {
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

// OpenFile opens a file with the app the system uses for it (Enter on a file). In the demo nothing is opened.
func (f *FS) OpenFile(path string) error {
	if f.app.mock != nil {
		return errors.New("This is the demo: its files are not real, so they can't be opened.")
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
