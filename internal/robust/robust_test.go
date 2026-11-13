package robust

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRenameReplaces(t *testing.T) {
	dir := t.TempDir()
	from, to := filepath.Join(dir, "new"), filepath.Join(dir, "old")
	if err := os.WriteFile(from, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(to, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Rename(from, to); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(to)
	if err != nil || string(b) != "new" {
		t.Fatalf("got %q, %v", b, err)
	}
	if _, err := os.Stat(from); !os.IsNotExist(err) {
		t.Fatalf("source still there: %v", err)
	}
}

func TestRenameMissingSourceFailsAtOnce(t *testing.T) {
	dir := t.TempDir()
	if err := Rename(filepath.Join(dir, "nope"), filepath.Join(dir, "x")); err == nil {
		t.Fatal("expected an error")
	}
}
