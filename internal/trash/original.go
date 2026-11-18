package trash

// Putting back an item the user picks in a trash folder (F-501): where it came from is read from the record the trash
// keeps of it (the freedesktop.org ".trashinfo" file on Linux, the "$I..." file of the Windows Recycle Bin). The
// macOS Trash keeps its "Put Back" notes in its own .DS_Store, which this package never reads; callers fall back on
// what they noted themselves (the undo journal) and else point at Finder's Put Back. Restore (locate.go) does the move,
// so the same rules hold: never over anything, and only out of a trash folder of the old place's volume.

import (
	"errors"
	"os"
	"path/filepath"
)

// ErrNoRecord is returned by Original when the trash keeps no record of where an item came from that this package
// can read (the macOS Trash, or a file that is not in a trash).
var ErrNoRecord = errors.New("the trash keeps no note of where it came from")

// Original says where the trashed item was before it went to the trash, from the trash's own record of it.
func Original(trashed string) (string, error) {
	if !filepath.IsAbs(trashed) {
		return "", errors.New("only full paths can be put back")
	}
	trashed = filepath.Clean(trashed)
	rec := recordOf(trashed)
	if rec == "" {
		return "", ErrNoRecord
	}
	b, err := os.ReadFile(rec)
	if err != nil {
		return "", ErrNoRecord
	}
	var p string
	if filepath.Base(filepath.Dir(trashed)) == "files" {
		p, _, err = parseTrashInfo(b, xdgTopDir(filepath.Dir(trashed)))
	} else {
		p, _, err = parseRecycleInfo(b)
	}
	if err != nil || !filepath.IsAbs(p) {
		return "", ErrNoRecord
	}
	return filepath.Clean(p), nil
}

// Folder is the trash folder of the user's home volume, the one a "go to the trash" command opens: ~/.Trash on macOS,
// the "files" folder of the freedesktop.org home trash on Linux, the user's own folder in the Recycle Bin of the
// system drive on Windows. "" when there is none.
func Folder() string { return homeTrashFolder() }
