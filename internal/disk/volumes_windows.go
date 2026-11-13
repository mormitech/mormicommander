// On Windows the drive bar shows every drive letter, as Total Commander does:
// fixed disks, removable drives, network drives and CD/DVD drives, even when
// no disk is inserted. Two Windows behaviours make a naive loop over the
// letters dangerous for a UI. Asking an empty floppy, CD or card reader for
// its label can pop up a system "There is no disk in the drive" dialog unless
// critical-error dialogs are switched off with SetErrorMode, and asking a
// disconnected network drive can block for many seconds while Windows tries
// to reconnect. So the error mode is set before any query (the Go runtime
// normally does this too, but the drive list must not depend on it), every
// drive is queried in its own goroutine, and drives that have not answered
// within queryTimeout are listed without label and size instead of holding
// up the whole list. A late answer is simply dropped.

package disk

import (
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// queryTimeout bounds how long Volumes waits for slow drives (disconnected
// network drives, spinning-up optical drives) before listing them without
// label and size.
const queryTimeout = 1500 * time.Millisecond

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

// noErrorDialogs turns off the system's critical-error and open-file error
// boxes for this process, so querying an empty drive fails quietly.
func noErrorDialogs() {
	const want = windows.SEM_FAILCRITICALERRORS | windows.SEM_NOOPENFILEERRORBOX
	old := windows.SetErrorMode(want)
	windows.SetErrorMode(old | want)
}

// Volumes lists every drive letter: fixed, removable, network and CD/DVD drives. A drive with no disk in it (an empty
// card reader) or one that does not answer in time is still listed, with no size, as Total Commander does.
func Volumes() []Volume {
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return nil
	}
	noErrorDialogs()

	type answer struct {
		index        int
		label        string
		total, avail uint64
		ok           bool
	}
	var out []Volume
	results := make(chan answer, 26)
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
		index := len(out)
		out = append(out, Volume{Name: root[:2], Path: root, Kind: kind})
		go func(index int, root string, p *uint16) {
			a := answer{index: index}
			label := make([]uint16, windows.MAX_PATH+1)
			if windows.GetVolumeInformation(p, &label[0], uint32(len(label)), nil, nil, nil, nil, 0) == nil {
				a.label = windows.UTF16ToString(label)
			}
			if u, err := UsageOf(root); err == nil {
				a.total, a.avail, a.ok = u.Total, u.Free, true
			}
			results <- a
		}(index, root, p)
	}

	deadline := time.NewTimer(queryTimeout)
	defer deadline.Stop()
	for pending := len(out); pending > 0; pending-- {
		select {
		case a := <-results:
			v := &out[a.index]
			v.Label = a.label
			if a.ok {
				v.Total, v.Free = a.total, a.avail
			}
		case <-deadline.C:
			return withHome(out)
		}
	}
	return withHome(out)
}

// withHome adds the user's folder (C:\Users\<name>) after the drive letters: the letters keep Total Commander's
// order and keys, and Home is one click away as on macOS and Linux.
func withHome(out []Volume) []Volume {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return out
	}
	v := Volume{Name: "Home", Path: home, Kind: "home"}
	if u, err := UsageOf(home); err == nil {
		v.Total, v.Free = u.Total, u.Free
	}
	return append(out, v)
}
