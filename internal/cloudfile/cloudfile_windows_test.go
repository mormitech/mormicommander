package cloudfile

import (
	"io/fs"
	"syscall"
	"testing"
	"time"
)

type fakeInfo struct{ sys any }

func (fakeInfo) Name() string       { return "f" }
func (fakeInfo) Size() int64        { return 0 }
func (fakeInfo) Mode() fs.FileMode  { return 0o644 }
func (fakeInfo) ModTime() time.Time { return time.Time{} }
func (fakeInfo) IsDir() bool        { return false }
func (f fakeInfo) Sys() any         { return f.sys }

func TestRecallAttributes(t *testing.T) {
	for _, a := range []uint32{AttrOffline, AttrRecallOnOpen, AttrRecallOnDataAccess} {
		if !Placeholder(fakeInfo{&syscall.Win32FileAttributeData{FileAttributes: a | syscall.FILE_ATTRIBUTE_ARCHIVE}}) {
			t.Errorf("attribute %#x not seen", a)
		}
	}
	if Placeholder(fakeInfo{&syscall.Win32FileAttributeData{FileAttributes: syscall.FILE_ATTRIBUTE_ARCHIVE | 0x400}}) {
		t.Error("archive/reparse counted as a placeholder")
	}
}
