//go:build windows

// Windows has three kinds of file-system link and each needs its own call.
// Symbolic links are made with CreateSymbolicLink, which must be told up
// front whether the target is a folder, and which needs either Developer Mode
// (the ALLOW_UNPRIVILEGED_CREATE flag) or administrator rights; older builds
// reject that flag, so we retry without it. Junctions (TC's "Create junction"
// for folders) need no privilege but have no API of their own: we create an
// empty folder and write a mount-point reparse buffer into it with
// FSCTL_SET_REPARSE_POINT. File identity for loop detection and the hard link
// count come from GetFileInformationByHandle (volume serial plus file index).

package links

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

const symlinkAllowUnprivileged = 0x2 // SYMBOLIC_LINK_FLAG_ALLOW_UNPRIVILEGED_CREATE

func platformSymlink(target, link string, isDir bool) error {
	target = filepath.FromSlash(target)
	lp, err := windows.UTF16PtrFromString(link)
	if err != nil {
		return &os.LinkError{Op: "symlink", Old: target, New: link, Err: err}
	}
	tp, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return &os.LinkError{Op: "symlink", Old: target, New: link, Err: err}
	}
	var flags uint32
	if isDir {
		flags |= windows.SYMBOLIC_LINK_FLAG_DIRECTORY
	}
	err = windows.CreateSymbolicLink(lp, tp, flags|symlinkAllowUnprivileged)
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		err = windows.CreateSymbolicLink(lp, tp, flags)
	}
	if err != nil {
		return &os.LinkError{Op: "symlink", Old: target, New: link, Err: err}
	}
	return nil
}

// platformJunction creates link as an NTFS junction pointing at target,
// which must be an absolute path on a local drive.
func platformJunction(target, link string) error {
	fail := func(err error) error { return &os.LinkError{Op: "junction", Old: target, New: link, Err: err} }
	abs, err := filepath.Abs(target)
	if err != nil {
		return fail(err)
	}
	abs = filepath.Clean(abs)
	if strings.HasPrefix(abs, `\\`) {
		return fail(fmt.Errorf("junctions can only point to local drives: %w", ErrUnsupported))
	}
	if err := os.Mkdir(link, 0o755); err != nil {
		return fail(err)
	}
	if err := setMountPoint(link, abs); err != nil {
		os.Remove(link)
		return fail(err)
	}
	return nil
}

func setMountPoint(dir, target string) error {
	sub, err := windows.UTF16FromString(`\??\` + target) // includes trailing NUL
	if err != nil {
		return err
	}
	prn, err := windows.UTF16FromString(target)
	if err != nil {
		return err
	}
	subBytes := (len(sub) - 1) * 2
	prnBytes := (len(prn) - 1) * 2
	pathBuf := (len(sub) + len(prn)) * 2
	dataLen := 8 + pathBuf
	buf := make([]byte, 8+dataLen)
	le := binary.LittleEndian
	le.PutUint32(buf[0:], windows.IO_REPARSE_TAG_MOUNT_POINT)
	le.PutUint16(buf[4:], uint16(dataLen))
	le.PutUint16(buf[8:], 0)                   // SubstituteNameOffset
	le.PutUint16(buf[10:], uint16(subBytes))   // SubstituteNameLength
	le.PutUint16(buf[12:], uint16(subBytes+2)) // PrintNameOffset
	le.PutUint16(buf[14:], uint16(prnBytes))   // PrintNameLength
	off := 16
	for _, c := range sub {
		le.PutUint16(buf[off:], c)
		off += 2
	}
	for _, c := range prn {
		le.PutUint16(buf[off:], c)
		off += 2
	}
	p, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return err
	}
	h, err := windows.CreateFile(p, windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	var ret uint32
	return windows.DeviceIoControl(h, windows.FSCTL_SET_REPARSE_POINT, &buf[0], uint32(len(buf)), nil, 0, &ret, nil)
}

func handleInfo(path string) (*windows.ByHandleFileInformation, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(p, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(h)
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

func fileIdentity(path string, fi fs.FileInfo) (FileID, bool) {
	info, err := handleInfo(path)
	if err != nil {
		return FileID{}, false
	}
	return FileID{Dev: uint64(info.VolumeSerialNumber), Ino: uint64(info.FileIndexHigh)<<32 | uint64(info.FileIndexLow)}, true
}

func linkCount(path string, fi fs.FileInfo) uint64 {
	info, err := handleInfo(path)
	if err != nil {
		return 0
	}
	return uint64(info.NumberOfLinks)
}

// isJunction reports whether path (described by its Lstat info) is a
// mount-point reparse point. Go reports those as ModeIrregular, not as
// ModeSymlink, so they need this extra check.
func isJunction(path string, fi fs.FileInfo) bool {
	if fi == nil || fi.Mode()&fs.ModeSymlink != 0 || !fi.IsDir() && fi.Mode()&fs.ModeIrregular == 0 {
		return false
	}
	p, err := windows.UTF16PtrFromString(filepath.Clean(path))
	if err != nil {
		return false
	}
	var data windows.Win32finddata
	h, err := windows.FindFirstFile(p, &data)
	if err != nil {
		return false
	}
	windows.FindClose(h)
	return data.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 && data.Reserved0 == windows.IO_REPARSE_TAG_MOUNT_POINT
}

func isCrossDeviceErr(err error) bool { return errors.Is(err, windows.ERROR_NOT_SAME_DEVICE) }

func isPrivilegeErr(err error) bool { return errors.Is(err, windows.ERROR_PRIVILEGE_NOT_HELD) }

func isLoopErr(err error) bool { return errors.Is(err, windows.ERROR_CANT_RESOLVE_FILENAME) }
