// On macOS the drive bar shows the startup disk at "/" (where the user browses
// it) and every volume mounted under /Volumes. The startup disk's name is not
// hard-coded: users rename "Macintosh HD", and Finder publishes the current
// name as the symbolic link /Volumes/<name> pointing to "/". internal/dirsize
// already reads that link (and takes the sizes from the data volume, which is
// the one that fills up), so the startup entry comes from dirsize.VolumeOf
// instead of duplicating that logic here.
//
// The kind of each other volume (the icon in the drive bar and popup) comes
// from statfs: a volume without MNT_LOCAL is a network share, cd9660 and udf
// are optical discs, and a volume the kernel flags MNT_REMOVABLE or that uses
// a FAT/exFAT file system (cards and USB sticks nearly always do) is
// removable. Everything else, such as a second internal disk or another APFS
// volume, is fixed. Without IOKit this is a best-effort guess; an external
// APFS or HFS+ USB disk may show as fixed.

package disk

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"

	"mormicommander/internal/dirsize"
)

// Volumes lists the startup disk as "/" and everything mounted under /Volumes. Unlike a disk analyzer, a file manager
// shows the startup disk at "/", where the user browses it (the Data volume is firmlinked into it).
func Volumes() []Volume {
	var out []Volume
	if v, err := dirsize.VolumeOf("/"); err == nil {
		name := v.Label
		if name == "" {
			name = "/"
		}
		out = append(out, Volume{Name: name, Path: "/", Kind: "fixed", Total: v.Total, Free: v.Available})
	}
	// The home folder comes right after the startup disk, as on Linux: it is where users go most.
	if home, err := os.UserHomeDir(); err == nil {
		if u, err := UsageOf(home); err == nil {
			out = append(out, Volume{Name: "Home", Path: home, Kind: "home", Total: u.Total, Free: u.Free})
		}
	}
	entries, _ := os.ReadDir("/Volumes")
	for _, e := range entries {
		p := filepath.Join("/Volumes", e.Name())
		if info, err := os.Lstat(p); err != nil || info.Mode()&os.ModeSymlink != 0 {
			continue // "/Volumes/Macintosh HD" is a link to "/"
		}
		var st unix.Statfs_t
		if unix.Statfs(p, &st) != nil || st.Blocks == 0 {
			continue
		}
		if cstr(st.Mntonname[:]) != p {
			continue // a plain folder under /Volumes, not a mount point
		}
		bs := uint64(st.Bsize)
		out = append(out, Volume{Name: e.Name(), Path: p, Kind: darwinKind(cstr(st.Fstypename[:]), st.Flags),
			Total: st.Blocks * bs, Free: st.Bavail * bs})
	}
	return out
}

// darwinKind classifies a volume by its file system type name and statfs flags (see the file comment).
func darwinKind(fstype string, flags uint32) string {
	switch fstype {
	case "smbfs", "afpfs", "nfs", "webdav", "macfuse", "osxfuse", "ftp":
		return "network"
	case "cd9660", "udf", "cddafs":
		return "cdrom"
	}
	if flags&unix.MNT_LOCAL == 0 {
		return "network"
	}
	if flags&unix.MNT_REMOVABLE != 0 {
		return "removable"
	}
	switch fstype {
	case "msdos", "exfat":
		return "removable"
	}
	return "fixed"
}

func cstr(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}
