//go:build windows

package lister

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// keepOpen is false on Windows: even with FILE_SHARE_DELETE, MoveFileEx
// refuses to rename another file over one that is still open ("Access is
// denied"), so an editor saving via rename or a log rotation would fail for
// as long as the lister shows the file. A File therefore holds no handle
// between reads there: each cache fill, direct scan or Reload opens the path
// and closes it again. Reload still notices the replacement by file identity.
const keepOpen = false

// openShared opens path for reading with FILE_SHARE_DELETE in addition to the
// read/write sharing os.Open grants. Without it the viewer's handle would
// stop other programs from renaming the file, replacing it (log rotation,
// editors that save via rename) or deleting it for as long as the lister
// shows it, which is what Unix allows and what Reload expects to observe.
func openShared(path string) (*os.File, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	p, err := windows.UTF16PtrFromString(abs)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING,
		// Backup semantics lets folders open too, as os.Open does, so the
		// caller can report "is a folder" instead of "access denied".
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(h), path), nil
}
