package fileops

// A copy should look like the original: same dates, same rights, same extended attributes. copyMeta applies what the
// request asks for, in an order that works: extended attributes and owner first (changing the owner clears the
// setuid bit, so the mode comes after it), then the mode, then the dates (writing attributes would otherwise bump the
// modification date), and the Windows attributes last, because a read-only flag set earlier would block the rest.
// Metadata problems are logged but don't fail the file: the bytes arrived, and many target file systems (FAT, network
// shares) simply can't store everything.
//
// Creation dates: Windows gets the source's creation date. macOS has no direct call for it, but setting a
// modification date earlier than the creation date moves the creation date back too, so old files keep old creation
// dates there. Linux has no settable creation date.

import (
	"io/fs"
	"os"
)

func (j *job) copyMeta(src, dst string, info fs.FileInfo) {
	if !j.req.NoAttrs {
		if err := copyXattrs(src, dst); err != nil {
			j.log("warning", src, dst, err)
		}
	}
	if j.req.Owner {
		if err := copyOwner(dst, info); err != nil && !isPerm(err) {
			j.log("warning", src, dst, err)
		}
	}
	if !j.req.NoPerms {
		m := info.Mode() & (fs.ModePerm | fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky)
		if err := chmod(dst, m); err != nil {
			j.log("warning", src, dst, err)
		}
	}
	if !j.req.NoDates {
		if err := os.Chtimes(lp(dst), atimeOf(info), info.ModTime()); err != nil {
			j.log("warning", src, dst, err)
		}
		if err := setCreationTime(dst, info); err != nil {
			j.log("warning", src, dst, err)
		}
	}
	if !j.req.NoPerms {
		if err := copyPlatformAttrs(src, dst); err != nil {
			j.log("warning", src, dst, err)
		}
	}
}
