package fileops

// Tests of the path helpers: long paths, "inside", "same path", the nearest existing parent and same-volume detection.

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPathHelpers(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "a")
	if !inside(filepath.Join(dir, "x", "y"), dir) || inside(dir+"x", dir) || !inside(dir, dir) {
		t.Error("inside wrong")
	}
	if !samePath(filepath.Join(dir, "a"), filepath.Join(dir, ".", "a")) || samePath(filepath.Join(dir, "a"), filepath.Join(dir, "b")) {
		t.Error("samePath wrong")
	}
	if got := existingAncestor(filepath.Join(dir, "no", "such")); got != dir {
		t.Errorf("existingAncestor %q", got)
	}
	if !SameVolume(filepath.Join(dir, "a"), filepath.Join(dir, "not-yet")) {
		t.Error("a folder and its own file are on different volumes?")
	}
	long := filepath.Join(dir, strings.Repeat("x", 300))
	if runtime.GOOS == "windows" {
		if !strings.HasPrefix(LongPath(long), `\\?\`) {
			t.Error("no long path prefix")
		}
	} else if LongPath(long) != long {
		t.Error("LongPath changed a path")
	}
}

func TestCopyDeepTree(t *testing.T) {
	// a tree deeper than Windows' old 260-character limit
	src, dst := t.TempDir(), t.TempDir()
	deep := filepath.Join(src, "d")
	for i := 0; i < 12; i++ {
		deep = filepath.Join(deep, strings.Repeat("n", 30))
	}
	if err := os.MkdirAll(LongPath(deep), 0o755); err != nil {
		t.Skip("file system refuses deep paths:", err)
	}
	if err := os.WriteFile(LongPath(filepath.Join(deep, "f")), []byte("deep"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := start(t, NewQueue(nil), Request{Kind: Copy, Sources: []string{filepath.Join(src, "d")}, Target: dst})
	if p.State != StateDone || p.FilesDone != 1 {
		t.Fatalf("%+v", p)
	}
	p = start(t, NewQueue(nil), Request{Kind: Delete, Sources: []string{filepath.Join(dst, "d")}})
	if p.State != StateDone || exists(filepath.Join(dst, "d")) {
		t.Fatalf("%+v", p)
	}
}
