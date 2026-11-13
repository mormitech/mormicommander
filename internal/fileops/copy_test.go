package fileops

// Tests of the copy and move engine beyond the basics in fileops_test.go: filters, rename masks, link modes,
// verification, moves across volumes, case-only renames, Shift+F5 auto-names, attributes, the free-space question,
// big-buffer mode and the clean-up after a cancel.

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCopyFilter(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	write(t, filepath.Join(src, "d", "a.txt"), "a")
	write(t, filepath.Join(src, "d", "b.md"), "b")
	write(t, filepath.Join(src, "d", "sub", "c.txt"), "c")
	write(t, filepath.Join(src, "d", "other", "d.md"), "d")
	write(t, filepath.Join(src, "d", "vendor", "e.txt"), "e")
	q := NewQueue(nil)
	p := start(t, q, Request{Kind: Copy, Sources: []string{filepath.Join(src, "d")}, Target: dst, Filter: "*.txt|vendor/"})
	if p.State != StateDone || p.Files != 2 {
		t.Fatalf("%+v", p)
	}
	if !exists(filepath.Join(dst, "d", "a.txt")) || !exists(filepath.Join(dst, "d", "sub", "c.txt")) {
		t.Error("matching files missing")
	}
	for _, n := range []string{"b.md", "other", "vendor"} {
		if exists(filepath.Join(dst, "d", n)) {
			t.Errorf("%s copied", n)
		}
	}
}

func TestCopyMask(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	write(t, filepath.Join(src, "a.txt"), "a")
	write(t, filepath.Join(src, "b.txt"), "b")
	q := NewQueue(nil)
	p := start(t, q, Request{Kind: Copy, Sources: []string{filepath.Join(src, "a.txt"), filepath.Join(src, "b.txt")},
		Target: dst, Rename: "*.bak"})
	if p.State != StateDone || read(t, filepath.Join(dst, "a.bak")) != "a" || read(t, filepath.Join(dst, "b.bak")) != "b" {
		t.Fatalf("%+v", p)
	}
}

func TestCopyMakesTarget(t *testing.T) {
	src := t.TempDir()
	write(t, filepath.Join(src, "a"), "a")
	dst := filepath.Join(t.TempDir(), "new", "deeper")
	p := start(t, NewQueue(nil), Request{Kind: Copy, Sources: []string{filepath.Join(src, "a")}, Target: dst})
	if p.State != StateDone || read(t, filepath.Join(dst, "a")) != "a" {
		t.Fatalf("%+v", p)
	}
}

func TestCopyLinkModes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	src := t.TempDir()
	write(t, filepath.Join(src, "real", "f.txt"), "f")
	os.MkdirAll(filepath.Join(src, "d"), 0o755)
	os.Symlink(filepath.Join(src, "real"), filepath.Join(src, "d", "dirlink"))
	os.Symlink(filepath.Join(src, "real", "f.txt"), filepath.Join(src, "d", "filelink"))
	os.Symlink("..", filepath.Join(src, "d", "loop"))
	q := NewQueue(nil)

	dst := t.TempDir()
	p := start(t, q, Request{Kind: Copy, Sources: []string{filepath.Join(src, "d")}, Target: dst, Links: LinksFollow})
	if fi, err := os.Lstat(filepath.Join(dst, "d", "dirlink")); err != nil || !fi.IsDir() {
		t.Fatalf("followed folder link not a folder: %v %v", fi, err)
	}
	if fi, _ := os.Lstat(filepath.Join(dst, "d", "filelink")); fi == nil || !fi.Mode().IsRegular() {
		t.Error("followed file link not a file")
	}
	if read(t, filepath.Join(dst, "d", "dirlink", "f.txt")) != "f" {
		t.Error("content of followed link missing")
	}
	if p.State != StateFailed || !strings.Contains(strings.Join(p.Errors, " "), "leads back") {
		t.Errorf("loop not reported: %+v", p)
	}

	dst = t.TempDir()
	p = start(t, q, Request{Kind: Copy, Sources: []string{filepath.Join(src, "d")}, Target: dst, Links: LinksSkip})
	if p.State != StateDone || exists(filepath.Join(dst, "d", "dirlink")) || exists(filepath.Join(dst, "d", "loop")) {
		t.Errorf("links not skipped: %+v", p)
	}
}

