package fileops

// Windows keeps three things the portable copy misses: the attributes (read-only, hidden, system, archive, not
// indexed), the creation date, and alternate data streams (extra named contents of a file, such as the
// Zone.Identifier stream that marks a download from the internet). This file copies all three. Owners and ACLs are
// not copied: a copy gets the rights of the folder it lands in, like a copy in Explorer.

import (
	"io"
	"io/fs"
	"os"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const copiedAttrs = windows.FILE_ATTRIBUTE_READONLY | windows.FILE_ATTRIBUTE_HIDDEN | windows.FILE_ATTRIBUTE_SYSTEM |
	windows.FILE_ATTRIBUTE_ARCHIVE | windows.FILE_ATTRIBUTE_NOT_CONTENT_INDEXED

func copyOwner(dst string, info fs.FileInfo) error { return nil }

func atimeOf(info fs.FileInfo) time.Time {
	if d, ok := info.Sys().(*syscall.Win32FileAttributeData); ok {
		return time.Unix(0, d.LastAccessTime.Nanoseconds())
	}
	return info.ModTime()
}

// setCreationTime gives dst the creation date of the source.
func setCreationTime(dst string, info fs.FileInfo) error {
	d, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return nil
	}
	name, err := windows.UTF16PtrFromString(lp(dst))
	if err != nil {
		return err
	}
	h, err := windows.CreateFile(name, windows.FILE_WRITE_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	ct := windows.Filetime{LowDateTime: d.CreationTime.LowDateTime, HighDateTime: d.CreationTime.HighDateTime}
	return windows.SetFileTime(h, &ct, nil, nil)
}

// copyPlatformAttrs copies the read-only, hidden, system, archive and not-indexed attributes.
func copyPlatformAttrs(src, dst string) error {
	sp, err := windows.UTF16PtrFromString(lp(src))
	if err != nil {
		return err
	}
	dp, err := windows.UTF16PtrFromString(lp(dst))
	if err != nil {
		return err
	}
	sa, err := windows.GetFileAttributes(sp)
	if err != nil {
		return err
	}
	da, err := windows.GetFileAttributes(dp)
	if err != nil {
		return err
	}
	want := da&^copiedAttrs | sa&copiedAttrs
	if want == da {
		return nil
	}
	return windows.SetFileAttributes(dp, want)
}

var (
	kernel32         = windows.NewLazySystemDLL("kernel32.dll")
	procFindFirstStr = kernel32.NewProc("FindFirstStreamW")
	procFindNextStr  = kernel32.NewProc("FindNextStreamW")
)

// win32FindStreamData is WIN32_FIND_STREAM_DATA.
type win32FindStreamData struct {
	StreamSize int64
	StreamName [windows.MAX_PATH + 36]uint16
}

// copyXattrs copies the alternate data streams of src (every stream but the unnamed main one, "::$DATA").
func copyXattrs(src, dst string) error {
	name, err := windows.UTF16PtrFromString(lp(src))
	if err != nil {
		return err
	}
	if procFindFirstStr.Find() != nil {
		return nil // an old Windows without the call
	}
	var d win32FindStreamData
	r, _, e := procFindFirstStr.Call(uintptr(unsafe.Pointer(name)), 0, uintptr(unsafe.Pointer(&d)), 0)
	h := windows.Handle(r)
	if h == windows.InvalidHandle {
		if e == windows.ERROR_HANDLE_EOF || e == windows.ERROR_INVALID_PARAMETER {
			return nil // no streams, or a file system without them (FAT)
		}
		return e
	}
	defer windows.FindClose(h)
	var first error
	for {
		s := windows.UTF16ToString(d.StreamName[:])
		if s != "::$DATA" && s != "" {
			if err := copyStream(src+s, dst+s); err != nil && first == nil {
				first = err
			}
		}
		ok, _, e := procFindNextStr.Call(uintptr(h), uintptr(unsafe.Pointer(&d)))
		if ok == 0 {
			if e != windows.ERROR_HANDLE_EOF && first == nil {
				first = e
			}
			return first
		}
	}
}

func copyStream(src, dst string) error {
	in, err := os.Open(lp(src))
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(lp(dst), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// linkCount is the number of names (hard links) the file has on NTFS; wipe leaves the data alone when another name
// shares it. A file that can't be opened counts as having one name.
func linkCount(p string, info fs.FileInfo) uint64 {
	f, err := os.Open(lp(p))
	if err != nil {
		return 1
	}
	defer f.Close()
	var d windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(f.Fd()), &d); err != nil {
		return 1
	}
	return uint64(d.NumberOfLinks)
}
