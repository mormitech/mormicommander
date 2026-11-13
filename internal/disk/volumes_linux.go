package disk

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

var localFS = map[string]bool{
	"ext2": true, "ext3": true, "ext4": true, "xfs": true, "btrfs": true, "f2fs": true, "zfs": true, "jfs": true,
	"ntfs": true, "ntfs3": true, "vfat": true, "exfat": true, "fuseblk": true, "hfsplus": true, "iso9660": true, "udf": true,
}

var netFS = map[string]bool{"nfs": true, "nfs4": true, "cifs": true, "smb3": true, "sshfs": true, "fuse.sshfs": true, "davfs": true, "fuse.rclone": true}

// Volumes lists "/" first, the home folder, then every mounted disk and network share, once per device.
func Volumes() []Volume {
	out := rootOnly()
	if home, err := os.UserHomeDir(); err == nil {
		if u, err := UsageOf(home); err == nil {
			out = append(out, Volume{Name: "Home", Path: home, Kind: "home", Total: u.Total, Free: u.Free})
		}
	}
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return out
	}
	defer f.Close()
	seen := map[string]bool{"/": true}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 3 {
			continue
		}
		dev, mnt, typ := fields[0], unescapeMount(fields[1]), fields[2]
		if !localFS[typ] && !netFS[typ] || seen[mnt] || seen[dev] {
			continue
		}
		if strings.HasPrefix(mnt, "/snap") || strings.HasPrefix(mnt, "/var/lib/docker") || strings.HasPrefix(mnt, "/boot") {
			continue
		}
		u, err := UsageOf(mnt)
		if err != nil || u.Total == 0 {
			continue
		}
		seen[dev], seen[mnt] = true, true
		kind := "fixed"
		switch {
		case netFS[typ]:
			kind = "network"
		case typ == "iso9660" || typ == "udf":
			kind = "cdrom"
		case strings.HasPrefix(mnt, "/media/") || strings.HasPrefix(mnt, "/run/media/") || strings.HasPrefix(mnt, "/mnt/"):
			kind = "removable"
		}
		out = append(out, Volume{Name: filepath.Base(mnt), Path: mnt, Kind: kind, Total: u.Total, Free: u.Free})
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
