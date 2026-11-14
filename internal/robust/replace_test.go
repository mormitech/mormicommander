// Tests for Replace: a plain rename when there is nothing to keep, an in-place update for hard-linked files (with
// the backup restored when the copy fails), and the fallback to a rename when the original is read-only.
package robust

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeFile(t *testing.T, p, s string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func onlyFiles(t *testing.T, dir string, want ...string) {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range ents {
		got = append(got, e.Name())
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("folder holds %v, want %v", got, want)
	}
}

func TestReplaceMissingTarget(t *testing.T) {
	dir := t.TempDir()
	tmp, target := filepath.Join(dir, "tmp"), filepath.Join(dir, "a.zip")
	writeFile(t, tmp, "new")
	if err := Replace(tmp, target); err != nil {
		t.Fatal(err)
	}
	if readFile(t, target) != "new" {
		t.Fatal("content not moved")
	}
	onlyFiles(t, dir, "a.zip")
}

func TestReplaceKeepsHardLinks(t *testing.T) {
	dir := t.TempDir()
	tmp, target, other := filepath.Join(dir, "tmp"), filepath.Join(dir, "a.zip"), filepath.Join(dir, "b.zip")
	writeFile(t, target, "old content that is longer")
	if err := os.Link(target, other); err != nil {
		t.Skip("no hard links here:", err)
	}
	writeFile(t, tmp, "new")
	if err := Replace(tmp, target); err != nil {
		t.Fatal(err)
	}
	if readFile(t, target) != "new" || readFile(t, other) != "new" {
		t.Fatalf("got %q and %q", readFile(t, target), readFile(t, other))
	}
	a, _ := os.Stat(target)
	b, _ := os.Stat(other)
	if !os.SameFile(a, b) {
		t.Fatal("hard link broken")
	}
	onlyFiles(t, dir, "a.zip", "b.zip")
}

func TestReplaceRestoresOnFailedCopy(t *testing.T) {
	dir := t.TempDir()
	tmp, target, other := filepath.Join(dir, "tmp"), filepath.Join(dir, "a.zip"), filepath.Join(dir, "b.zip")
	writeFile(t, target, "original")
	if err := os.Link(target, other); err != nil {
		t.Skip("no hard links here:", err)
	}
	writeFile(t, tmp, "new content")
	boom := errors.New("disk full")
	calls := 0
	copyData = func(w io.Writer, r io.Reader) (int64, error) {
		calls++
		if calls == 1 { // the new content: write a little, then fail
			w.Write([]byte("ne"))
			return 2, boom
		}
		return io.Copy(w, r)
	}
	defer func() { copyData = io.Copy }()
	if err := Replace(tmp, target); !errors.Is(err, boom) {
		t.Fatalf("want the copy error, got %v", err)
	}
	if readFile(t, target) != "original" || readFile(t, other) != "original" {
		t.Fatalf("original not restored: %q", readFile(t, target))
	}
	onlyFiles(t, dir, "a.zip", "b.zip", "tmp") // the caller removes its own temporary file
}

func TestReplaceReadOnlyLinkedFallsBackToRename(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs a Unix user that read-only modes apply to")
	}
	dir := t.TempDir()
	tmp, target, other := filepath.Join(dir, "tmp"), filepath.Join(dir, "a.zip"), filepath.Join(dir, "b.zip")
	writeFile(t, target, "old")
	if err := os.Link(target, other); err != nil {
		t.Skip("no hard links here:", err)
	}
	os.Chmod(target, 0o444)
	writeFile(t, tmp, "new")
	if err := Replace(tmp, target); err != nil {
		t.Fatal(err)
	}
	if readFile(t, target) != "new" || readFile(t, other) != "old" {
		t.Fatalf("got %q and %q", readFile(t, target), readFile(t, other))
	}
	onlyFiles(t, dir, "a.zip", "b.zip")
}
