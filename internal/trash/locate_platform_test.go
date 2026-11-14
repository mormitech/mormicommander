//go:build darwin || linux

package trash

// The list of trash folders on macOS and Linux, computed only: nothing in them is read or changed.

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestPlatformTrashDirs(t *testing.T) {
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home folder")
	}
	switch runtime.GOOS {
	case "darwin":
		// an item in the home folder goes to ~/.Trash
		if got := platformTrashDirs(filepath.Join(home, "some file.txt")); len(got) != 1 || got[0] != filepath.Join(home, ".Trash") {
			t.Errorf("home item: %v", got)
		}
	case "linux":
		// an item on the volume of XDG_DATA_HOME goes to its home trash
		got := platformTrashDirs(filepath.Join(data, "x.txt"))
		if len(got) == 0 || got[len(got)-1] != filepath.Join(data, "Trash", "files") {
			t.Errorf("home trash: %v", got)
		}
	}
	if top, ok := mountPoint(filepath.Join(data, "missing", "deeper")); !ok || top == "" {
		t.Error("mountPoint of a path that is gone")
	}
}
