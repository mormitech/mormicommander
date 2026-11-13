// Since macOS Catalina the startup disk is two APFS volumes: a read-only system volume mounted at "/" and a data
// volume mounted at /System/Volumes/Data, whose folders (/Users, /Applications, /private and so on) are firmlinked
// into "/". The user sees one disk, "Macintosh HD", so "stay on this volume" must treat both device numbers as the
// same volume; otherwise counting "/" with SameVolume would skip every user file. canonicalDev maps the data volume's
// device number to the system volume's.
package dirsize

import (
	"sync"
	"syscall"
)

var (
	aliasOnce sync.Once
	dataDev   uint64
	rootDev   uint64
	aliasOK   bool
)

func canonicalDev(dev uint64) uint64 {
	aliasOnce.Do(func() {
		var a, b syscall.Stat_t
		if syscall.Stat("/", &a) == nil && syscall.Stat("/System/Volumes/Data", &b) == nil {
			rootDev, dataDev, aliasOK = uint64(a.Dev), uint64(b.Dev), true
		}
	})
	if aliasOK && dev == dataDev {
		return rootDev
	}
	return dev
}
