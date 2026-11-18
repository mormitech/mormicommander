package main

import (
	"os"
	"path/filepath"
	"testing"

	"mormicommander/internal/config"
)

// #24: the treemap remembers counted sizes only with the option on, and turning it off removes the file.
func TestTreemapRemember(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MORMICOMMANDER_CONFIG_DIR", dir)
	on := false
	old := treemapRemember
	treemapRemember = func() bool { return on }
	t.Cleanup(func() { treemapRemember = old })
	file := filepath.Join(dir, "foldersizes.bin")
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")

	tm := &Treemap{}
	if n, _ := tm.Remember(map[string]int64{a: 10}); n != 0 {
		t.Fatal("remembered with the option off")
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("a file was written with the option off")
	}

	on = true
	if n, err := tm.Remember(map[string]int64{a: 10, b: 20, "sftp://host/x": 5, "relative": 1}); n != 2 || err != nil {
		t.Fatalf("kept %d, %v (only local absolute paths count)", n, err)
	}
	// a new session (new object) reads the file
	tm2 := &Treemap{}
	got := tm2.Remembered([]string{a, b, filepath.Join(dir, "c")})
	if len(got) != 2 || got[a].Bytes != 10 || got[b].Bytes != 20 || got[a].When == 0 {
		t.Fatalf("%+v", got)
	}

	on = false
	if got := tm2.Remembered([]string{a}); len(got) != 0 {
		t.Errorf("sizes given with the option off: %+v", got)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Error("turning the option off must remove the file")
	}
}

func TestTreemapOptionOffByDefault(t *testing.T) {
	s, ok := config.LookupSetting("Operation.TreemapRemember")
	if !ok || s.Default != "0" {
		t.Errorf("missing or not off by default: %+v", s)
	}
}
