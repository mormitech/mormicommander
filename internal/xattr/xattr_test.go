// Tests of the parts of the xattr package that need no file system support: which attributes are worth a warning
// and which file systems drop extended attributes.
package xattr

import "testing"

func TestDropsXattrs(t *testing.T) {
	for fs, want := range map[string]bool{
		"msdos": true, "exfat": true, "vfat": true, "FAT32": true, " exFAT ": true, "cd9660": true,
		"apfs": false, "hfs": false, "ext4": false, "NTFS": false, "smbfs": false, "btrfs": false, "": false,
	} {
		if got := DropsXattrs(fs); got != want {
			t.Errorf("DropsXattrs(%q) = %v, want %v", fs, got, want)
		}
	}
}

func TestWorth(t *testing.T) {
	for name, want := range map[string]bool{
		Quarantine: false, "com.apple.provenance": false, "com.apple.lastuseddate#PS": false,
		"com.apple.metadata:kMDItemWhereFroms": false, "security.selinux": false, "com.apple.macl": false,
		"com.apple.metadata:_kMDItemUserTags": true, "com.apple.FinderInfo": true, "com.apple.ResourceFork": true, "user.note": true,
	} {
		if got := Worth(name); got != want {
			t.Errorf("Worth(%q) = %v, want %v", name, got, want)
		}
	}
}
