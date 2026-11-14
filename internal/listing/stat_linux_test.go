// This file checks that a listed file on Linux gets its creation date from statx when the file system records one,
// and that the creation date stays empty when it does not (rather than falling back to another timestamp).

//go:build linux

package listing

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFromFileInfoLinuxBirthTime(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.txt")
	before := time.Now().Add(-time.Minute)
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Lstat(p)
	if err != nil {
		t.Fatal(err)
	}
	it := FromFileInfo(dir, fi)
	bt, ok := birthTime(p)
	if !ok {
		if !it.CreateTime.IsZero() {
			t.Fatalf("CreateTime = %v without a birth time, want zero", it.CreateTime)
		}
		t.Skip("this file system does not record a birth time")
	}
	if !it.CreateTime.Equal(bt) {
		t.Fatalf("CreateTime = %v, want statx birth time %v", it.CreateTime, bt)
	}
	if it.CreateTime.Before(before) || it.CreateTime.After(time.Now().Add(time.Minute)) {
		t.Fatalf("CreateTime = %v, not around now", it.CreateTime)
	}
}
