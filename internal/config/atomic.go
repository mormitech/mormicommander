// Atomic saving of settings files. A crash, a full disk or a power cut in the
// middle of writing wincmd.ini must never leave the user with half a file and
// all their hotlist entries and hotkeys gone. So we write the new content to a
// temporary file in the same folder, flush it to disk, and only then rename it
// over the old file; a rename inside one folder is atomic on every platform we
// support (Go's os.Rename uses MoveFileEx with MOVEFILE_REPLACE_EXISTING on
// Windows). The old file's permission bits are kept, and optionally the
// previous version is kept as a ".bak" copy next to it.

package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteFileAtomic writes data to path through a temporary file and a rename.
// perm is used only when the file doesn't exist yet; an existing file keeps
// its mode. Missing parent folders are created (0755).
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if st, err := os.Stat(path); err == nil {
		if !st.Mode().IsRegular() {
			return fmt.Errorf("config: %s is not a regular file", path)
		}
		perm = st.Mode().Perm()
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			tmp.Close()
			os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	ok = true
	syncDir(dir)
	return nil
}

// WriteFileAtomicBackup is WriteFileAtomic that first copies the current file
// (if any) to path+".bak", so the user can get the previous settings back.
func WriteFileAtomicBackup(path string, data []byte, perm os.FileMode) error {
	if old, err := os.ReadFile(path); err == nil {
		if err := WriteFileAtomic(path+".bak", old, perm); err != nil {
			return err
		}
	}
	return WriteFileAtomic(path, data, perm)
}
