package fileops

// Tests of RenameItem: a plain rename, refusing to replace another file, a case-only rename (which must work on
// case-insensitive and case-sensitive file systems alike) and refusing names that would leave the folder.

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRenameItem(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	write(t, a, "a")
	write(t, filepath.Join(dir, "b.txt"), "b")

	if _, err := RenameItem(a, "b.txt"); !errors.Is(err, ErrExists) {
		t.Errorf("replace: %v", err)
	}
	p, err := RenameItem(a, "A.txt")
	if err != nil || filepath.Base(p) != "A.txt" {
		t.Fatalf("case-only: %q %v", p, err)
	}
	names := map[string]bool{}
	es, _ := os.ReadDir(dir)
	for _, e := range es {
		names[e.Name()] = true
	}
	if !names["A.txt"] || names["a.txt"] || len(es) != 2 {
		t.Errorf("entries %v", names)
	}
	p, err = RenameItem(p, "c.txt")
	if err != nil || read(t, p) != "a" {
		t.Fatalf("plain: %v", err)
	}
	for _, bad := range []string{"", ".", "..", "x/y", `x\y`} {
		if _, err := RenameItem(p, bad); err == nil {
			t.Errorf("accepted name %q", bad)
		}
	}
	if _, err := RenameItem(filepath.Join(dir, "missing"), "z"); err == nil {
		t.Error("renamed a missing file")
	}
}
