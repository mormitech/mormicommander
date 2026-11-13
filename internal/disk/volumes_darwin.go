package disk

import (
	"os"
	"path/filepath"
	"syscall"
)

// Volumes lists the startup disk as "/" and everything mounted under /Volumes. Unlike a disk analyzer, a file manager
// shows the startup disk at "/", where the user browses it (the Data volume is firmlinked into it).
func Volumes() []Volume {
	var out []Volume
	if u, err := UsageOf("/System/Volumes/Data"); err == nil && u.Total > 0 {
		out = append(out, Volume{Name: "Macintosh HD", Path: "/", Kind: "fixed", Total: u.Total, Free: u.Free})
	} else if u, err := UsageOf("/"); err == nil {
		out = append(out, Volume{Name: "Macintosh HD", Path: "/", Kind: "fixed", Total: u.Total, Free: u.Free})
	}
	entries, _ := os.ReadDir("/Volumes")
	for _, e := range entries {
		p := filepath.Join("/Volumes", e.Name())
		if info, err := os.Lstat(p); err != nil || info.Mode()&os.ModeSymlink != 0 {
			continue // "/Volumes/Macintosh HD" is a link to "/"
		}
		u, err := UsageOf(p)
		if err != nil || u.Total == 0 {
			continue
		}
		out = append(out, Volume{Name: e.Name(), Path: p, Kind: darwinKind(p), Total: u.Total, Free: u.Free})
	}
	return out
}

// darwinKind tells network shares (smbfs, afpfs, nfs, webdav) from local disks by the file system type name.
func darwinKind(p string) string {
	var st syscall.Statfs_t
	if syscall.Statfs(p, &st) != nil {
		return "removable"
	}
	name := make([]byte, 0, 16)
	for _, c := range st.Fstypename {
		if c == 0 {
			break
		}
		name = append(name, byte(c))
	}
	switch string(name) {
	case "smbfs", "afpfs", "nfs", "webdav", "macfuse", "osxfuse":
		return "network"
	case "cd9660", "udf":
		return "cdrom"
	}
	return "removable"
}
