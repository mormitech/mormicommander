package main

import (
	"path/filepath"
	"testing"

	"mormicommander/internal/dirsize"
)

// A cloud folder is marked (its own style, left out of the panel total) only where it is listed in a folder of the
// disk; inside cloud storage nothing is marked, so the folder the user counts in decides what adds up.
func TestCloudMarkOnlyOutsideCloudStorage(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cs := filepath.Join(home, "Library", "CloudStorage")
	for _, c := range []struct {
		path string
		mark bool
	}{
		{cs, true}, // seen from ~/Library
		{filepath.Join(home, "Library", "Mobile Documents"), true},
		{filepath.Join(cs, "OneDrive-Firm"), false}, // seen from inside CloudStorage
		{filepath.Join(cs, "OneDrive-Firm", "Projects"), false},
	} {
		r := dirsize.Result{Path: c.path, Cloud: dirsize.InCloud(c.path)}
		if got := cloudMark(r); got != c.mark {
			t.Errorf("%s: mark %v, want %v", c.path, got, c.mark)
		}
	}
	if cloudMark(dirsize.Result{Path: filepath.Join(home, "Documents")}) {
		t.Error("a folder of the disk is marked")
	}
}
