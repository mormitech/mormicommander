package fileops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A free name for a long name stays within 255 bytes and keeps its extension (#149).
func TestUniqueNameLongName(t *testing.T) {
	dir := t.TempDir()
	name := strings.Repeat("n", 251) + ".txt" // 255 bytes
	os.WriteFile(filepath.Join(dir, name), nil, 0o644)
	got := UniqueName(dir, name)
	if len(got) > 255 || !strings.HasSuffix(got, "(2).txt") {
		t.Fatalf("UniqueName: %d bytes, %q", len(got), got[len(got)-10:])
	}
	if err := os.WriteFile(filepath.Join(dir, got), nil, 0o644); err != nil {
		t.Fatalf("the free name can't be used: %v", err)
	}
}
