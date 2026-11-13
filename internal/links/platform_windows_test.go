//go:build windows

// Windows-only tests: junctions are created with the hand-built mount-point
// reparse buffer, are recognised by Inspect as junctions (Go reports them as
// irregular files, not symlinks), resolve to their folder, and can be
// replaced with Overwrite. Hard link counts come from the file handle.

package links

import (
	"os"
	"path/filepath"
	"testing"
)

func TestJunction(t *testing.T) {
	dir := realTemp(t)
	target := filepath.Join(dir, "target")
	writeFile(t, filepath.Join(target, "f.txt"), "f")
	j := filepath.Join(dir, "junc")
	if err := CreateJunction(target, j, CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(j, "f.txt")); err != nil || string(b) != "f" {
		t.Fatalf("read through junction: %q %v", b, err)
	}
	info, err := Inspect(j)
	if err != nil {
		t.Fatal(err)
	}
	if info.Kind != KindJunction || !info.TargetIsDir || info.Broken {
		t.Errorf("Inspect = %+v", info)
	}
	if !IsLink(j) {
		t.Error("IsLink(junction) = false")
	}
	other := filepath.Join(dir, "other")
	if err := os.Mkdir(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := CreateJunction(other, j, CreateOptions{Overwrite: true}); err != nil {
		t.Fatalf("overwrite junction: %v", err)
	}
	if _, err := os.Stat(filepath.Join(j, "f.txt")); err == nil {
		t.Error("junction still points at the old folder")
	}
	f := filepath.Join(dir, "file")
	writeFile(t, f, "")
	if err := CreateJunction(f, filepath.Join(dir, "j2"), CreateOptions{}); err == nil {
		t.Error("junction to a file was created")
	}
}
