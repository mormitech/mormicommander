// This file turns numeric user and group ids into names for the owner and group columns. A folder of 100k files
// usually has one or two owners, and looking a name up in the user database is slow, so results are cached for the
// life of the process; an id with no name is shown as the number, which is what ls does too.

//go:build darwin || linux

package listing

import (
	"os/user"
	"strconv"
	"sync"
)

var (
	idMu    sync.Mutex
	userIDs = map[uint32]string{}
	groupID = map[uint32]string{}
)

func userName(uid uint32) string {
	idMu.Lock()
	defer idMu.Unlock()
	if n, ok := userIDs[uid]; ok {
		return n
	}
	s := strconv.FormatUint(uint64(uid), 10)
	n := s
	if u, err := user.LookupId(s); err == nil {
		n = u.Username
	}
	userIDs[uid] = n
	return n
}

func groupName(gid uint32) string {
	idMu.Lock()
	defer idMu.Unlock()
	if n, ok := groupID[gid]; ok {
		return n
	}
	s := strconv.FormatUint(uint64(gid), 10)
	n := s
	if g, err := user.LookupGroupId(s); err == nil {
		n = g.Name
	}
	groupID[gid] = n
	return n
}
