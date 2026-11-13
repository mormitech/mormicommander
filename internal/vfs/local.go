package vfs

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// Local is the real file system of this computer.
type Local struct{}

// List reads a folder. Big folders are read by several goroutines at once: the names come in one call, but the size,
// date and type of each file is one more system call per file, and those run in parallel well on SSDs and network
// shares alike.
func (Local) List(ctx context.Context, p string) (Listing, error) {
	p = Local{}.Clean(p)
	info, err := os.Stat(p)
	if err != nil {
		return Listing{}, err
	}
	if !info.IsDir() {
		return Listing{}, ErrNotFolder
	}
	des, err := os.ReadDir(p)
	if err != nil && len(des) == 0 {
		return Listing{}, err
	}
	es := make([]Entry, len(des))
	fill := func(lo, hi int) {
		for i := lo; i < hi; i++ {
			if i%512 == 0 && ctx.Err() != nil {
				return
			}
			es[i] = localEntry(p, des[i])
		}
	}
	const chunk = 2048
	if len(des) <= chunk {
		fill(0, len(des))
	} else {
		var wg sync.WaitGroup
		workers := min(8, runtime.NumCPU())
		step := (len(des) + workers - 1) / workers
		for lo := 0; lo < len(des); lo += step {
			wg.Add(1)
			go func(lo, hi int) { defer wg.Done(); fill(lo, hi) }(lo, min(lo+step, len(des)))
		}
		wg.Wait()
	}
	if err := ctx.Err(); err != nil {
		return Listing{}, err
	}
	return Listing{Path: p, Parent: localParent(p), Entries: es}, nil
}

// localEntry describes one directory entry. A link is shown with the size and type of what it points to (a link to a
// folder opens like a folder), and marked as a link; a link to nothing is marked broken.
func localEntry(dir string, de fs.DirEntry) Entry {
	e := Entry{Name: de.Name()}
	info, err := de.Info()
	if err != nil { // the file went away between the two calls, or can't be read: still show its name
		if de.IsDir() {
			e.Flags |= FlagDir
		}
		return e
	}
	e.Flags |= sysFlags(info, e.Name)
	if info.Mode()&fs.ModeSymlink != 0 {
		e.Flags |= FlagLink
		if t, err := os.Stat(filepath.Join(dir, e.Name)); err == nil {
			info = t
		} else {
			e.Flags |= FlagBroken
		}
	}
	e.MTime = info.ModTime().UnixMilli()
	e.Mode = uint32(info.Mode())
	if info.IsDir() {
		e.Flags |= FlagDir
	} else {
		e.Size = info.Size()
	}
	if strings.HasPrefix(e.Name, ".") {
		e.Flags |= FlagHidden
	}
	return e
}

// Stat describes one path, following links the way List does.
func (Local) Stat(p string) (Entry, error) {
	p = Local{}.Clean(p)
	li, err := os.Lstat(p)
	if err != nil {
		return Entry{}, err
	}
	return localEntry(filepath.Dir(p), fs.FileInfoToDirEntry(li)), nil
}

// Clean makes p absolute and canonical. "~" stands for the home folder.
func (Local) Clean(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			p = home + p[1:]
		}
	}
	if runtime.GOOS == "windows" && len(p) == 2 && p[1] == ':' {
		p += `\` // "C:" alone means "the current folder of drive C", which a panel never wants
	}
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	return filepath.Clean(p)
}

// Parent is the folder above p ("" at a root).
func (Local) Parent(p string) string { return localParent(Local{}.Clean(p)) }

// Root is the top of the volume p is on: "C:\" on Windows, "/" elsewhere. (On macOS, folders of /Volumes/X are on the
// volume X: the root of those is /Volumes/X, which is what Ctrl+\ is expected to reach.)
func (Local) Root(p string) string {
	p = Local{}.Clean(p)
	if runtime.GOOS == "windows" {
		return filepath.VolumeName(p) + `\`
	}
	if runtime.GOOS == "darwin" {
		if rest, ok := strings.CutPrefix(p, "/Volumes/"); ok && rest != "" {
			name, _, _ := strings.Cut(rest, "/")
			return "/Volumes/" + name
		}
	}
	if runtime.GOOS == "linux" {
		for _, pre := range []string{"/media/", "/run/media/", "/mnt/"} {
			if rest, ok := strings.CutPrefix(p, pre); ok && rest != "" {
				parts := strings.SplitN(rest, "/", 3)
				n := 1
				if pre != "/mnt/" && len(parts) >= 2 { // /media/<user>/<volume>
					n = 2
				}
				if len(parts) >= n {
					return pre + strings.Join(parts[:n], "/")
				}
			}
		}
	}
	return "/"
}

// Join puts name under dir.
func (Local) Join(dir, name string) string { return filepath.Join(dir, name) }
