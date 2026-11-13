// Tests for the macOS filesystem type classification that decides between kqueue and polling.
package fswatch

import "testing"

func TestIsNetworkFSName(t *testing.T) {
	cases := map[string]bool{
		"apfs": false, "hfs": false, "msdos": false, "exfat": false, "devfs": false,
		"smbfs": true, "nfs": true, "afpfs": true, "webdav": true, "macfuse": true, "osxfuse": true, "fuse-t": true,
		"SMBFS": true,
	}
	for name, want := range cases {
		if got := isNetworkFSName(name); got != want {
			t.Errorf("isNetworkFSName(%q) = %v, want %v", name, got, want)
		}
	}
	if isNetworkPath(t.TempDir()) {
		t.Error("the temp folder should be local")
	}
}
