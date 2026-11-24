// Package vfs lists folders for the file panels. It has no UI dependencies, so it cross-compiles and tests on its own.
//
// A panel never reads the disk itself: it asks an FS. Local is the real file system; Mock is a fictional tree for
// screenshots (--demo). Feature agents add more (archives, FTP, SFTP) by implementing FS and registering it for a URL
// scheme with Register, so a panel can show "zip:///path/a.zip/inner" or "ftp://host/path" the same way.
package vfs

import (
	"context"
	"errors"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

// Flags of an entry, as a bit set (the panel gets one small number per file instead of a row of booleans).
const (
	FlagDir      = 1 << iota // a folder, or a link to one
	FlagLink                 // a symbolic link (or a Windows reparse point)
	FlagHidden               // a dotfile, or hidden by the system (macOS UF_HIDDEN, Windows FILE_ATTRIBUTE_HIDDEN)
	FlagExec                 // can be run (Unix x bit, or .exe/.bat/.cmd/.com on Windows)
	FlagBroken               // a link whose target is missing
	FlagSystem               // Windows FILE_ATTRIBUTE_SYSTEM
	FlagReadOnly             // Windows FILE_ATTRIBUTE_READONLY, or no write bit for anyone on Unix
	FlagNoAttr               // has no attributes to show (a server or share in the Network panel): the Attr column is blank
	FlagArchive              // Windows FILE_ATTRIBUTE_ARCHIVE (only meaningful together with FlagWinAttrs)
	FlagWinAttrs             // the Windows attribute word was read, so FlagArchive is the real archive bit (F-686)
	FlagCloud                // an online-only cloud placeholder (iCloud Drive, OneDrive …): reading it downloads it (#30)
)

// Entry is one file or folder of a listing.
type Entry struct {
	Name  string // the name in the folder, never a path
	Size  int64  // bytes; 0 for folders
	MTime int64  // last change, Unix milliseconds
	Flags int    // Flag* bits
	Mode  uint32 // Unix permission bits (and type bits as in io/fs.FileMode); on Windows only the read-only bit is meaningful
	// Sparse is set for a sparse file that takes far less space than its length (Docker's Docker.raw), and Disk is
	// then its size on disk, so the size column can show the space it really takes (local.go, sparse_unix.go).
	Sparse bool
	Disk   int64
}

// Listing is a folder as a panel shows it.
type Listing struct {
	Path    string  `json:"path"`    // the folder, cleaned
	Parent  string  `json:"parent"`  // the folder above, "" at the root
	Entries []Entry `json:"-"`       // see Rows for the form the page gets
	Rows    [][]any `json:"entries"` // [name, size, mtime, flags, mode(, disk)] per entry: compact, because a folder can have 100k files
}

// FS is a file system a panel can show.
type FS interface {
	// List reads the folder p. It honours ctx, so a slow network folder can be left with Esc.
	List(ctx context.Context, p string) (Listing, error)
	// Stat describes one path (for the file under the cursor, links and the like).
	Stat(p string) (Entry, error)
	// Clean makes a path canonical for this FS (no trailing separator, no "..").
	Clean(p string) string
	// Parent is the folder above p, or "" when p is a root.
	Parent(p string) string
	// Root is the top of the volume p is on ("/" on Unix, "C:\" on Windows).
	Root(p string) string
	// Join puts a name under a folder.
	Join(dir, name string) string
}

// Rows turns entries into the compact rows of Listing.Rows.
func Rows(es []Entry) [][]any {
	rows := make([][]any, len(es))
	for i, e := range es {
		if e.Sparse {
			rows[i] = []any{e.Name, e.Size, e.MTime, e.Flags, e.Mode, e.Disk}
			continue
		}
		rows[i] = []any{e.Name, e.Size, e.MTime, e.Flags, e.Mode}
	}
	return rows
}

// ErrNotFolder is returned by List for a path that is a file.
var ErrNotFolder = errors.New("not a folder")

var (
	regMu   sync.Mutex
	schemes = map[string]FS{}
)

// Register makes fs answer for paths that start with scheme + "://" (for example "ftp"). Call it from an init function.
func Register(scheme string, fs FS) {
	regMu.Lock()
	defer regMu.Unlock()
	schemes[scheme] = fs
}

// For returns the FS that answers for p: a registered scheme, or def for plain paths.
func For(p string, def FS) FS {
	if i := strings.Index(p, "://"); i > 0 {
		regMu.Lock()
		fs := schemes[p[:i]]
		regMu.Unlock()
		if fs != nil {
			return fs
		}
	}
	return def
}

// Schemes lists the registered URL schemes, sorted.
func Schemes() []string {
	regMu.Lock()
	defer regMu.Unlock()
	out := make([]string, 0, len(schemes))
	for s := range schemes {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// SplitExt splits a name into base and extension the way the Ext column shows it: "a.tar.gz" → "a.tar", "gz";
// ".bashrc" has no extension (a dotfile is all name), and neither has "README".
func SplitExt(name string) (base, ext string) {
	i := strings.LastIndexByte(name, '.')
	if i <= 0 || i == len(name)-1 {
		return name, ""
	}
	return name[:i], name[i+1:]
}

// slashParent is Parent for "/"-separated paths (Mock, and Local on Unix).
func slashParent(p string) string {
	p = path.Clean(p)
	if p == "/" || p == "." {
		return ""
	}
	return path.Dir(p)
}

// isWindowsPath says whether p looks like a Windows path, so the Local FS on Windows treats it with its own rules.
func isWindowsPath() bool { return runtime.GOOS == "windows" }

// localParent is Parent for the real file system.
func localParent(p string) string {
	if !isWindowsPath() {
		return slashParent(p)
	}
	p = filepath.Clean(p)
	d := filepath.Dir(p)
	if d == p {
		return ""
	}
	return d
}
