package tarfmt

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// An archive whose name is near the 255-byte limit can be created: its temporary file uses only the start of the
// name (#149).
func TestCreateLongArchiveName(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.txt")
	os.WriteFile(src, []byte("a"), 0o644)
	arc := filepath.Join(dir, strings.Repeat("n", 245)+".tar.gz")
	if err := Create(context.Background(), arc, []string{src}, CreateOptions{Format: DetectFormat(arc)}); err != nil {
		t.Fatalf("create: %v", err)
	}
}
