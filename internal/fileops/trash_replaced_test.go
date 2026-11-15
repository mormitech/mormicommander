package fileops

// Board #99: with Request.TrashReplaced a file or link that an overwrite replaces goes to the trash first; off (the
// default) it is replaced in place as before. The trash is a stand-in that moves into a temporary folder.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// binTrash replaces trashMove with a move into a fresh folder and returns that folder and the paths it was given.
func binTrash(t *testing.T) (string, *[]string) {
	t.Helper()
	bin := t.TempDir()
	var got []string
	orig := trashMove
	trashMove = func(_ context.Context, p string) error {
		got = append(got, p)
		return os.Rename(p, filepath.Join(bin, filepath.Base(p)))
	}
	t.Cleanup(func() { trashMove = orig })
	return bin, &got
}

func TestTrashReplacedCopy(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	write(t, filepath.Join(src, "a.txt"), "new")
	write(t, filepath.Join(dst, "a.txt"), "old")
	write(t, filepath.Join(src, "b.txt"), "b") // nothing to replace

	// off: replaced in place, nothing trashed
	bin, got := binTrash(t)
	p := start(t, NewQueue(nil), Request{Kind: Copy, Sources: []string{filepath.Join(src, "a.txt")}, Target: dst, Overwrite: Overwrite})
	if p.State != StateDone || read(t, filepath.Join(dst, "a.txt")) != "new" || len(*got) != 0 {
		t.Fatalf("off: %+v trashed %v", p, *got)
	}

	// on: the old file is in the trash with its contents, the new one in its place; a new file trashes nothing
	write(t, filepath.Join(dst, "a.txt"), "old")
	bin, got = binTrash(t)
	p = start(t, NewQueue(nil), Request{Kind: Copy, Sources: []string{filepath.Join(src, "a.txt"), filepath.Join(src, "b.txt")},
		Target: dst, Overwrite: Overwrite, TrashReplaced: true})
	if p.State != StateDone || read(t, filepath.Join(dst, "a.txt")) != "new" || read(t, filepath.Join(dst, "b.txt")) != "b" {
		t.Fatalf("on: %+v", p)
	}
	if len(*got) != 1 || (*got)[0] != filepath.Join(dst, "a.txt") || read(t, filepath.Join(bin, "a.txt")) != "old" {
		t.Fatalf("on: trashed %v", *got)
	}

	// a trash that fails leaves the old file whole and no temporary file behind
	write(t, filepath.Join(dst, "a.txt"), "old")
	orig := trashMove
	trashMove = func(context.Context, string) error { return errors.New("no trash here") }
	defer func() { trashMove = orig }()
	p = start(t, NewQueue(nil), Request{Kind: Copy, Sources: []string{filepath.Join(src, "a.txt")}, Target: dst, Overwrite: Overwrite, TrashReplaced: true})
	if p.State != StateFailed || read(t, filepath.Join(dst, "a.txt")) != "old" {
		t.Fatalf("failing trash: %+v", p)
	}
	if ents, _ := os.ReadDir(dst); len(ents) != 2 {
		t.Errorf("failing trash: %d entries left, want a.txt and b.txt", len(ents))
	}
}

func TestTrashReplacedMoveAndLink(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "src"), filepath.Join(dir, "dst")
	write(t, filepath.Join(src, "a.txt"), "new")
	write(t, filepath.Join(dst, "a.txt"), "old")
	bin, got := binTrash(t)
	p := start(t, NewQueue(nil), Request{Kind: Move, Sources: []string{filepath.Join(src, "a.txt")}, Target: dst, Overwrite: Overwrite, TrashReplaced: true})
	if p.State != StateDone || read(t, filepath.Join(dst, "a.txt")) != "new" || exists(filepath.Join(src, "a.txt")) {
		t.Fatalf("move: %+v", p)
	}
	if len(*got) != 1 || read(t, filepath.Join(bin, "a.txt")) != "old" {
		t.Fatalf("move: trashed %v", *got)
	}
	if runtime.GOOS == "windows" {
		return // making links needs extra rights there
	}
	// a link copied over an old link: the old link itself goes to the trash, its target is untouched
	write(t, filepath.Join(dir, "target-old"), "keep me")
	os.Symlink("target-new", filepath.Join(src, "lnk"))
	os.Symlink(filepath.Join(dir, "target-old"), filepath.Join(dst, "lnk"))
	bin, got = binTrash(t)
	p = start(t, NewQueue(nil), Request{Kind: Copy, Sources: []string{filepath.Join(src, "lnk")}, Target: dst, Overwrite: Overwrite, TrashReplaced: true})
	if p.State != StateDone {
		t.Fatalf("link: %+v", p)
	}
	if tgt, _ := os.Readlink(filepath.Join(dst, "lnk")); tgt != "target-new" {
		t.Errorf("link points to %q", tgt)
	}
	if tgt, err := os.Readlink(filepath.Join(bin, "lnk")); err != nil || tgt != filepath.Join(dir, "target-old") {
		t.Errorf("old link not trashed as a link: %q %v", tgt, err)
	}
	if read(t, filepath.Join(dir, "target-old")) != "keep me" {
		t.Error("the old link's target was touched")
	}
}
