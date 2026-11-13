// The Windows side of reading and changing attributes. Windows keeps the
// attribute bits TC users know (read-only, hidden, system, archive, plus the
// NTFS compressed and encrypted states), three time stamps including a real,
// settable creation time, and an owner that is a security identifier rather
// than a number. The attribute bits come for free with every directory read,
// so they are taken from the FileInfo; the owner needs one security query and
// the size on disk needs the compressed size rounded up to the cluster size of
// the volume. Unix permissions and changing the owner do not exist here (the
// native Security tab covers them), so those report themselves unsupported.
package attrs

import (
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modkernel32               = windows.NewLazySystemDLL("kernel32.dll")
	procGetCompressedFileSize = modkernel32.NewProc("GetCompressedFileSizeW")
	procGetDiskFreeSpace      = modkernel32.NewProc("GetDiskFreeSpaceW")

	clusterCache sync.Map // volume root -> int64 cluster size
)

const (
	canSetCreated  = true
	canReadCreated = true
	canHideFlag    = false
)

func fillPlatform(info *Info, fi fs.FileInfo, _ bool) {
	if d, ok := fi.Sys().(*syscall.Win32FileAttributeData); ok {
		info.Win = WinAttr(d.FileAttributes)
		info.WinNative = true
		info.Created = time.Unix(0, d.CreationTime.Nanoseconds())
		info.HasCreated = true
		info.Accessed = time.Unix(0, d.LastAccessTime.Nanoseconds())
		info.HasAccessed = true
		if info.Win&AttrHidden != 0 {
			info.Hidden = true
		}
	}
	if info.Kind == KindDir || info.Kind == KindSymlink {
		info.SizeOnDisk = 0
	} else {
		info.SizeOnDisk = sizeOnDisk(info.Path)
	}
	info.Owner, info.Group = ownerNames(info.Path)
}

// sizeOnDisk returns the allocated size: the compressed (or sparse) size
// rounded up to whole clusters, which is what Explorer shows.
func sizeOnDisk(path string) int64 {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return -1
	}
	var high uint32
	low, _, callErr := procGetCompressedFileSize.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&high)))
	if uint32(low) == 0xFFFFFFFF && callErr != windows.ERROR_SUCCESS {
		return -1
	}
	size := int64(high)<<32 | int64(uint32(low))
	cluster := clusterSize(path)
	if cluster > 0 && size%cluster != 0 {
		size += cluster - size%cluster
	}
	return size
}

// clusterSize returns the allocation unit of the volume holding path.
func clusterSize(path string) int64 {
	abs, err := filepath.Abs(path)
	if err != nil {
		return 0
	}
	p, err := windows.UTF16PtrFromString(abs)
	if err != nil {
		return 0
	}
	buf := make([]uint16, windows.MAX_PATH+1)
	if err := windows.GetVolumePathName(p, &buf[0], uint32(len(buf))); err != nil {
		return 0
	}
	root := windows.UTF16ToString(buf)
	if v, ok := clusterCache.Load(root); ok {
		return v.(int64)
	}
	var spc, bps, free, total uint32
	rp, _ := windows.UTF16PtrFromString(root)
	r, _, _ := procGetDiskFreeSpace.Call(uintptr(unsafe.Pointer(rp)),
		uintptr(unsafe.Pointer(&spc)), uintptr(unsafe.Pointer(&bps)),
		uintptr(unsafe.Pointer(&free)), uintptr(unsafe.Pointer(&total)))
	if r == 0 {
		return 0
	}
	c := int64(spc) * int64(bps)
	clusterCache.Store(root, c)
	return c
}

// ownerNames returns "DOMAIN\user" style names of the owner and primary group.
func ownerNames(path string) (owner, group string) {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.GROUP_SECURITY_INFORMATION)
	if err != nil {
		return "", ""
	}
	name := func(sid *windows.SID) string {
		if sid == nil {
			return ""
		}
		acc, dom, _, err := sid.LookupAccount("")
		if err != nil {
			return sid.String()
		}
		if dom != "" {
			return dom + `\` + acc
		}
		return acc
	}
	if o, _, err := sd.Owner(); err == nil {
		owner = name(o)
	}
	if g, _, err := sd.Group(); err == nil {
		group = name(g)
	}
	return owner, group
}

// unixWinAttr is only meaningful on Unix; on Windows attributes are native.
func unixWinAttr(_ Kind, _ fs.FileMode, _ bool) WinAttr { return 0 }

func groupName(gid int) string { return strconv.Itoa(gid) }

func lookupUID(string) (int, error) { return -1, ErrUnsupported }
func lookupGID(string) (int, error) { return -1, ErrUnsupported }

func chmodPath(string, fs.FileMode) error { return ErrUnsupported }

func chownPath(string, int, int, bool) error { return ErrUnsupported }

// settableMask is what SetFileAttributes may change; the others are states
// the file system manages (folder, compressed, encrypted, sparse, reparse).
const settableMask = AttrReadOnly | AttrHidden | AttrSystem | AttrArchive | AttrTemporary | AttrOffline | AttrNotIndexed

// setWinAttrs writes the attribute bits. Bits outside settableMask are
// dropped and an empty set becomes FILE_ATTRIBUTE_NORMAL, as Windows requires.
func setWinAttrs(path string, attrs WinAttr, _ bool) error {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	v := uint32(attrs & settableMask)
	if v == 0 {
		v = uint32(AttrNormal)
	}
	if err := windows.SetFileAttributes(p, v); err != nil {
		return &os.PathError{Op: "SetFileAttributes", Path: path, Err: err}
	}
	return nil
}

func setHiddenFlag(string, bool, bool) error { return ErrUnsupported }

// setTimes opens the entry for attribute writing only (so it works on
// folders, and on links themselves when not following) and sets any of the
// three times; nil pointers keep the old value.
func setTimes(path string, mtime, atime, ctime *time.Time, follow bool) error {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	flags := uint32(windows.FILE_FLAG_BACKUP_SEMANTICS)
	if !follow {
		flags |= windows.FILE_FLAG_OPEN_REPARSE_POINT
	}
	h, err := windows.CreateFile(p, windows.FILE_WRITE_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, flags, 0)
	if err != nil {
		return &os.PathError{Op: "open", Path: path, Err: err}
	}
	defer windows.CloseHandle(h)
	ft := func(t *time.Time) *windows.Filetime {
		if t == nil {
			return nil
		}
		f := windows.NsecToFiletime(t.UnixNano())
		return &f
	}
	if err := windows.SetFileTime(h, ft(ctime), ft(atime), ft(mtime)); err != nil {
		return &os.PathError{Op: "SetFileTime", Path: path, Err: err}
	}
	return nil
}

func extraAccounts(bool) []Account { return nil }

func platformCaps() Capabilities {
	return Capabilities{
		SettableAttrs: AttrReadOnly | AttrHidden | AttrSystem | AttrArchive,
		ReportedAttrs: AttrReadOnly | AttrHidden | AttrSystem | AttrArchive | AttrCompressed | AttrEncrypted |
			AttrDirectory | AttrTemporary | AttrSparse | AttrReparsePoint | AttrOffline | AttrNotIndexed,
		CreatedTime: true,
		ReadCreated: true,
		NativeAttrs: true,
	}
}

func ownerLabel(uid int) string { return strconv.Itoa(uid) }

// currentUID: there are no Unix owners here, so no planned change can open a
// folder by its owner bits.
func currentUID() int { return -2 }
