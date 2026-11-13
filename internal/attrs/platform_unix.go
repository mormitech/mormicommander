//go:build unix

// The Unix side (macOS, Linux and the BSDs) of reading and changing
// attributes. Here the real attributes are the permission bits, the numeric
// owner and group, three or four time stamps and, on macOS, the BSD file
// flags. The Windows-style attribute set is derived from them: a file whose
// owner cannot write it counts as read-only, and a dot name or the macOS
// hidden flag counts as hidden. Setting "read-only" therefore clears every
// write bit and clearing it gives the owner write permission back, which is
// what a TC user switching platforms would expect. Things that differ between
// macOS and Linux (birth time, the hidden flag, setting the creation date) are
// in the extras_*.go files.
package attrs

import (
	"io/fs"
	"os"
	"os/user"
	"strconv"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

func fillPlatform(info *Info, _ fs.FileInfo, follow bool) {
	var st unix.Stat_t
	var err error
	if follow {
		err = unix.Stat(info.Path, &st)
	} else {
		err = unix.Lstat(info.Path, &st)
	}
	if err != nil {
		info.Win = deriveWinAttr(info)
		return
	}
	info.UID = int(st.Uid)
	info.GID = int(st.Gid)
	info.Owner = userName(info.UID)
	info.Group = groupName(info.GID)
	info.Links = uint64(st.Nlink)
	info.Inode = uint64(st.Ino)
	info.Device = uint64(st.Dev)
	info.SizeOnDisk = int64(st.Blocks) * 512
	info.Accessed = time.Unix(st.Atim.Unix())
	info.HasAccessed = true
	info.Changed = time.Unix(st.Ctim.Unix())
	info.HasChanged = true
	fillExtras(info, &st, follow)
	info.Win = deriveWinAttr(info)
}

// deriveWinAttr builds the Windows-style view of a Unix entry.
func deriveWinAttr(info *Info) WinAttr {
	var a WinAttr
	if info.Kind == KindDir {
		a |= AttrDirectory
	}
	if info.Kind == KindSymlink {
		a |= AttrReparsePoint
	}
	if info.Kind != KindSymlink && info.Mode&0o200 == 0 {
		a |= AttrReadOnly
	}
	if info.Hidden || info.HiddenFlag {
		a |= AttrHidden
	}
	return a
}

// unixWinAttr computes the derived attribute set from a planned state, used
// by the change preview.
func unixWinAttr(kind Kind, perm fs.FileMode, hidden bool) WinAttr {
	i := Info{Kind: kind, Mode: perm, Hidden: hidden}
	return deriveWinAttr(&i)
}

var (
	userCache  sync.Map // int -> string
	groupCache sync.Map
)

// userName turns a numeric owner into a name, caching the answer because a
// panel full of files asks the same question thousands of times.
func userName(uid int) string {
	if v, ok := userCache.Load(uid); ok {
		return v.(string)
	}
	name := strconv.Itoa(uid)
	if u, err := user.LookupId(name); err == nil && u.Username != "" {
		name = u.Username
	}
	userCache.Store(uid, name)
	return name
}

func groupName(gid int) string {
	if v, ok := groupCache.Load(gid); ok {
		return v.(string)
	}
	name := strconv.Itoa(gid)
	if g, err := user.LookupGroupId(name); err == nil && g.Name != "" {
		name = g.Name
	}
	groupCache.Store(gid, name)
	return name
}

// lookupUID accepts a user name or a number.
func lookupUID(s string) (int, error) {
	if n, err := strconv.Atoi(s); err == nil && n >= 0 {
		return n, nil
	}
	u, err := user.Lookup(s)
	if err != nil {
		return -1, err
	}
	return strconv.Atoi(u.Uid)
}

// lookupGID accepts a group name or a number.
func lookupGID(s string) (int, error) {
	if n, err := strconv.Atoi(s); err == nil && n >= 0 {
		return n, nil
	}
	g, err := user.LookupGroup(s)
	if err != nil {
		return -1, err
	}
	return strconv.Atoi(g.Gid)
}

func chmodPath(path string, perm fs.FileMode) error {
	return os.Chmod(path, perm&PermMask)
}

func chownPath(path string, uid, gid int, follow bool) error {
	if follow {
		return os.Chown(path, uid, gid)
	}
	return os.Lchown(path, uid, gid)
}

// setWinAttrs has nothing to write on Unix: read-only becomes a permission
// change and hidden becomes the macOS flag, both handled by the planner.
func setWinAttrs(string, WinAttr, bool) error { return ErrUnsupported }

// setTimes sets modification and access times with nanosecond precision and,
// where the platform allows it, the creation time. A nil pointer keeps that
// time. The creation time goes last because macOS moves it back whenever the
// modification time is set earlier than it.
func setTimes(path string, mtime, atime, ctime *time.Time, follow bool) error {
	if mtime != nil || atime != nil {
		var st unix.Stat_t
		var err error
		if follow {
			err = unix.Stat(path, &st)
		} else {
			err = unix.Lstat(path, &st)
		}
		if err != nil {
			return &os.PathError{Op: "stat", Path: path, Err: err}
		}
		ts := []unix.Timespec{st.Atim, st.Mtim}
		if atime != nil {
			ts[0] = unix.NsecToTimespec(atime.UnixNano())
		}
		if mtime != nil {
			ts[1] = unix.NsecToTimespec(mtime.UnixNano())
		}
		flags := 0
		if !follow {
			flags = unix.AT_SYMLINK_NOFOLLOW
		}
		if err := unix.UtimesNanoAt(unix.AT_FDCWD, path, ts, flags); err != nil {
			return &os.PathError{Op: "utimes", Path: path, Err: err}
		}
	}
	if ctime != nil {
		if err := setCreated(path, *ctime, follow); err != nil {
			return &os.PathError{Op: "set creation time", Path: path, Err: err}
		}
	}
	return nil
}

func platformCaps() Capabilities {
	c := Capabilities{
		UnixPerms:     true,
		Owner:         true,
		SettableAttrs: AttrReadOnly,
		ReportedAttrs: AttrReadOnly | AttrHidden | AttrDirectory | AttrReparsePoint,
		CreatedTime:   canSetCreated,
		ReadCreated:   canReadCreated,
		HiddenFlag:    canHideFlag,
	}
	if canHideFlag {
		c.SettableAttrs |= AttrHidden
	}
	return c
}

// ownerLabel names a numeric owner for the preview.
func ownerLabel(uid int) string { return userName(uid) }

// currentUID is the user the program runs as, for deciding whether a
// permission change opens a folder to us.
func currentUID() int { return os.Getuid() }
