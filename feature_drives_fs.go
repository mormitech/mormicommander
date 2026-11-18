package main

// The drive list as a folder (F-716, Commands > Drive list as folder, cm_OpenDrives; TC's "My computer"): drives://
// is a folder a panel shows, with one folder entry per drive or volume of the drive list (Windows drive letters, the
// macOS startup disk and /Volumes, the Linux mounts, and Home). Enter on one opens the real top of that volume, so
// the panel is on an ordinary local folder again (as a Network share does, feature_network_fs.go). With Options >
// Display > ".. in a drive's root opens the drive list as a folder" the ".." of a drive's top leads here
// (frontend/src/features/navigation.js). Nothing can be copied, moved, made or deleted in drives:// itself
// (FS.PathInfo marks it read-only, and fileops refuses it before asking anything).

import (
	"context"
	"io/fs"
	"strconv"
	"strings"

	"mormicommander/internal/disk"
	"mormicommander/internal/vfs"
)

const drivesRoot = "drives://"

// drivesVolumes lists the volumes the folder shows (a variable so tests never depend on the real disks).
var drivesVolumes = disk.Volumes

func init() { vfs.Register("drives", drivesFS{}) }

// drivesPath says whether p is the drive list folder or a name in it.
func drivesPath(p string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(p)), drivesRoot)
}

// drivesName is the entry name of a volume: its drive bar name, with characters a path can't hold replaced.
func drivesName(v disk.Volume) string {
	n := strings.NewReplacer("/", "∕", "\\", "∖").Replace(strings.TrimSpace(v.Name))
	if n == "" {
		n = strings.NewReplacer("/", "∕", "\\", "∖").Replace(v.Path)
	}
	return n
}

// drivesEntries pairs the entry names of the folder with their volumes, in drive list order (a name that comes twice
// gets " (2)" and so on).
func drivesEntries(vols []disk.Volume) ([]string, map[string]disk.Volume) {
	names := []string{}
	by := map[string]disk.Volume{}
	for _, v := range vols {
		base := drivesName(v)
		n := base
		for i := 2; ; i++ {
			if _, dup := by[strings.ToLower(n)]; !dup {
				break
			}
			n = base + " (" + strconv.Itoa(i) + ")"
		}
		by[strings.ToLower(n)] = v
		names = append(names, n)
	}
	return names, by
}

// drivesFS is the vfs.FS of drives://.
type drivesFS struct{}

func (drivesFS) List(ctx context.Context, p string) (vfs.Listing, error) {
	vols := drivesVolumes()
	names, by := drivesEntries(vols)
	p = drivesClean(p)
	if p == drivesRoot {
		l := vfs.Listing{Path: drivesRoot}
		for _, n := range names {
			v := by[strings.ToLower(n)]
			// the size column shows how much is free on it (the drive popup's "free of" in short)
			l.Entries = append(l.Entries, vfs.Entry{Name: n, Size: int64(v.Free), Flags: vfs.FlagDir | vfs.FlagNoAttr, Mode: uint32(fs.ModeDir | 0o755)})
		}
		return l, nil
	}
	v, ok := by[strings.ToLower(p[len(drivesRoot):])]
	if !ok {
		return vfs.Listing{}, &fs.PathError{Op: "list", Path: p, Err: fs.ErrNotExist}
	}
	// the volume's own top: the panel lands on the real folder
	return vfs.Local{}.List(ctx, v.Path)
}

func (drivesFS) Stat(p string) (vfs.Entry, error) {
	p = drivesClean(p)
	return vfs.Entry{Name: strings.TrimPrefix(p, drivesRoot), Flags: vfs.FlagDir | vfs.FlagNoAttr, Mode: uint32(fs.ModeDir | 0o755)}, nil
}

// drivesClean drops a trailing "/" (but keeps "drives://").
func drivesClean(p string) string {
	p = strings.TrimSpace(p)
	if strings.EqualFold(p, "drives:") || strings.EqualFold(p, drivesRoot) {
		return drivesRoot
	}
	return drivesRoot + strings.TrimRight(p[len(drivesRoot):], "/")
}

func (drivesFS) Clean(p string) string { return drivesClean(p) }

func (drivesFS) Parent(p string) string {
	if drivesClean(p) == drivesRoot {
		return ""
	}
	return drivesRoot
}

func (drivesFS) Root(string) string { return drivesRoot }

func (drivesFS) Join(dir, name string) string { return drivesRoot + name }
