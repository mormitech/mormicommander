package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A file whose name is near the 255-byte limit can still be written atomically: the temporary name next to it uses
// only the start of the name (#149).
func TestWriteFileAtomicLongName(t *testing.T) {
	p := filepath.Join(t.TempDir(), strings.Repeat("n", 250)+".json")
	if err := WriteFileAtomic(p, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if b, _ := os.ReadFile(p); string(b) != "{}" {
		t.Fatalf("content %q", b)
	}
}
