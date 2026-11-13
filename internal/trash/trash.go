// Package trash moves files to the Trash (macOS, Linux) or the Recycle Bin (Windows), so a delete can be undone in the
// system's own way. The path is always passed as a separate argument or environment variable, never inside script
// text, so no file name can be read as a command. Platform parts are in trash_<os>.go.
//
// Every path must be absolute and must exist (checked with Lstat, so a broken symbolic link still counts). The check
// matters because the callers clean their paths, and a cleaned empty or relative path names the program's working
// folder: trashing "." by accident would throw away a whole folder nobody selected. A symbolic link is always trashed
// as the link itself, never as the file or folder it points to.
package trash

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Supported says whether this platform has a way to move files to the trash at all.
func Supported() bool { return supported }

// Move puts one file, folder or link in the trash.
func Move(path string) error {
	if err := check(path); err != nil {
		return err
	}
	return moveOne(path)
}

// MoveAll puts several items in the trash with as few calls to the system as the platform allows (one Finder call on
// macOS for all ordinary items), which is much faster than calling Move for each item of a large selection. All paths
// are checked before anything is moved. On a failure the returned error names what failed; some items may already be
// in the trash by then, so a caller that needs a per-item result should use Move.
func MoveAll(paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	for _, p := range paths {
		if err := check(p); err != nil {
			return err
		}
	}
	return moveMany(paths)
}

// check refuses paths that are empty, relative or missing.
func check(path string) error {
	if path == "" || !filepath.IsAbs(path) {
		return fmt.Errorf("%q: only full paths can be moved to the trash", path)
	}
	if _, err := os.Lstat(path); err != nil {
		return err
	}
	return nil
}

// isSymlink says whether path is a symbolic link itself.
func isSymlink(path string) bool {
	fi, err := os.Lstat(path)
	return err == nil && fi.Mode()&os.ModeSymlink != 0
}

// runCommand runs a helper program and returns its combined output; the tests replace it, so they never touch the
// real trash.
var runCommand = func(cmd *exec.Cmd) ([]byte, error) { return cmd.CombinedOutput() }

// lookPath finds a helper program; the tests replace it.
var lookPath = exec.LookPath

// errNoTool is returned on Linux when neither gio nor trash-put is installed.
var errNoTool = errors.New("no trash tool found: install gio (glib) or trash-cli")

// moveEach is the fallback for MoveAll on platforms without a batch call.
func moveEach(paths []string) error {
	for _, p := range paths {
		if err := moveOne(p); err != nil {
			return err
		}
	}
	return nil
}
