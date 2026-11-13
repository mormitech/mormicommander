package dirsize

import (
	"strings"
	"testing"
)

const sampleMountinfo = `22 1 259:2 / / rw,relatime shared:1 - ext4 /dev/nvme0n1p2 rw,errors=remount-ro
23 22 0:21 / /proc rw,nosuid,nodev,noexec,relatime shared:12 - proc proc rw
24 22 259:1 / /boot/efi rw,relatime shared:2 - vfat /dev/nvme0n1p1 rw,fmask=0077
40 22 8:17 / /media/jp/My\040Disk rw,nosuid,nodev,relatime shared:300 - exfat /dev/sdb1 rw,uid=1000
41 22 0:50 / /mnt/share rw,relatime shared:301 - cifs //nas/share rw,vers=3.1.1
42 22 259:2 /home/jp/data /srv/data rw,relatime shared:1 - ext4 /dev/nvme0n1p2 rw
43 40 0:60 / /media/jp/My\040Disk/inner rw shared:302 - tmpfs tmpfs rw
bad line
44 22 x:y / /bad rw - ext4 /dev/x rw
`

func TestParseMountinfo(t *testing.T) {
	m := parseMountinfo(strings.NewReader(sampleMountinfo))
	if len(m) != 7 {
		t.Fatalf("parsed %d entries, want 7: %+v", len(m), m)
	}
	usb := m[3]
	if usb.MountPoint != "/media/jp/My Disk" || usb.FSType != "exfat" || usb.Source != "/dev/sdb1" || usb.Major != 8 || usb.Minor != 17 {
		t.Errorf("usb entry %+v", usb)
	}
	if m[4].FSType != "cifs" || !networkFS[m[4].FSType] || m[4].Source != "//nas/share" {
		t.Errorf("cifs entry %+v", m[4])
	}
}

func TestFindMount(t *testing.T) {
	m := parseMountinfo(strings.NewReader(sampleMountinfo))
	tests := []struct {
		path         string
		major, minor uint64
		haveDev      bool
		want         string
	}{
		{"/home/jp", 259, 2, true, "/"},
		{"/", 259, 2, true, "/"},
		{"/proc/self", 0, 21, true, "/proc"},
		{"/processes", 259, 2, true, "/"}, // "/proc" is not a prefix of "/processes"
		{"/boot/efi/EFI", 259, 1, true, "/boot/efi"},
		{"/media/jp/My Disk/photos", 8, 17, true, "/media/jp/My Disk"},
		{"/media/jp/My Disk/inner/x", 0, 60, true, "/media/jp/My Disk/inner"},
		{"/mnt/share/docs", 0, 0, false, "/mnt/share"},
		{"/srv/data/x", 259, 2, true, "/srv/data"},
	}
	for _, tc := range tests {
		got, ok := findMount(m, tc.path, tc.major, tc.minor, tc.haveDev)
		if !ok || got.MountPoint != tc.want {
			t.Errorf("findMount(%q) = %q, %v; want %q", tc.path, got.MountPoint, ok, tc.want)
		}
	}
	if _, ok := findMount(nil, "/x", 0, 0, false); ok {
		t.Error("findMount on no mounts reported a mount")
	}
}

func TestUnescape(t *testing.T) {
	tests := []struct{ in, octal, hex string }{
		{"plain", "plain", "plain"},
		{`My\040Disk`, "My Disk", `My\040Disk`},
		{`My\x20Disk`, `My\x20Disk`, "My Disk"},
		{`a\134b`, `a\b`, `a\134b`},
		{`tail\04`, `tail\04`, `tail\04`},
		{`tail\x2`, `tail\x2`, `tail\x2`},
		{`\x2fslash\x`, `\x2fslash\x`, `/slash\x`},
	}
	for _, tc := range tests {
		if got := unescapeOctal(tc.in); got != tc.octal {
			t.Errorf("unescapeOctal(%q) = %q, want %q", tc.in, got, tc.octal)
		}
		if got := unescapeHex(tc.in); got != tc.hex {
			t.Errorf("unescapeHex(%q) = %q, want %q", tc.in, got, tc.hex)
		}
	}
}
