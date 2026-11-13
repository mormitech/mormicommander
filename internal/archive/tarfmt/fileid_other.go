//go:build !unix

// On Windows (and other non-Unix systems) the file information Go returns
// carries no inode number, so packing does not try to detect hard links:
// every name is stored with its own copy of the data, which is always
// correct, just not as compact. The Unix version lives in fileid_unix.go.
package tarfmt

import "io/fs"

type fileKey struct{ dev, ino uint64 }

func hardLinkKey(fs.FileInfo) (fileKey, bool) { return fileKey{}, false }
