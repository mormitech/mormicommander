// Package cloudfile tells whether a file is a cloud placeholder: an iCloud Drive or OneDrive (or other File Provider /
// Cloud Files) item whose contents are not on this computer, so that reading it makes the system download it
// (board #30, F-873). Only what the system itself records is used, read from the stat data a listing already has:
//
//   - macOS: the SF_DATALESS file flag (st_flags 0x40000000), which the File Provider framework sets on "online-only"
//     files and folders of iCloud Drive, OneDrive, Dropbox, Google Drive …
//   - Windows: FILE_ATTRIBUTE_RECALL_ON_DATA_ACCESS (0x400000), FILE_ATTRIBUTE_RECALL_ON_OPEN (0x40000) and
//     FILE_ATTRIBUTE_OFFLINE (0x1000), which the Cloud Files API sets on OneDrive "available when online" files.
//   - Linux and the rest: no such signal, so nothing is a placeholder.
//
// Nothing here opens or reads a file, so asking never triggers a download.
package cloudfile

import (
	"io/fs"
	"os"
)

// Placeholder reports whether the system marks the file of info as online-only (its contents are not downloaded).
func Placeholder(info fs.FileInfo) bool {
	if info == nil {
		return false
	}
	return placeholder(info)
}

// PathPlaceholder is Placeholder for a path (the item itself, a symbolic link is not followed). A path that can't be
// looked at is not a placeholder.
func PathPlaceholder(path string) bool {
	info, err := os.Lstat(path)
	if err != nil {
		return false
	}
	return placeholder(info)
}
