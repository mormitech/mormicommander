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
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"

	"mormicommander/internal/dirsize"
	"mormicommander/internal/hang"
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
		if u, err := UsageOfQuick(home); err == nil {
			out = append(out, Volume{Name: "Home", Path: home, Kind: "home", Total: u.Total, Free: u.Free})
		}
	}
	// Each entry of /Volumes is asked in its own goroutine with a time limit (probeEach): a share whose server is gone
	// would otherwise block the whole drive bar. One that does not answer is listed as a stale network volume, since
	// only a remote or vanished file system hangs like that.
	entries, _ := os.ReadDir("/Volumes")
	paths := make([]string, len(entries))
	for i, e := range entries {
		paths[i] = filepath.Join("/Volumes", e.Name())
	}
	vols, errs := probeEach(paths, darwinMount)
	for i, p := range paths {
		switch {
		case errors.Is(errs[i], hang.ErrNotResponding):
			out = append(out, Volume{Name: entries[i].Name(), Path: p, Kind: "network", Stale: true})
		case errs[i] == nil && vols[i] != nil:
			v := *vols[i]
			if v.Name == "" {
				v.Name = entries[i].Name()
			}
			out = append(out, v)
		}
	}
	return out
}

// darwinMount describes the volume mounted at p, or nil when p is not a mount point (a link such as
// "/Volumes/Macintosh HD", which points to "/", or a plain folder).
func darwinMount(p string) (*Volume, error) {
	if info, err := os.Lstat(p); err != nil || info.Mode()&os.ModeSymlink != 0 {
		return nil, nil
	}
	var st unix.Statfs_t
	if unix.Statfs(p, &st) != nil || st.Blocks == 0 {
		return nil, nil
	}
	if cstr(st.Mntonname[:]) != p {
		return nil, nil
	}
	// A volume mounted "nobrowse" is one the system asks apps to leave out, as Finder does: the Recovery volume that
	// macOS mounts at /Volumes/Recovery while it prepares an update, for example. It can't be ejected from here either.
	if st.Flags&unix.MNT_DONTBROWSE != 0 {
		return nil, nil
	}
	bs := uint64(st.Bsize)
	kind := darwinKind(cstr(st.Fstypename[:]), st.Flags)
	v := &Volume{Path: p, Kind: kind, Total: st.Blocks * bs, Free: st.Bavail * bs}
	if kind == "network" {
		v.Name = shareLabel(filepath.Base(p), cstr(st.Mntfromname[:]))
		v.Host, v.Share = shareOf(cstr(st.Mntfromname[:]))
	}
	return v, nil
}

// shareLabel is the drive bar name of a network share mounted at a folder named folder from source
// ("//user@host/share"). macOS sometimes names the folder after the server instead of the share (share "z" of
// DiskStation.local mounts at /Volumes/DiskStation.local), so a folder named like the server shows the share's name;
// any other folder keeps its own name ("" means the folder name).
func shareLabel(folder, source string) string {
	host, share := shareOf(source)
	if share == "" {
		return ""
	}
	short, _, _ := strings.Cut(host, ".")
	if strings.EqualFold(folder, host) || strings.EqualFold(folder, short) {
		return share
	}
	return ""
}

// shareOf splits a network mount's source ("//user@host:port/share") into its server and share, without the user and
// port; "" for both when the source is not of that form or names a folder inside a share.
func shareOf(source string) (host, share string) {
	rest, ok := strings.CutPrefix(source, "//")
	if !ok {
		return "", ""
	}
	host, share, _ = strings.Cut(rest, "/")
	if i := strings.LastIndex(host, "@"); i >= 0 {
		host = host[i+1:]
	}
	if j := strings.LastIndex(host, ":"); j >= 0 && !strings.Contains(host[j:], "]") {
		host = host[:j]
	}
	share = strings.Trim(share, "/")
	if share == "" || strings.Contains(share, "/") {
		return "", ""
	}
	if dec, err := url.PathUnescape(share); err == nil {
		share = dec
	}
	return host, share
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

// VolumeRootOf is the mount folder of the /Volumes volume p is on ("/Volumes/music" for "/Volumes/music/flac"), or ""
// when p is not on one (the startup disk, a plain folder in /Volumes). A panel treats that folder as the top of a
// drive of its own, as the drive bar shows it: there is no ".." above it. A volume that does not answer in time counts
// as none.
func VolumeRootOf(p string) string {
	rest, ok := strings.CutPrefix(filepath.Clean(p), "/Volumes/")
	if !ok || rest == "" {
		return ""
	}
	root := "/Volumes/" + strings.SplitN(rest, "/", 2)[0]
	vols, errs := probeEach([]string{root}, darwinMount)
	if errs[0] != nil || vols[0] == nil {
		return ""
	}
	return root
}
