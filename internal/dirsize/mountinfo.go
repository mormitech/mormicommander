// Linux describes its mounts in /proc/self/mountinfo and names labelled disks with links in /dev/disk/by-label. This
// file parses both formats. It has no build tag on purpose: the parsing is plain text work, so its tests run on every
// development machine, not only on Linux, while volume_linux.go feeds it the real files. Mount points in mountinfo
// escape spaces and other special bytes as octal (\040), and udev escapes them in label names as hex (\x20); both are
// decoded so that a USB stick labelled "My Disk" shows as "My Disk".
package dirsize

import (
	"bufio"
	"io"
	"strconv"
	"strings"
)

// mountEntry is one line of /proc/self/mountinfo.
type mountEntry struct {
	Major, Minor uint64
	MountPoint   string
	FSType       string
	Source       string
	Options      string // per-mount options, e.g. "ro,nosuid"
}

// parseMountinfo reads the mountinfo format; malformed lines are skipped.
func parseMountinfo(r io.Reader) []mountEntry {
	var out []mountEntry
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 10 {
			continue
		}
		sep := -1
		for i := 6; i < len(f); i++ {
			if f[i] == "-" {
				sep = i
				break
			}
		}
		if sep < 0 || sep+2 >= len(f) {
			continue
		}
		mm := strings.SplitN(f[2], ":", 2)
		if len(mm) != 2 {
			continue
		}
		maj, err1 := strconv.ParseUint(mm[0], 10, 32)
		min, err2 := strconv.ParseUint(mm[1], 10, 32)
		if err1 != nil || err2 != nil {
			continue
		}
		out = append(out, mountEntry{
			Major: maj, Minor: min,
			MountPoint: unescapeOctal(f[4]),
			Options:    f[5],
			FSType:     f[sep+1],
			Source:     unescapeOctal(f[sep+2]),
		})
	}
	return out
}

// findMount picks the mount that holds path: the longest mount point that is a prefix of path, preferring mounts whose
// device numbers match the path's (bind mounts and overmounts make the prefix alone ambiguous), and on equal length
// the one mounted last, which hides the earlier ones.
func findMount(mounts []mountEntry, path string, major, minor uint64, haveDev bool) (mountEntry, bool) {
	best, bestScore := mountEntry{}, -1
	for _, m := range mounts {
		if !under(path, m.MountPoint) {
			continue
		}
		score := len(m.MountPoint) * 2
		if haveDev && m.Major == major && m.Minor == minor {
			score += 1 << 20
		}
		if score >= bestScore {
			best, bestScore = m, score
		}
	}
	return best, bestScore >= 0
}

// under reports whether path is mount point mp or inside it.
func under(path, mp string) bool {
	if mp == "/" {
		return strings.HasPrefix(path, "/")
	}
	return path == mp || strings.HasPrefix(path, mp+"/")
}

// unescapeOctal decodes mountinfo's \ooo escapes.
func unescapeOctal(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			if n, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(n))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// unescapeHex decodes udev's \xHH escapes in /dev/disk/by-label names.
func unescapeHex(s string) string {
	if !strings.Contains(s, `\x`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) && s[i+1] == 'x' {
			if n, err := strconv.ParseUint(s[i+2:i+4], 16, 8); err == nil {
				b.WriteByte(byte(n))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// networkFS are file system types that live on another machine.
var networkFS = map[string]bool{
	"nfs": true, "nfs4": true, "cifs": true, "smb3": true, "smbfs": true, "sshfs": true, "fuse.sshfs": true,
	"davfs": true, "fuse.davfs2": true, "fuse.rclone": true, "9p": true, "afs": true, "ceph": true, "glusterfs": true,
	"fuse.glusterfs": true, "fuse.s3fs": true, "fuse.gvfsd-fuse": true,
}
