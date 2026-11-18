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

// The flag is read from the stat data only (a fake stat here: a real dataless file would need iCloud).
func TestDatalessFlag(t *testing.T) {
	if !Placeholder(fakeInfo{&syscall.Stat_t{Flags: SFDataless}}) {
		t.Error("SF_DATALESS not seen")
	}
	if Placeholder(fakeInfo{&syscall.Stat_t{Flags: 0x8000 | 0x20}}) || Placeholder(fakeInfo{nil}) {
		t.Error("hidden/compressed flags or missing stat data counted as a placeholder")
	}
}
