package robust

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// RenameNoReplace never replaces what is at the new name (#149).
func TestRenameNoReplaceKeepsTheTarget(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	os.WriteFile(a, []byte("a"), 0o644)
	os.WriteFile(b, []byte("b"), 0o644)
	if err := RenameNoReplace(a, b); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("rename over an existing file: %v, want ErrExist", err)
	}
	if got, _ := os.ReadFile(b); string(got) != "b" {
		t.Fatalf("the target was replaced: %q", got)
	}
	if err := RenameNoReplace(a, filepath.Join(dir, "c")); err != nil {
		t.Fatalf("rename to a free name: %v", err)
	}
	if _, err := os.Lstat(a); err == nil {
		t.Fatal("the old name is still there")
	}
}

func TestListsName(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "File"), nil, 0o644)
	if !ListsName(dir, "File") || ListsName(dir, "file") {
		t.Fatal("ListsName must match the exact spelling only")
	}
}
