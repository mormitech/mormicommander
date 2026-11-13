package fileops

// Renaming in place (Shift+F6, or F6 with a new name in the same folder) is instant, so it is a plain function rather
// than a queued job. It refuses to replace a different file that already has the new name, and it handles the case
// that trips up simple renames: on case-insensitive file systems (macOS and Windows by default) "a.txt" and "A.txt"
// are the same file, so a direct rename may do nothing or fail. Such a case-only rename goes through a temporary name.

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// ErrExists is returned by RenameItem when another file already has the new name.
var ErrExists = errors.New("a file with that name already exists")

// RenameItem renames the file or folder at path to newName in the same folder and returns the new path.
func RenameItem(path, newName string) (string, error) {
	if err := checkName(newName); err != nil {
		return "", err
	}
	path = filepath.Clean(path)
	si, err := lstat(path)
	if err != nil {
		return "", err
	}
	dst := filepath.Join(filepath.Dir(path), newName)
	if dst == path {
		return dst, nil
	}
	if di, err := lstat(dst); err == nil {
		if !sameFile(si, di) || !strings.EqualFold(filepath.Base(path), newName) {
			return "", ErrExists
		}
		// the same file under another case: two steps through a free temporary name
		tmp := filepath.Join(filepath.Dir(path), fmt.Sprintf(".%s.mc-rename-%d", newName, time.Now().UnixNano()))
		if err := rename(path, tmp); err != nil {
			return "", err
		}
		if err := rename(tmp, dst); err != nil {
			rename(tmp, path)
			return "", err
		}
		return dst, nil
	}
	if err := rename(path, dst); err != nil {
		return "", err
	}
	return dst, nil
}

// checkName refuses names that are empty, special, or would point into another folder.
func checkName(name string) error {
	switch {
	case name == "" || name == "." || name == "..":
		return errors.New("the name is empty or not allowed")
	case strings.ContainsAny(name, `/\`) || strings.ContainsRune(name, 0):
		return errors.New("the name must not contain / or \\")
	}
	return checkNamePlatform(name)
}
