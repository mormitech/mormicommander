package disk

import "testing"

func TestVolumeRootOfLeavesNonVolumesAlone(t *testing.T) {
	for _, p := range []string{"/", "/Users", "/Volumes", "/Volumes/no such volume/x"} {
		if r := VolumeRootOf(p); r != "" {
			t.Errorf("VolumeRootOf(%q) = %q, want none", p, r)
		}
	}
}