func TestCopyVerifyAndBigBuffer(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	write(t, filepath.Join(src, "a"), strings.Repeat("abc", 100000))
	write(t, filepath.Join(src, "b"), "")
	p := start(t, NewQueue(nil), Request{Kind: Copy, Sources: []string{filepath.Join(src, "a"), filepath.Join(src, "b")},
		Target: dst, Verify: true, BufferSize: 8 << 20, NoCache: true})
	if p.State != StateDone || p.Verified != 2 || read(t, filepath.Join(dst, "a")) != strings.Repeat("abc", 100000) {
		t.Fatalf("%+v", p)
	}
}

func TestVerifyDetectsDifference(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "f")
	write(t, f, "hello")
	if err := verifyFile(f, 0, 5, wmCreate, []byte("not the hash")); err != errVerify {
		t.Fatalf("got %v", err)
	}
}

func TestMoveAcrossVolumes(t *testing.T) {
	orig := sameVolume
	sameVolume = func(a, b string) bool { return false }
	defer func() { sameVolume = orig }()
	src, dst := t.TempDir(), t.TempDir()
	write(t, filepath.Join(src, "d", "a"), "a")
	write(t, filepath.Join(src, "d", "sub", "b"), "b")
	write(t, filepath.Join(src, "d", "keep"), "src-keep")
	write(t, filepath.Join(dst, "d", "keep"), "dst-keep")
	p := start(t, NewQueue(nil), Request{Kind: Move, Sources: []string{filepath.Join(src, "d")}, Target: dst,
		Overwrite: Skip, Verify: true})
	if p.State != StateDone || p.Verified != 2 {
		t.Fatalf("%+v", p)
	}
	if read(t, filepath.Join(dst, "d", "sub", "b")) != "b" || exists(filepath.Join(src, "d", "sub")) || exists(filepath.Join(src, "d", "a")) {
		t.Error("files not moved, or empty source folder left")
	}
	if read(t, filepath.Join(src, "d", "keep")) != "src-keep" {
		t.Error("skipped file removed from the source")
	}
}

func TestMoveSameVolumeMergeRenames(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	write(t, filepath.Join(src, "d", "a"), "a")
	write(t, filepath.Join(dst, "d", "other"), "o")
	p := start(t, NewQueue(nil), Request{Kind: Move, Sources: []string{filepath.Join(src, "d")}, Target: dst})
	if p.State != StateDone || read(t, filepath.Join(dst, "d", "a")) != "a" || exists(filepath.Join(src, "d")) {
		t.Fatalf("%+v", p)
	}
}

func TestMoveCaseOnly(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.txt"), "a")
	p := start(t, NewQueue(nil), Request{Kind: Move, Sources: []string{filepath.Join(dir, "a.txt")}, Target: dir, Rename: "A.txt"})
	if p.State != StateDone {
		t.Fatalf("%+v", p)
	}
	es, _ := os.ReadDir(dir)
	if len(es) != 1 || es[0].Name() != "A.txt" {
		t.Errorf("entries %v", es)
	}
}

func TestCopySameFolderAutoName(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.txt"), "a")
	p := start(t, NewQueue(nil), Request{Kind: Copy, Sources: []string{filepath.Join(dir, "a.txt")}, Target: dir, Overwrite: AutoRenameSource})
	if p.State != StateDone || read(t, filepath.Join(dir, "a(2).txt")) != "a" {
		t.Fatalf("%+v", p)
	}
}

func TestCopyKeepsMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix modes")
	}
	src := t.TempDir()
	f := filepath.Join(src, "x")
	write(t, f, "x")
	os.Chmod(f, 0o751)
	old := time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)
	os.Chtimes(f, old, old)
	q := NewQueue(nil)
	dst := t.TempDir()
	start(t, q, Request{Kind: Copy, Sources: []string{f}, Target: dst})
	fi, _ := os.Stat(filepath.Join(dst, "x"))
	if fi.Mode().Perm() != 0o751 || !fi.ModTime().Equal(old) {
		t.Errorf("mode %v time %v", fi.Mode(), fi.ModTime())
	}
	if !atimeOf(fi).Equal(old) {
		t.Errorf("access time %v", atimeOf(fi))
	}
	dst = t.TempDir()
	start(t, q, Request{Kind: Copy, Sources: []string{f}, Target: dst, NoDates: true, NoPerms: true})
	fi, _ = os.Stat(filepath.Join(dst, "x"))
	if fi.Mode().Perm() == 0o751 || fi.ModTime().Equal(old) {
		t.Errorf("NoPerms/NoDates ignored: %v %v", fi.Mode(), fi.ModTime())
	}
}

func TestCopyDirModeAndDate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix modes")
	}
	src, dst := t.TempDir(), t.TempDir()
	d := filepath.Join(src, "d")
	write(t, filepath.Join(d, "f"), "f")
	old := time.Date(2005, 1, 1, 0, 0, 0, 0, time.UTC)
	os.Chmod(d, 0o555) // read-only folder: the copy must still get its file
	os.Chtimes(d, old, old)
	defer os.Chmod(d, 0o755)
	p := start(t, NewQueue(nil), Request{Kind: Copy, Sources: []string{d}, Target: dst})
	defer os.Chmod(filepath.Join(dst, "d"), 0o755)
	fi, _ := os.Stat(filepath.Join(dst, "d"))
	if p.State != StateDone || fi.Mode().Perm() != 0o555 || !fi.ModTime().Equal(old) || read(t, filepath.Join(dst, "d", "f")) != "f" {
		t.Fatalf("%+v %v %v", p, fi.Mode(), fi.ModTime())
	}
}

func TestNoSpaceQuestion(t *testing.T) {
	orig := freeSpace
	freeSpace = func(string) (int64, bool) { return 1, true }
	defer func() { freeSpace = orig }()
	src, dst := t.TempDir(), t.TempDir()
	write(t, filepath.Join(src, "f"), "too big")
	q := NewQueue(nil)
	var got Conflict
	q.AskFunc = func(p Progress) Answer { got = *p.Ask; return Answer{Choice: ChoiceCancel} }
	p := start(t, q, Request{Kind: Copy, Sources: []string{filepath.Join(src, "f")}, Target: dst})
	if p.State != StateCancelled || got.Type != AskNoSpace || got.Need != 7 || got.Free != 1 || exists(filepath.Join(dst, "f")) {
		t.Fatalf("%+v %+v", p, got)
	}
	q.AskFunc = func(p Progress) Answer { return Answer{Choice: ChoiceYes} }
	if p := start(t, q, Request{Kind: Copy, Sources: []string{filepath.Join(src, "f")}, Target: dst}); p.State != StateDone {
		t.Fatalf("%+v", p)
	}
	q.AskFunc = func(p Progress) Answer { t.Error("asked despite IgnoreSpace"); return Answer{Choice: ChoiceCancel} }
	start(t, q, Request{Kind: Copy, Sources: []string{filepath.Join(src, "f")}, Target: t.TempDir(), IgnoreSpace: true})
}

func TestCancelRemovesPartialFile(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	write(t, filepath.Join(src, "big"), strings.Repeat("x", 1<<20))
	q := NewQueue(nil)
	id, _ := q.Start(Request{Kind: Copy, Sources: []string{filepath.Join(src, "big")}, Target: dst, SpeedLimit: 128 << 10})
	waitFor(t, "first bytes", func() bool { p, _ := q.Get(id); return p.BytesDone > 0 })
	q.Cancel(id)
	p := q.Wait(id)
	if p.State != StateCancelled || exists(filepath.Join(dst, "big")) {
		t.Fatalf("%+v, partial file left: %v", p, exists(filepath.Join(dst, "big")))
	}
}
