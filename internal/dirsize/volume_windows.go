// On Windows the volume that holds a path is found with GetVolumePathName, which also understands volumes mounted
// into folders (C:\mnt\backup\) and UNC shares (\\server\share\). GetVolumeInformation then gives the label, serial
// number, file system name and read-only flag; GetDiskFreeSpaceEx the sizes as this user sees them (quotas included);
// GetDiskFreeSpace the cluster size, which "size on disk" rounds to; and GetDriveType whether the drive is removable,
// a CD/DVD or a network drive. This file also holds the small Win32 helpers that stat_windows.go needs and that
// golang.org/x/sys/windows does not wrap.
package dirsize

import (
	"os"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32                  = windows.NewLazySystemDLL("kernel32.dll")
	procGetDiskFreeSpaceW     = kernel32.NewProc("GetDiskFreeSpaceW")
	procGetCompressedFileSize = kernel32.NewProc("GetCompressedFileSizeW")
)

// longPath adds the \\?\ prefix to long absolute paths so Win32 calls accept paths over MAX_PATH, as the os package
// does internally.
func longPath(p string) string {
	if len(p) < 248 || strings.HasPrefix(p, `\\?\`) {
		return p
	}
	if strings.HasPrefix(p, `\\`) {
		return `\\?\UNC\` + p[2:]
	}
	if len(p) >= 3 && p[1] == ':' && (p[2] == '\\' || p[2] == '/') {
		return `\\?\` + strings.ReplaceAll(p, "/", `\`)
	}
	return p
}

// volumeRoot returns the mount point that holds path, with a trailing backslash.
func volumeRoot(path string) (string, error) {
	p, err := windows.UTF16PtrFromString(longPath(path))
	if err != nil {
		return "", err
	}
	buf := make([]uint16, windows.MAX_LONG_PATH)
	if err := windows.GetVolumePathName(p, &buf[0], uint32(len(buf))); err != nil {
		return "", err
	}
	return windows.UTF16ToString(buf), nil
}

// clusterSize is the allocation unit of the volume that holds path.
func clusterSize(path string) (uint64, error) {
	root, err := volumeRoot(path)
	if err != nil {
		return 0, err
	}
	p, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return 0, err
	}
	var spc, bps, free, total uint32
	r, _, e := procGetDiskFreeSpaceW.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&spc)),
		uintptr(unsafe.Pointer(&bps)), uintptr(unsafe.Pointer(&free)), uintptr(unsafe.Pointer(&total)))
	if r == 0 {
		return 0, e
	}
	return uint64(spc) * uint64(bps), nil
}

// compressedSize is the size on disk of a compressed or sparse file.
func compressedSize(path string) (int64, error) {
	p, err := windows.UTF16PtrFromString(longPath(path))
	if err != nil {
		return 0, err
	}
	var high uint32
	low, _, e := procGetCompressedFileSize.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&high)))
	if uint32(low) == 0xFFFFFFFF && e != windows.ERROR_SUCCESS {
		return 0, e
	}
	return int64(high)<<32 | int64(uint32(low)), nil
}

// VolumeOf describes the volume that holds path.
func VolumeOf(path string) (VolumeInfo, error) {
	v := VolumeInfo{Path: path}
	// GetVolumePathName works on the path's text and succeeds for paths that do not exist; fail for those as statfs
	// does on macOS and Linux, so callers do not show a drive for a folder that is gone.
	if _, err := os.Stat(path); err != nil {
		return v, err
	}
	root, err := volumeRoot(path)
	if err != nil {
		return v, &os.PathError{Op: "GetVolumePathName", Path: path, Err: err}
	}
	v.Root = strings.TrimPrefix(root, `\\?\`)
	rp, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return v, err
	}
	var avail, total, free uint64
	if err := windows.GetDiskFreeSpaceEx(rp, &avail, &total, &free); err != nil {
		return v, &os.PathError{Op: "GetDiskFreeSpaceEx", Path: path, Err: err}
	}
	v.Total, v.Free, v.Available = total, free, avail

	label := make([]uint16, windows.MAX_PATH+1)
	fsName := make([]uint16, windows.MAX_PATH+1)
	var serial, maxComp, flags uint32
	if windows.GetVolumeInformation(rp, &label[0], uint32(len(label)), &serial, &maxComp, &flags, &fsName[0], uint32(len(fsName))) == nil {
		v.Label = windows.UTF16ToString(label)
		v.FileSystem = windows.UTF16ToString(fsName)
		v.Serial = serial
		v.ReadOnly = flags&windows.FILE_READ_ONLY_VOLUME != 0
	}
	if c, err := clusterSize(root); err == nil {
		v.BlockSize = c
	}
	switch windows.GetDriveType(rp) {
	case windows.DRIVE_REMOTE:
		v.Remote = true
	case windows.DRIVE_REMOVABLE, windows.DRIVE_CDROM:
		v.Removable = true
	}
	name := make([]uint16, 64)
	if windows.GetVolumeNameForVolumeMountPoint(rp, &name[0], uint32(len(name))) == nil {
		v.Device = windows.UTF16ToString(name)
	} else if strings.HasPrefix(v.Root, `\\`) {
		v.Device = strings.TrimSuffix(v.Root, `\`)
	}
	return v, nil
}
