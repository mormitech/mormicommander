// Tests for Linux free-space arithmetic: block counts are in f_frsize units,
// not f_bsize, which matters on mounts where the two differ.

package disk

import (
	"testing"

	"golang.org/x/sys/unix"
)

func TestUsageFromStatfsUsesFrsize(t *testing.T) {
	tests := []struct {
		name                string
		st                  unix.Statfs_t
		wantTotal, wantFree uint64
	}{
		{"equal", unix.Statfs_t{Bsize: 4096, Frsize: 4096, Blocks: 100, Bavail: 10}, 409600, 40960},
		{"nfs-like", unix.Statfs_t{Bsize: 1 << 20, Frsize: 4096, Blocks: 100, Bavail: 10}, 409600, 40960},
		{"no-frsize", unix.Statfs_t{Bsize: 512, Blocks: 100, Bavail: 10}, 51200, 5120},
	}
	for _, tt := range tests {
		u := usageFromStatfs(&tt.st)
		if u.Total != tt.wantTotal || u.Free != tt.wantFree {
			t.Errorf("%s: got %+v, want total %d free %d", tt.name, u, tt.wantTotal, tt.wantFree)
		}
	}
}
