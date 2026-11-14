// This file holds the one rule every remote client uses before it writes a downloaded folder tree to the local disk:
// a path that came from the server must be a plain relative path that stays inside the destination folder on this
// operating system. A server name such as "..\x" is harmless to the server but climbs out of the folder on Windows,
// and "CON" or "a:b" cannot be stored there at all. FTP and SFTP both refuse such a name with an error (stopping the
// copy) rather than one of them silently leaving it out, so a download never ends looking complete when it is not.
package remotefs

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
)

// ErrUnsafeName is wrapped in the error a tree download returns for a server name that cannot be stored safely in
// the local destination folder.
var ErrUnsafeName = errors.New("not a safe local name")

// LocalRel turns rel, a slash-separated path below the download root as the server named it, into a relative local
// path that is guaranteed to stay inside the destination folder. It returns ErrUnsafeName when that is not possible:
// an empty, absolute or climbing path (or one with empty elements), a name this OS cannot store, or (on Windows) a backslash inside a server name,
// which would turn into a folder separator and put the file in the wrong place even when it stays inside.
func LocalRel(rel string) (string, error) {
	if filepath.Separator == '\\' && strings.ContainsRune(rel, '\\') {
		return "", ErrUnsafeName
	}
	lrel := filepath.FromSlash(rel)
	if rel == "" || !fs.ValidPath(rel) || !filepath.IsLocal(lrel) {
		return "", ErrUnsafeName
	}
	return lrel, nil
}
