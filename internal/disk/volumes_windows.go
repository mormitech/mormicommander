package disk

import (
	"golang.org/x/sys/windows"
)

// UsageOf reports the volume that holds path.
func UsageOf(path string) (Usage, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return Usage{}, err
	}
	var avail, total, free uint64
	if err := windows.GetDiskFreeSpaceEx(p, &avail, &total, &free); err != nil {
		return Usage{}, err
	}
	return Usage{Total: total, Free: avail}, nil
}

// Volumes lists every drive letter: fixed, removable, network and CD/DVD drives. A drive with no disk in it (an empty
// card reader) is still listed, with no size, as Total Commander does.
func Volumes() []Volume {
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return nil
	}
	var out []Volume
	for i := 0; i < 26; i++ {
		if mask&(1<<uint(i)) == 0 {
			continue
		}
		root := string(rune('A'+i)) + `:\`
		p, _ := windows.UTF16PtrFromString(root)
		kind := "fixed"
		switch windows.GetDriveType(p) {
		case windows.DRIVE_REMOVABLE:
			kind = "removable"
		case windows.DRIVE_REMOTE:
			kind = "network"
		case windows.DRIVE_CDROM:
			kind = "cdrom"
		case windows.DRIVE_FIXED, windows.DRIVE_RAMDISK:
		default:
			continue
		}
		v := Volume{Name: root[:2], Path: root, Kind: kind}
		label := make([]uint16, windows.MAX_PATH+1)
		if windows.GetVolumeInformation(p, &label[0], uint32(len(label)), nil, nil, nil, nil, 0) == nil {
			v.Label = windows.UTF16ToString(label)
		}
		if u, err := UsageOf(root); err == nil {
			v.Total, v.Free = u.Total, u.Free
		}
		out = append(out, v)
	}
	return out
}
