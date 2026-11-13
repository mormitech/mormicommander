package dirsize

import (
	"syscall"
	"testing"
)

func TestCanonicalDevFirmlinks(t *testing.T) {
	var root, users syscall.Stat_t
	if syscall.Stat("/", &root) != nil || syscall.Stat("/Users", &users) != nil {
		t.Skip("cannot stat / or /Users")
	}
	if canonicalDev(uint64(users.Dev)) != canonicalDev(uint64(root.Dev)) {
		t.Errorf("/Users (dev %d) and / (dev %d) are not treated as one volume", users.Dev, root.Dev)
	}
	v, err := VolumeOf("/Users")
	if err != nil || v.Root != "/" || v.Label == "" {
		t.Errorf("VolumeOf(/Users) = %+v, %v", v, err)
	}
}
