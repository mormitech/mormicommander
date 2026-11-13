// Linux extras. Modern Linux file systems (ext4, btrfs, xfs) record a birth
// time, but the classic stat(2) call does not return it; statx(2) does, when
// both the kernel and the file system support it, so we ask statx and simply
// report "unknown" when it is missing. Linux offers no way at all to set the
// birth time and has no hidden flag (hidden means a dot name), so those
// features report themselves as unsupported and the dialog greys them out.
package attrs

import (
	"errors"
	"time"

	"golang.org/x/sys/unix"
)

const (
	canSetCreated  = false
	canReadCreated = true
	canHideFlag    = false
)

func fillExtras(info *Info, _ *unix.Stat_t, follow bool) {
	flags := unix.AT_SYMLINK_NOFOLLOW
	if follow {
		flags = 0
	}
	var stx unix.Statx_t
	if err := unix.Statx(unix.AT_FDCWD, info.Path, flags, unix.STATX_BTIME, &stx); err != nil {
		return
	}
	if stx.Mask&unix.STATX_BTIME != 0 {
		info.Created = time.Unix(stx.Btime.Sec, int64(stx.Btime.Nsec))
		info.HasCreated = true
	}
}

func setCreated(string, time.Time, bool) error {
	return errors.New("the creation date cannot be changed on Linux")
}

func setHiddenFlag(string, bool, bool) error { return ErrUnsupported }

func extraAccounts(bool) []Account { return nil }
