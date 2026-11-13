// macOS extras. APFS and HFS+ record a creation ("birth") date that Finder
// shows and that can be changed, and they keep BSD file flags, of which
// UF_HIDDEN is the one Finder uses to hide a file without renaming it. Both
// are written with setattrlist(2), because unlike chflags(2) and utimes(2) it
// can act on a symbolic link itself (FSOPT_NOFOLLOW) and it is the only call
// that can set the creation date. The account list for the owner and group
// boxes comes from Directory Services, since /etc/passwd on a Mac lists only
// system accounts.
package attrs

import (
	"context"
	"os/exec"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	canSetCreated  = true
	canReadCreated = true
	canHideFlag    = true
)

func fillExtras(info *Info, st *unix.Stat_t, _ bool) {
	info.Flags = st.Flags
	info.HiddenFlag = st.Flags&unix.UF_HIDDEN != 0
	if st.Btim.Sec != 0 || st.Btim.Nsec != 0 {
		info.Created = time.Unix(st.Btim.Unix())
		info.HasCreated = true
	}
}

func setattrOptions(follow bool) int {
	if follow {
		return 0
	}
	return unix.FSOPT_NOFOLLOW
}

// setCreated writes the birth time with setattrlist and ATTR_CMN_CRTIME.
func setCreated(path string, t time.Time, follow bool) error {
	ts := unix.NsecToTimespec(t.UnixNano())
	buf := unsafe.Slice((*byte)(unsafe.Pointer(&ts)), unsafe.Sizeof(ts))
	al := unix.Attrlist{Bitmapcount: unix.ATTR_BIT_MAP_COUNT, Commonattr: unix.ATTR_CMN_CRTIME}
	return unix.Setattrlist(path, &al, buf, setattrOptions(follow))
}

// setHiddenFlag switches UF_HIDDEN on or off, keeping all other flags.
func setHiddenFlag(path string, on bool, follow bool) error {
	var st unix.Stat_t
	var err error
	if follow {
		err = unix.Stat(path, &st)
	} else {
		err = unix.Lstat(path, &st)
	}
	if err != nil {
		return err
	}
	flags := st.Flags
	if on {
		flags |= unix.UF_HIDDEN
	} else {
		flags &^= unix.UF_HIDDEN
	}
	if flags == st.Flags {
		return nil
	}
	buf := unsafe.Slice((*byte)(unsafe.Pointer(&flags)), unsafe.Sizeof(flags))
	al := unix.Attrlist{Bitmapcount: unix.ATTR_BIT_MAP_COUNT, Commonattr: unix.ATTR_CMN_FLAGS}
	return unix.Setattrlist(path, &al, buf, setattrOptions(follow))
}

// extraAccounts asks Directory Services for local users or groups. A short
// timeout keeps a misbehaving directory from freezing the dialog.
func extraAccounts(groups bool) []Account {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	what, key := "/Users", "UniqueID"
	if groups {
		what, key = "/Groups", "PrimaryGroupID"
	}
	out, err := exec.CommandContext(ctx, "dscl", ".", "-list", what, key).Output()
	if err != nil {
		return nil
	}
	return parseDscl(out)
}
