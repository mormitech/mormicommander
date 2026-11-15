package dirsize

import "golang.org/x/sys/windows"

// Space reports the size and free space of the volume that holds path.
func Space(path string) (Usage, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return Usage{}, err
	}
	var avail, total, free uint64
	if err := windows.GetDiskFreeSpaceEx(p, &avail, &total, &free); err != nil {
		return Usage{}, err
	}
	return Usage{Total: total, Available: avail}, nil
}
