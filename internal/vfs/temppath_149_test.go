package vfs

import (
	"path/filepath"
	"strings"
	"testing"
)

// The staging path for a downloaded server file is always one plain name inside its own private folder, so the
// folder a caller removes (filepath.Dir of it) is that private folder (#149).
func TestNewTempPathPlainName(t *testing.T) {
	a := NewArchiveFS()
	defer a.Close()
	for _, name := range []string{"ok.zip", "a/b.zip", "../up.zip", "..", "x/../../y.zip"} {
		p, err := a.newTempPath(name)
		if err != nil {
			t.Fatal(err)
		}
		dir := filepath.Dir(p)
		root, _ := a.tempDir()
		if filepath.Dir(dir) != root || strings.Contains(filepath.Base(p), "..") && filepath.Base(p) != "file" {
			t.Errorf("%q: %s is not one name in a private folder under %s", name, p, root)
		}
	}
}
