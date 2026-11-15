// Tests for the macOS drive list: the startup disk is named after the
// /Volumes link Finder keeps for it (not a hard-coded "Macintosh HD"), and
// volume kinds come from the statfs flags and file system type, so internal
// second disks are not called removable.

package disk

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestDarwinKind(t *testing.T) {
	local := uint32(unix.MNT_LOCAL)
	tests := []struct {
		fstype string
		flags  uint32
		want   string
	}{
		{"apfs", local, "fixed"},
		{"hfs", local, "fixed"},
		{"apfs", local | unix.MNT_REMOVABLE, "removable"},
		{"msdos", local, "removable"},
		{"exfat", local, "removable"},
		{"smbfs", 0, "network"},
		{"nfs", 0, "network"},
		{"apfs", 0, "network"},
		{"cd9660", local, "cdrom"},
		{"udf", local | unix.MNT_REMOVABLE, "cdrom"},
	}
	for _, tt := range tests {
		if got := darwinKind(tt.fstype, tt.flags); got != tt.want {
			t.Errorf("darwinKind(%q, %#x) = %q, want %q", tt.fstype, tt.flags, got, tt.want)
		}
	}
}

func TestStartupDiskName(t *testing.T) {
	vols := Volumes()
	if len(vols) == 0 || vols[0].Path != "/" {
		t.Fatalf("first volume is not the startup disk: %+v", vols)
	}
	want := ""
	entries, _ := os.ReadDir("/Volumes")
	for _, e := range entries {
		if tgt, err := os.Readlink(filepath.Join("/Volumes", e.Name())); err == nil && tgt == "/" {
			want = e.Name()
		}
	}
	if want == "" {
		t.Skip("no /Volumes link to / on this machine")
	}
	if vols[0].Name != want {
		t.Errorf("startup disk name = %q, want %q", vols[0].Name, want)
	}
	if vols[0].Kind != "fixed" {
		t.Errorf("startup disk kind = %q", vols[0].Kind)
	}
}

// macOS can name a share's mount folder after the server (share "z" of DiskStation.local at
// /Volumes/DiskStation.local); the drive bar then shows the share's name. Other folders keep their own name.
func TestShareLabel(t *testing.T) {
	for _, c := range []struct{ folder, source, want string }{
		{"DiskStation.local", "//GUEST@DiskStation.local/z", "z"},
		{"DiskStation", "//pat@DiskStation.local/z", "z"},
		{"DiskStation.local", "//GUEST:@DiskStation.local:445/my%20files", "my files"},
		{"music", "//GUEST:@DiskStation.local/music", ""},
		{"music-1", "//GUEST@other.local/music", ""},
		{"DiskStation.local", "//GUEST@DiskStation.local/", ""},
		{"nas", "nas:/export/media", ""},
	} {
		if got := shareLabel(c.folder, c.source); got != c.want {
			t.Errorf("shareLabel(%q, %q) = %q, want %q", c.folder, c.source, got, c.want)
		}
	}
}
