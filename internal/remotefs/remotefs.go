// Package remotefs holds the few plain types that every remote file system
// client in mormiCommander shares: a directory entry, a transfer progress
// report, transfer options and the FS interface itself. Total Commander shows
// FTP, SFTP and (through plugins) WebDAV servers in a file panel exactly like a
// local folder, so the panel and the copy/move job engine should not care
// which protocol sits behind a connection. Keeping these types in their own
// tiny, dependency-free package lets the SFTP, WebDAV and FTP clients all
// implement one interface without importing each other, and lets the UI layer
// wire any of them into the virtual file system with a single adapter.
package remotefs

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"path"
	"time"
)

// ErrExists is returned by a transfer when TransferOptions.NoOverwrite is set
// and the destination already exists. The job engine turns it into Total
// Commander's "file exists: overwrite / skip / resume" question.
var ErrExists = errors.New("remotefs: destination already exists")

// ErrResumeUnsupported is returned when a transfer was asked to resume but the
// protocol or server cannot continue a partial file (for example a WebDAV
// upload, because HTTP PUT has no standard way to append).
var ErrResumeUnsupported = errors.New("remotefs: resume is not supported by this server")

// Entry describes one remote file, folder or link. It carries a little more
// than fs.FileInfo because a file panel needs to know whether a symbolic link
// points to a folder (so Enter can navigate into it) and where it points.
type Entry struct {
	Name    string      // base name
	Path    string      // full remote path (slash separated)
	Size    int64       // size in bytes (for links: size of the target when resolvable)
	Mode    fs.FileMode // permission and type bits as reported by the server
	ModTime time.Time   // last modification time
	ATime   time.Time   // last access time, zero when unknown
	UID     uint32      // numeric owner, 0 when unknown
	GID     uint32      // numeric group, 0 when unknown

	// IsDir is true for folders and for links that point to folders.
	IsDir bool
	// IsLink is true for symbolic links; Mode then has fs.ModeSymlink set.
	IsLink bool
	// LinkTarget is the raw link text for symbolic links.
	LinkTarget string
	// LinkBroken is true when the link target could not be resolved.
	LinkBroken bool
}

// Info adapts the entry to the standard fs.FileInfo interface, so code that
// already sorts or filters fs.FileInfo values (for example the panel's sort
// and the Find Files engine) can consume remote listings unchanged. For a link
// to a folder, IsDir reports true, matching how the panel treats it.
func (e Entry) Info() fs.FileInfo { return entryInfo{e} }

type entryInfo struct{ e Entry }

func (i entryInfo) Name() string { return i.e.Name }
func (i entryInfo) Size() int64  { return i.e.Size }
func (i entryInfo) Mode() fs.FileMode {
	m := i.e.Mode
	if i.e.IsDir && !i.e.IsLink {
		m |= fs.ModeDir
	}
	return m
}
func (i entryInfo) ModTime() time.Time { return i.e.ModTime }
func (i entryInfo) IsDir() bool        { return i.e.IsDir }
func (i entryInfo) Sys() any           { return i.e }

// Progress reports how far one file transfer has got. Total is -1 when the
// size is not known in advance. Done counts bytes of this file including any
// part that was already present when a transfer resumed, so a progress bar
// starts at the resumed position rather than at zero.
type Progress struct {
	Path  string // the remote path being transferred
	Done  int64  // bytes completed so far
	Total int64  // total bytes, or -1 when unknown
}

// ProgressFunc receives progress reports. It is called from the goroutine that
// runs the transfer and must return quickly.
type ProgressFunc func(Progress)

// TransferOptions controls a single download or upload.
type TransferOptions struct {
	// Resume continues a partial destination file instead of starting over:
	// only the missing tail is transferred and appended. When the destination
	// is already as large as (or larger than) the source, nothing is copied.
	Resume bool
	// NoOverwrite makes the transfer fail with ErrExists when the destination
	// exists (and Resume is not set).
	NoOverwrite bool
	// PreserveTimes copies the modification time to the destination.
	PreserveTimes bool
	// PreserveMode copies the permission bits to the destination.
	PreserveMode bool
	// Progress, when not nil, receives progress reports.
	Progress ProgressFunc
}

// FS is the operation set the panel and the job engine need from any remote
// connection. Paths are slash separated and absolute (or relative to the
// server's start folder). Metadata calls are quick request/response round
// trips; the long-running transfers take a context so the user can cancel
// them from the progress dialog.
type FS interface {
	// ReadDir lists a folder, without "." and "..".
	ReadDir(dir string) ([]Entry, error)
	// Stat returns information about a path, following links.
	Stat(p string) (Entry, error)
	// Mkdir creates one folder.
	Mkdir(p string) error
	// Remove deletes a file or an empty folder.
	Remove(p string) error
	// Rename moves or renames a file or folder within the server.
	Rename(oldPath, newPath string) error
	// Open opens a remote file for reading (used by F3 view / F4 edit).
	Open(p string) (io.ReadCloser, error)
	// Create creates or truncates a remote file for writing.
	Create(p string) (io.WriteCloser, error)
	// Download copies a remote file to a local path.
	Download(ctx context.Context, remote, local string, opts TransferOptions) (int64, error)
	// Upload copies a local file to a remote path.
	Upload(ctx context.Context, local, remote string, opts TransferOptions) (int64, error)
	// Close disconnects.
	Close() error
}

// Join joins remote path elements with forward slashes, whatever the local
// operating system uses. Remote servers always speak slash-separated paths,
// so filepath.Join would be wrong on Windows.
func Join(elem ...string) string { return path.Join(elem...) }

// ProgressWriter wraps an io.Writer, counting bytes and reporting them to a
// ProgressFunc. Every Write first checks the context, so a cancelled transfer
// stops at the next buffer boundary. Clients use it on both the download side
// (wrapping the local file) and the upload side (wrapping the remote file).
type ProgressWriter struct {
	Ctx      context.Context
	W        io.Writer
	Path     string
	Done     int64
	Total    int64
	Progress ProgressFunc
}

// Write implements io.Writer.
func (p *ProgressWriter) Write(b []byte) (int, error) {
	if p.Ctx != nil {
		if err := p.Ctx.Err(); err != nil {
			return 0, err
		}
	}
	n, err := p.W.Write(b)
	p.Done += int64(n)
	if p.Progress != nil && n > 0 {
		p.Progress(Progress{Path: p.Path, Done: p.Done, Total: p.Total})
	}
	return n, err
}

// ProgressReader is the reading counterpart of ProgressWriter.
type ProgressReader struct {
	Ctx      context.Context
	R        io.Reader
	Path     string
	Done     int64
	Total    int64
	Progress ProgressFunc
}

// Read implements io.Reader.
func (p *ProgressReader) Read(b []byte) (int, error) {
	if p.Ctx != nil {
		if err := p.Ctx.Err(); err != nil {
			return 0, err
		}
	}
	n, err := p.R.Read(b)
	p.Done += int64(n)
	if p.Progress != nil && n > 0 {
		p.Progress(Progress{Path: p.Path, Done: p.Done, Total: p.Total})
	}
	return n, err
}
