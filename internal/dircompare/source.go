// This file defines what the comparison reads from: a Lister lists folders and opens files. Total Commander compares
// whatever the two panels show, which may be a local folder on one side and a ZIP archive or an FTP folder on the
// other, so the engine never calls the os package directly. OS is the Lister for the local disk; other file systems
// (archives, FTP, SFTP) wrap their own listing code in this small interface when the UI wires them up. OS also marks
// pipes, sockets and devices as Special and refuses to open them, and it can resolve real paths so that a recursive
// comparison recognises symbolic link loops.
package dircompare

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Entry is one file or folder as the comparison sees it.
type Entry struct {
	Name    string    // name in its folder, never a path
	Size    int64     // bytes; ignored for folders
	ModTime time.Time // last modification
	IsDir   bool      // a folder, or a link to one
	Hidden  bool      // a dotfile or a file the system marks hidden
	// Special marks a named pipe, socket or device (or a link to one). Such entries are compared by date and size
	// only: their bytes are never read, because opening a pipe without a writer blocks forever.
	Special bool
}

// RealPather is an optional interface of a Lister that can resolve a folder to its canonical path with every symbolic
// link expanded. Recursive comparison uses it to notice a link that leads back to one of its own ancestors (a
// symlink loop) and stops there instead of walking forever. Listers without it are protected by a depth limit.
type RealPather interface {
	RealPath(ctx context.Context, dir string) (string, error)
}

// ErrNotRegular is returned by OS.Open for named pipes, sockets and devices.
var ErrNotRegular = errors.New("dircompare: not a regular file")

// specialMode holds the type bits of entries whose contents are not read: pipes, sockets and devices.
const specialMode = fs.ModeNamedPipe | fs.ModeSocket | fs.ModeDevice | fs.ModeCharDevice

// Lister lists folders and opens files of one file system.
type Lister interface {
	// ReadDir lists the folder dir. Links should be described by their target, as a panel shows them.
	ReadDir(ctx context.Context, dir string) ([]Entry, error)
	// Open opens a file for reading (used only by content comparison).
	Open(ctx context.Context, path string) (io.ReadCloser, error)
	// Join puts a name under a folder in this file system's path syntax.
	Join(dir, name string) string
}

// OS is the Lister for the local file system.
type OS struct{}

// ReadDir lists a local folder. Symbolic links are followed so a link to a file compares as that file; a broken link
// is reported as the link itself (size 0).
func (OS) ReadDir(ctx context.Context, dir string) ([]Entry, error) {
	des, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(des))
	for _, de := range des {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var info fs.FileInfo
		if de.Type()&fs.ModeSymlink != 0 {
			info, err = os.Stat(filepath.Join(dir, de.Name()))
		}
		if info == nil || err != nil {
			info, err = de.Info()
			if err != nil {
				// The entry vanished between the listing and the stat; leave it out like a refreshed panel would.
				continue
			}
		}
		e := Entry{Name: de.Name(), ModTime: info.ModTime(), IsDir: info.IsDir(), Hidden: isHidden(de.Name(), info),
			Special: info.Mode()&specialMode != 0}
		if !e.IsDir {
			e.Size = info.Size()
		}
		out = append(out, e)
	}
	return out, nil
}

// Open opens a local file. Pipes, sockets and devices are refused before the open call, which could otherwise block
// forever out of reach of the context.
func (OS) Open(_ context.Context, path string) (io.ReadCloser, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if st.Mode()&specialMode != 0 {
		return nil, &fs.PathError{Op: "open", Path: path, Err: ErrNotRegular}
	}
	return os.Open(path)
}

// RealPath resolves a local folder with all symbolic links expanded (RealPather).
func (OS) RealPath(_ context.Context, dir string) (string, error) { return filepath.EvalSymlinks(dir) }

// Join joins with the local path separator.
func (OS) Join(dir, name string) string { return filepath.Join(dir, name) }
