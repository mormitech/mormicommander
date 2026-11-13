// On Windows a folder listing (FindNextFile) gives the logical size of each file but not its size on disk, its hard
// link count or its file index, so learning those costs one open of the file per entry. The open asks for attribute
// access only and shares everything, so it never blocks other programs and works on files that are open elsewhere.
// FILE_STANDARD_INFO gives the allocated size and link count; for compressed and sparse files the size on disk comes
// from GetCompressedFileSize instead, which is what Explorer's "Size on disk" shows. A file index is read only for
// files with more than one link. If a file cannot be opened, or Options.Quick is set, the size on disk is estimated
// by rounding the logical size up to the cluster size. Folders are opened only when "stay on this volume" or "follow
// links" needs their volume serial number or file index.
package dirsize

import (
	"io/fs"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// statEnv holds the cluster size of the volume being counted and the Quick flag.
type statEnv struct {
	cluster int64
	quick   bool
}

func newStatEnv(root string, quick bool) *statEnv {
	env := &statEnv{cluster: 4096, quick: quick}
	if c, err := clusterSize(root); err == nil && c > 0 {
		env.cluster = int64(c)
	}
	return env
}

type fileStandardInfo struct {
	AllocationSize int64
	EndOfFile      int64
	NumberOfLinks  uint32
	DeletePending  byte
	Directory      byte
}

const (
	fileAttributeSparse     = 0x200
	fileAttributeCompressed = 0x800
	fileReadAttributes      = 0x80
)

func roundUp(n, unit int64) int64 {
	if unit <= 0 || n <= 0 {
		return n
	}
	return (n + unit - 1) / unit * unit
}

// openMeta opens a file or folder for reading its attributes only, without following reparse points.
func openMeta(path string) (windows.Handle, error) {
	p, err := windows.UTF16PtrFromString(longPath(path))
	if err != nil {
		return windows.InvalidHandle, err
	}
	return windows.CreateFile(p, fileReadAttributes,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
}

// fileStat reads size on disk and link count; the file identity is read for multi-link files when wantID is set, and
// for every file when allIDs is set (links are being followed, so one file may be reached twice).
func fileStat(path string, fi fs.FileInfo, env *statEnv, wantID, allIDs bool) entryStat {
	size := fi.Size()
	st := entryStat{size: size, alloc: roundUp(size, env.cluster)}
	if env.quick {
		return st
	}
	var attrs uint32
	if d, ok := fi.Sys().(*syscall.Win32FileAttributeData); ok {
		attrs = d.FileAttributes
	}
	h, err := openMeta(path)
	if err != nil {
		return st
	}
	defer windows.CloseHandle(h)
	var info fileStandardInfo
	if windows.GetFileInformationByHandleEx(h, windows.FileStandardInfo, (*byte)(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))) == nil {
		st.alloc = info.AllocationSize
		st.links = uint64(info.NumberOfLinks)
	}
	if attrs&(fileAttributeSparse|fileAttributeCompressed) != 0 {
		if n, err := compressedSize(path); err == nil {
			st.alloc = roundUp(n, env.cluster)
		}
	}
	if (st.links > 1 && wantID) || allIDs {
		var bh windows.ByHandleFileInformation
		if windows.GetFileInformationByHandle(h, &bh) == nil {
			st.id = fileID{dev: uint64(bh.VolumeSerialNumber), ino: uint64(bh.FileIndexHigh)<<32 | uint64(bh.FileIndexLow)}
			st.hasID = true
		}
	}
	return st
}

// dirDev returns the volume serial number of a folder. The folder is opened following reparse points, so a volume
// mounted into a folder reports its own serial number and is recognised as another volume.
func dirDev(path string, fi fs.FileInfo, env *statEnv) (uint64, bool) {
	bh, ok := byHandleFollow(path)
	if !ok {
		return 0, false
	}
	return uint64(bh.VolumeSerialNumber), true
}

func dirID(path string, fi fs.FileInfo, env *statEnv) (fileID, bool) {
	bh, ok := byHandleFollow(path)
	if !ok {
		return fileID{}, false
	}
	return fileID{dev: uint64(bh.VolumeSerialNumber), ino: uint64(bh.FileIndexHigh)<<32 | uint64(bh.FileIndexLow)}, true
}

// byHandleFollow opens a folder following reparse points, so a junction reports the folder (and volume) it points to.
func byHandleFollow(path string) (windows.ByHandleFileInformation, bool) {
	var bh windows.ByHandleFileInformation
	p, err := windows.UTF16PtrFromString(longPath(path))
	if err != nil {
		return bh, false
	}
	h, err := windows.CreateFile(p, fileReadAttributes,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return bh, false
	}
	defer windows.CloseHandle(h)
	return bh, windows.GetFileInformationByHandle(h, &bh) == nil
}
