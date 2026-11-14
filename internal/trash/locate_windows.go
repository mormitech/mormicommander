package trash

// On Windows a recycled item goes to the "$Recycle.Bin" folder at the root of its own drive, into the folder of the
// user (named by the user's SID), as "$R" plus six random characters plus the old extension; next to it a "$I" file
// with the same ending keeps the old place and the date. Only the user's own folder can be read, so every folder that
// can be read is a candidate; the identity check in Locate picks the right item. Network shares and drives without a
// Recycle Bin give no folders, and their deletes are journaled as not undoable here.

import (
	"os"
	"path/filepath"
)

func platformTrashDirs(path string) []string {
	vol := filepath.VolumeName(path)
	if vol == "" || len(vol) != 2 { // a drive letter ("C:"); a share ("\\server\share") has no Recycle Bin we can use
		return nil
	}
	bin := vol + `\$Recycle.Bin`
	list, err := os.ReadDir(bin)
	if err != nil {
		return nil
	}
	var dirs []string
	for _, d := range list {
		if d.IsDir() {
			dirs = append(dirs, filepath.Join(bin, d.Name()))
		}
	}
	return dirs
}
