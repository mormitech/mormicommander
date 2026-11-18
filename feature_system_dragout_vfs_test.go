// Tests for dragging files out of archives and servers (feature_system_dragout_vfs.go): which paths are copied out,
// the size and count limits (checked before anything is written), the per-drag folder with the copies of files and
// folders, the cleanup on failure and when the button is let go, and the cleanup of old per-drag folders, which never
// touches anything but "drag-…" folders inside "mormicommander-dragout". The native drag itself needs a window and a
// mouse, so it is not started here.
package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// dragZip makes a zip with a file, a folder with two files and a big file, and a router that reads it.
func dragZip(t *testing.T) (arc string, root string) {
	t.Helper()
	dir := t.TempDir()
	arc = filepath.Join(dir, "a.zip")
	arcEditZip(t, arc, map[string]string{
		"one.txt":       "1",
		"sub/two.txt":   "22",
		"sub/three.txt": "333",
		"big.bin":       strings.Repeat("x", 4096),
	})
	return arc, filepath.Join(t.TempDir(), dragTempRootName)
}

func TestDragVfsSources(t *testing.T) {
	r := arcEditRouter(t)
	arc, _ := dragZip(t)
	got, err := dragVfsSources(r, []string{filepath.Join(arc, "one.txt"), filepath.Join(arc, "one.txt"), filepath.Join(arc, "sub")})
	if err != nil || len(got) != 2 {
		t.Fatalf("dragVfsSources = %v, %v", got, err)
	}
	for name, in := range map[string][]string{
		"none":  nil,
		"empty": {" "},
		"local": {arc}, // the archive file itself is a local file: Start drags it
	} {
		if _, err := dragVfsSources(r, in); err == nil {
			t.Errorf("%s: %v was accepted", name, in)
		}
	}
}

func TestDragCopyOut(t *testing.T) {
	r := arcEditRouter(t)
	arc, root := dragZip(t)
	var seen []dragPrep
	dir, copies, err := dragCopyOut(context.Background(), r, []string{filepath.Join(arc, "one.txt"), filepath.Join(arc, "sub")}, root, 1<<20, 100,
		func(p dragPrep) { seen = append(seen, p) })
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(dir) != root || !strings.HasPrefix(filepath.Base(dir), dragTempPrefix) {
		t.Errorf("per-drag folder %s is not a drag- folder in %s", dir, root)
	}
	want := []string{filepath.Join(dir, "one.txt"), filepath.Join(dir, "sub")}
	if len(copies) != 2 || copies[0] != want[0] || copies[1] != want[1] {
		t.Errorf("copies = %v, want %v", copies, want)
	}
	for p, c := range map[string]string{"one.txt": "1", "sub/two.txt": "22", "sub/three.txt": "333"} {
		b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(p)))
		if err != nil || string(b) != c {
			t.Errorf("%s = %q, %v", p, b, err)
		}
	}
	if len(seen) == 0 || seen[len(seen)-1].Done != 6 || seen[len(seen)-1].Total != 6 {
		t.Errorf("progress = %+v", seen)
	}
}

func TestDragCopyOutLimits(t *testing.T) {
	r := arcEditRouter(t)
	arc, root := dragZip(t)
	big := []string{filepath.Join(arc, "big.bin")}
	if _, _, err := dragCopyOut(context.Background(), r, big, root, 1000, 100, nil); err == nil || !strings.Contains(err.Error(), "too big") {
		t.Errorf("a drag over the size limit: %v", err)
	}
	if _, _, err := dragCopyOut(context.Background(), r, []string{filepath.Join(arc, "sub")}, root, 1<<20, 2, nil); err == nil || !strings.Contains(err.Error(), "Too many") {
		t.Errorf("a drag over the count limit: %v", err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Errorf("a refused drag wrote something: %v", err)
	}
}

func TestDragCopyOutReleased(t *testing.T) {
	r := arcEditRouter(t)
	arc, root := dragZip(t)
	ctx, cancel := context.WithCancelCause(context.Background())
	n := 0
	_, _, err := dragCopyOut(ctx, r, []string{filepath.Join(arc, "one.txt"), filepath.Join(arc, "sub")}, root, 1<<20, 100, func(dragPrep) {
		if n++; n == 2 {
			cancel(errDragReleased)
		}
	})
	if err == nil || !errors.Is(context.Cause(ctx), errDragReleased) {
		t.Fatalf("let go while copying: %v", err)
	}
	if es, _ := os.ReadDir(root); len(es) != 0 {
		t.Errorf("the per-drag folder is left behind: %v", es)
	}
}

func TestWatchButton(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	go watchButton(ctx, cancel, func() bool { return false }) // the button is up
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("letting go of the button did not stop the copying")
	}
	if !errors.Is(context.Cause(ctx), errDragReleased) {
		t.Errorf("cause = %v", context.Cause(ctx))
	}
}

func TestRemoveDragTempOnlyDragFolders(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, dragTempRootName)
	mk := func(p string) string {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	drag := mk(filepath.Join(root, "drag-1"))
	other := mk(filepath.Join(root, "keep"))
	outside := mk(filepath.Join(base, "drag-2"))
	nested := mk(filepath.Join(drag, "drag-3"))
	if removeDragTemp(root, other) || removeDragTemp(root, outside) || removeDragTemp(base, outside) || removeDragTemp(root, nested) {
		t.Error("removed something that is not a per-drag folder")
	}
	link := filepath.Join(root, "drag-link")
	if os.Symlink(other, link) == nil && removeDragTemp(root, link) {
		t.Error("removed through a link")
	}
	if !removeDragTemp(root, drag) {
		t.Error("the per-drag folder was not removed")
	}
	for _, p := range []string{other, outside} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s is gone", p)
		}
	}
}

func TestCleanDragTemp(t *testing.T) {
	root := filepath.Join(t.TempDir(), dragTempRootName)
	old := filepath.Join(root, "drag-old")
	fresh := filepath.Join(root, "drag-new")
	keep := filepath.Join(root, "notes")
	for _, d := range []string{old, fresh, keep} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	file := filepath.Join(root, "drag-file")
	os.WriteFile(file, []byte("x"), 0o644)
	long := time.Now().Add(-2 * time.Hour)
	for _, p := range []string{old, keep, file} {
		os.Chtimes(p, long, long)
	}
	if n := cleanDragTemp(root, time.Hour, time.Now()); n != 1 {
		t.Errorf("removed %d folders, want 1", n)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Error("the old per-drag folder is still there")
	}
	for _, p := range []string{fresh, keep, file} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s was removed", p)
		}
	}
	if n := cleanDragTemp(filepath.Join(root, "missing"), time.Hour, time.Now()); n != 0 {
		t.Error("cleaning a missing folder removed something")
	}
}

func TestSystemDragStartFromDemo(t *testing.T) {
	d := &SystemDrag{app: NewApp(startOpts{Demo: true})}
	if _, err := d.StartFrom([]string{"/x.zip/a"}); err == nil {
		t.Error("the demo copied files out for a drag")
	}
}
