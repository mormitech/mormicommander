// This file replaces an existing file with a freshly written one while keeping what belongs to the old file rather
// than to its content. The archive writers build every changed archive in a temporary file and rename it over the
// original. A rename swaps in a brand-new file, so on its own it drops the original's hard links (the other names
// keep the old content), its owner and group, and its extended attributes (Finder tags, quarantine flag, user.*
// attributes). Replace keeps them:
//
//   - When the original has other hard links, the new content is copied into the original file itself, so every name
//     sees the update. A backup copy is taken first and written back if the copy fails part way, so a failure still
//     leaves the original as it was.
//   - Otherwise the owner, group and extended attributes of the original are put on the temporary file before the
//     rename. Whatever the system refuses (another user's file, a file system without attributes) is skipped quietly.
//
// When the original cannot be opened for writing, Replace falls back to the plain rename the writers always did.
package robust

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// copyData copies the new content during an in-place update; tests swap it to simulate a failure part way through.
var copyData = io.Copy

// errFallback marks an in-place update that changed nothing and should be replaced by a plain rename.
var errFallback = errors.New("robust: in-place update not possible")

// Replace moves tmp over target like Rename, but keeps target's hard links, owner, group and extended attributes.
// tmp is gone afterwards when Replace succeeds.
func Replace(tmp, target string) error {
	fi, err := os.Lstat(target)
	if err != nil || !fi.Mode().IsRegular() {
		return Rename(tmp, target)
	}
	if linkCount(target, fi) > 1 {
		err := overwriteInPlace(tmp, target)
		if err == nil {
			os.Remove(tmp)
			return nil
		}
		if !errors.Is(err, errFallback) {
			return err
		}
	}
	keepOwner(fi, tmp)
	copyXattrs(target, tmp)
	return Rename(tmp, target)
}

// overwriteInPlace copies tmp's content into target's own file. Errors wrapping errFallback mean target was not
// touched; any other error means the copy failed and target was restored from the backup (or, if even that failed,
// the error names the backup file that still holds the original content).
func overwriteInPlace(tmp, target string) error {
	dst, err := os.OpenFile(target, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("%w: %v", errFallback, err)
	}
	defer dst.Close()
	src, err := os.Open(tmp)
	if err != nil {
		return fmt.Errorf("%w: %v", errFallback, err)
	}
	defer src.Close()

	backup, err := os.CreateTemp(filepath.Dir(target), "."+TempStem(filepath.Base(target))+".backup-*")
	if err != nil {
		return fmt.Errorf("%w: %v", errFallback, err)
	}
	backupName := backup.Name()
	keepBackup := false
	defer func() {
		backup.Close()
		if !keepBackup {
			os.Remove(backupName)
		}
	}()
	if _, err := io.Copy(backup, dst); err != nil {
		return fmt.Errorf("%w: %v", errFallback, err)
	}

	writeErr := rewrite(dst, src)
	if writeErr == nil {
		return nil
	}
	if _, err := backup.Seek(0, io.SeekStart); err == nil {
		if err = rewrite(dst, backup); err == nil {
			return writeErr
		}
	}
	keepBackup = true
	return fmt.Errorf("%v (the original content is kept in %s)", writeErr, backupName)
}

// rewrite replaces the whole content of dst with what r yields and flushes it to disk.
func rewrite(dst *os.File, r io.Reader) error {
	if err := dst.Truncate(0); err != nil {
		return err
	}
	if _, err := dst.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if _, err := copyData(dst, r); err != nil {
		return err
	}
	return dst.Sync()
}
