// Package xattr reads and removes extended attributes of files: the extra named values a file system keeps beside a
// file's data. On macOS they hold Finder tags, colour labels, resource forks and the quarantine flag that Gatekeeper
// puts on files downloaded from the internet (com.apple.quarantine); on Linux they hold user.* values and ACLs. On
// Windows and the other systems there is nothing to read here: Supported says false and every call is a no-op.
//
// Links are never followed: the attributes read or removed are always those of the link itself, never of what it
// points to. The package has no UI and is used by the Properties dialog, "Remove quarantine" and the copy warning
// for file systems that drop extended attributes (FAT, exFAT).
package xattr

import (
	"context"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// Quarantine is the attribute macOS puts on downloaded files; while it is there, Gatekeeper asks before opening them.
const Quarantine = "com.apple.quarantine"

// Attr is one extended attribute: its name and the size of its value in bytes.
type Attr struct {
	Name string `json:"name"`
	Size int    `json:"size"`
}

// Supported says whether this system has extended attributes that this package can read.
func Supported() bool { return supported }

// List returns the extended attributes of one file or folder, sorted by name. A file system without extended
// attributes gives an empty list, not an error.
func List(path string) ([]Attr, error) {
	names, err := listNames(path)
	if err != nil {
		return nil, err
	}
	out := make([]Attr, 0, len(names))
	for _, n := range names {
		out = append(out, Attr{Name: n, Size: valueSize(path, n)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Has says whether the file carries the named attribute.
func Has(path, name string) bool { return valueSize(path, name) >= 0 }

// Remove deletes one attribute from one file. An attribute that is not there is not an error. It answers whether
// something was removed.
func Remove(path, name string) (bool, error) {
	if !Has(path, name) {
		return false, nil
	}
	if err := removeName(path, name); err != nil {
		if isAbsent(err) {
			return false, nil
		}
		return false, &fs.PathError{Op: "remove attribute", Path: path, Err: err}
	}
	return true, nil
}

// Failure is one file whose attribute could not be removed.
type Failure struct {
	Path string
	Err  error
}

// Result is what RemoveAll did.
type Result struct {
	Checked int       // files and folders looked at
	Removed int       // of those, the ones that had the attribute and lost it
	Failed  []Failure // the ones that had it and kept it
}

// RemoveAll removes the named attribute from every path and, when recursive, from everything inside the folders
// among them (links are neither followed nor entered). progress, when given, is called with each path looked at.
// It stops between files when ctx is cancelled and returns ctx's error with what it did so far.
func RemoveAll(ctx context.Context, paths []string, name string, recursive bool, progress func(path string, r Result)) (Result, error) {
	var r Result
	one := func(p string) {
		r.Checked++
		ok, err := Remove(p, name)
		if err != nil {
			r.Failed = append(r.Failed, Failure{Path: p, Err: err})
		} else if ok {
			r.Removed++
		}
		if progress != nil {
			progress(p, r)
		}
	}
	for _, root := range paths {
		if err := ctx.Err(); err != nil {
			return r, err
		}
		if !recursive {
			one(root)
			continue
		}
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if cerr := ctx.Err(); cerr != nil {
				return cerr
			}
			if err != nil {
				// d == nil: the path itself is not there. Otherwise a folder whose contents can't be read: it was
				// already handled on the first visit, and what is inside it stays as it is.
				if d == nil {
					r.Failed = append(r.Failed, Failure{Path: p, Err: err})
				}
				return nil
			}
			one(p)
			return nil
		})
		if err != nil {
			return r, err
		}
	}
	return r, nil
}

// FindAny looks for the first file among paths (and, for folders, inside them, at most limit entries in all) that
// carries any extended attribute other than those the system adds by itself and that nobody misses. It answers that
// file's path and the attribute's name, or "" when none was found within the limit.
func FindAny(ctx context.Context, paths []string, limit int) (string, string) {
	seen := 0
	var foundPath, foundName string
	stop := fs.SkipAll
	for _, root := range paths {
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if ctx.Err() != nil || seen >= limit {
				return stop
			}
			if err != nil {
				return nil // not there, or a folder that can't be read (already looked at on the first visit)
			}
			seen++
			if names, lerr := listNames(p); lerr == nil {
				for _, n := range names {
					if Worth(n) {
						foundPath, foundName = p, n
						return stop
					}
				}
			}
			return nil
		})
		if foundPath != "" || seen >= limit || ctx.Err() != nil {
			break
		}
	}
	return foundPath, foundName
}

// Worth says whether losing this attribute in a copy is worth a word to the user. The quarantine flag and the
// system's own bookkeeping (Spotlight, the last-used date, file provider marks) are not: losing them is harmless.
func Worth(name string) bool {
	switch {
	case name == Quarantine, name == "com.apple.lastuseddate#PS", name == "com.apple.provenance",
		strings.HasPrefix(name, "com.apple.metadata:kMDItemDownloaded"), strings.HasPrefix(name, "com.apple.metadata:kMDItemWhereFroms"),
		strings.HasPrefix(name, "com.apple.fileprovider."), strings.HasPrefix(name, "security."), name == "com.apple.macl":
		return false
	}
	return true
}

// DropsXattrs says whether a file system (by the name the system gives it: "msdos", "exfat", "vfat", "FAT32" …)
// keeps no extended attributes, so a copy onto it loses them. macOS writes them to "._name" files there, which
// most other systems ignore or delete.
func DropsXattrs(fileSystem string) bool {
	switch strings.ToLower(strings.TrimSpace(fileSystem)) {
	case "msdos", "exfat", "vfat", "fat", "fat12", "fat16", "fat32", "fuse.exfat", "sdcardfs", "udf", "cd9660", "iso9660":
		return true
	}
	return false
}
