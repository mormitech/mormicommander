//go:build unix && !darwin && !linux

// Extras for the other Unix systems (FreeBSD, OpenBSD, NetBSD, Solaris and
// friends). They are not release targets for mormiCommander, so this file only
// makes the package build there: the classic permissions, owners and times
// work through the shared Unix code, while birth time, the hidden flag and
// setting the creation date are reported as unsupported.
package attrs

import (
	"time"

	"golang.org/x/sys/unix"
)

const (
	canSetCreated  = false
	canReadCreated = false
	canHideFlag    = false
)

func fillExtras(*Info, *unix.Stat_t, bool) {}

func setCreated(string, time.Time, bool) error { return ErrUnsupported }

func setHiddenFlag(string, bool, bool) error { return ErrUnsupported }

func extraAccounts(bool) []Account { return nil }
