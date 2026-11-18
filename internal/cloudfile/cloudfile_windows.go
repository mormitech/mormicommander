package cloudfile

import (
	"io/fs"
	"syscall"
)

// Windows attribute bits the Cloud Files API (OneDrive Files On-Demand and other sync engines) sets on placeholders.
const (
	AttrOffline            = 0x1000   // FILE_ATTRIBUTE_OFFLINE
	AttrRecallOnOpen       = 0x40000  // FILE_ATTRIBUTE_RECALL_ON_OPEN
	AttrRecallOnDataAccess = 0x400000 // FILE_ATTRIBUTE_RECALL_ON_DATA_ACCESS
	placeholderMask        = AttrOffline | AttrRecallOnOpen | AttrRecallOnDataAccess
)

func placeholder(info fs.FileInfo) bool {
	d, ok := info.Sys().(*syscall.Win32FileAttributeData)
	return ok && d.FileAttributes&placeholderMask != 0
}
