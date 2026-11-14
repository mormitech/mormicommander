package trash

// On macOS an item on the volume of the home folder goes to ~/.Trash; an item on another volume goes to that volume's
// .Trashes/<uid> folder. Finder and NSFileManager both rename the item there, adding a number when the name is taken.

import (
	"os"
	"path/filepath"
	"strconv"
)

func platformTrashDirs(path string) []string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil
	}
	if sameDevice(filepath.Dir(path), home) {
		return []string{filepath.Join(home, ".Trash")}
	}
	top, ok := mountPoint(filepath.Dir(path))
	if !ok {
		return nil
	}
	return []string{filepath.Join(top, ".Trashes", strconv.Itoa(os.Getuid()))}
}
