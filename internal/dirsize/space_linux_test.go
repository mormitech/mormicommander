package dirsize

import (
	"testing"

	"golang.org/x/sys/unix"
)

// Block counts are in f_frsize units, not f_bsize, which matters on mounts where the two differ (moved here from
// internal/disk, board #102).
func TestStatfsBlockUsesFrsize(t *testing.T) {
	for _, tt := range []struct {
		st   unix.Statfs_t
		want uint64
	}{
		{unix.Statfs_t{Bsize: 4096, Frsize: 4096}, 4096},
		{unix.Statfs_t{Bsize: 1 << 20, Frsize: 4096}, 4096},
		{unix.Statfs_t{Bsize: 512}, 512},
	} {
		if got := statfsBlock(&tt.st); got != tt.want {
			t.Errorf("%+v: %d, want %d", tt.st, got, tt.want)
		}
	}
}
