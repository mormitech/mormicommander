// On Linux the drive bar shows "/" first, then the home folder (Linux has no
// drive letters, and home is where users go most), then every real disk and
// network share from /proc/mounts, once per device. Pseudo file systems
// (proc, tmpfs, overlay ...) and noise such as snap and docker mounts are
// left out. The kind is guessed from the file system type and the mount
// point: network types are shares, iso9660/udf are optical discs, and mounts
// under /media, /run/media or /mnt are where desktops put removable media.

package disk

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"mormicommander/internal/hang"
	"mormicommander/internal/netbrowse"
)

var localFS = map[string]bool{
	"ext2": true, "ext3": true, "ext4": true, "xfs": true, "btrfs": true, "f2fs": true, "zfs": true, "jfs": true,
	"ntfs": true, "ntfs3": true, "vfat": true, "exfat": true, "fuseblk": true, "hfsplus": true, "iso9660": true, "udf": true,
}

var netFS = map[string]bool{"gvfs": true, "nfs": true, "nfs4": true, "cifs": true, "smb3": true, "sshfs": true, "fuse.sshfs": true, "davfs": true, "fuse.rclone": true}

// Volumes lists "/" first, the home folder, then every mounted disk and network share, once per device.
func Volumes() []Volume {
	out := rootOnly()
	if home, err := os.UserHomeDir(); err == nil {
		if u, err := UsageOfQuick(home); err == nil {
			out = append(out, Volume{Name: "Home", Path: home, Kind: "home", Total: u.Total, Free: u.Free})
		}
	}
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return out
	}
	defer f.Close()
	type mount struct{ dev, mnt, typ string }
	var mounts []mount
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 3 {
			continue
		}
		dev, mnt, typ := fields[0], unescapeMount(fields[1]), fields[2]
		if typ == "fuse.gvfsd-fuse" {
			// GVfs (gio mount, the desktop's "Other locations") shows every share as a folder of its one FUSE mount
			for _, g := range gvfsShares(mnt) {
				mounts = append(mounts, mount{g, g, "gvfs"})
			}
			continue
		}
		if !localFS[typ] && !netFS[typ] || mnt == "/" {
			continue
		}
		if strings.HasPrefix(mnt, "/snap") || strings.HasPrefix(mnt, "/var/lib/docker") || strings.HasPrefix(mnt, "/boot") {
			continue
		}
		mounts = append(mounts, mount{dev, mnt, typ})
	}
	// statfs on every mount at the same time, each with a time limit (probeEach): an NFS or CIFS share whose server is
	// gone is listed as stale instead of blocking the drive bar.
	paths := make([]string, len(mounts))
	for i, m := range mounts {
		paths[i] = m.mnt
	}
	usages, errs := probeEach(paths, UsageOf)
	seen := map[string]bool{"/": true} // once per device and mount point, the first one that answers wins
	for i, m := range mounts {
		if seen[m.mnt] || seen[m.dev] {
			continue
		}
		kind := "fixed"
		switch {
		case netFS[m.typ]:
			kind = "network"
		case m.typ == "iso9660" || m.typ == "udf":
			kind = "cdrom"
		case strings.HasPrefix(m.mnt, "/media/") || strings.HasPrefix(m.mnt, "/run/media/") || strings.HasPrefix(m.mnt, "/mnt/"):
			kind = "removable"
		}
		v := Volume{Name: filepath.Base(m.mnt), Path: m.mnt, Kind: kind}
		if m.typ == "gvfs" {
			v.Name = netbrowse.GVfsName(filepath.Base(m.mnt))
		}
		switch {
		case errors.Is(errs[i], hang.ErrNotResponding):
			v.Stale = true
		case errs[i] != nil || usages[i].Total == 0:
			continue
		default:
			v.Total, v.Free = usages[i].Total, usages[i].Free
		}
		seen[m.dev], seen[m.mnt] = true, true
		out = append(out, v)
	}
	return out
}

// gvfsShares lists the network shares inside the GVfs FUSE folder (smb-share:..., nfs:..., afp-volume:...); other
// GVfs locations such as phones (mtp:) are not drives.
func gvfsShares(root string) []string {
	des, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []string
	for _, d := range des {
		if netbrowse.GVfsURI(d.Name()) != "" {
			out = append(out, filepath.Join(root, d.Name()))
		}
	}
	return out
}

// unescapeMount undoes the octal escapes of /proc/mounts (a space is \040).
func unescapeMount(s string) string {
	r := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	return r.Replace(s)
}

func rootOnly() []Volume {
	u, err := UsageOf("/")
	if err != nil {
		return nil
	}
	return []Volume{{Name: "Root", Path: "/", Kind: "fixed", Total: u.Total, Free: u.Free}}
}
