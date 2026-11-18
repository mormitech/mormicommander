package trash

// On Linux the freedesktop.org trash specification says where an item goes: to the home trash
// ($XDG_DATA_HOME/Trash, by default ~/.local/share/Trash) when it is on the same volume as that folder, otherwise to
// "$top/.Trash/$uid" or "$top/.Trash-$uid" at the top of its own volume. The item lies in the "files" folder of the
// trash and its record ("<name>.trashinfo", with the old place and the date) in the "info" folder next to it. gio
// copies an item to the home trash when its volume has no trash of its own, so the home trash is always looked at
// too.

import (
	"os"
	"path/filepath"
	"strconv"
)

func platformTrashDirs(path string) []string {
	var dirs []string
	home := xdgHomeTrash()
	if home != "" && sameDevice(filepath.Dir(path), filepath.Dir(filepath.Dir(home))) {
		return []string{home}
	}
	if top, ok := mountPoint(filepath.Dir(path)); ok {
		uid := strconv.Itoa(os.Getuid())
		dirs = append(dirs, filepath.Join(top, ".Trash", uid, "files"), filepath.Join(top, ".Trash-"+uid, "files"))
	}
	if home != "" {
		dirs = append(dirs, home)
	}
	return dirs
}

func homeTrashFolder() string { return xdgHomeTrash() }
