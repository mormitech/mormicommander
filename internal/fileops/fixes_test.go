package fileops

// Regression tests for the milestone fixes: empty and relative paths are refused before a job starts (a cleaned ""
// is the working folder), new names and masks can't step out of the target folder, an interrupted overwrite keeps
// the old target, an exclude-only filter keeps empty folders, and wipe leaves the data of a hard-linked file alone
// and never strands a folder under a random name.

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestValidateRefusesEmptyAndRelativePaths(t *testing.T) {
	dir := t.TempDir()
	abs := filepath.Join(dir, "a")
	tests := []struct {
		name string
		r    Request
	}{
		{"delete empty", Request{Kind: Delete, Sources: []string{""}}},
		{"delete dot", Request{Kind: Delete, Sources: []string{"."}}},
		{"trash relative", Request{Kind: Trash, Sources: []string{"keep.txt"}}},
		{"wipe empty", Request{Kind: Wipe, Sources: []string{abs, ""}}},
		{"copy dot", Request{Kind: Copy, Sources: []string{"."}, Target: dir}},
		{"move relative target", Request{Kind: Move, Sources: []string{abs}, Target: "out"}},
		{"rename dotdot", Request{Kind: Copy, Sources: []string{abs}, Target: dir, Rename: ".."}},
		{"rename dot", Request{Kind: Copy, Sources: []string{abs}, Target: dir, Rename: "."}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := validate(tc.r); err == nil {
				t.Fatalf("validate(%+v) accepted a bad request", tc.r)
			}
		})
	}
}

func TestDeleteEmptySourceKeepsWorkingFolder(t *testing.T) {
	dir := t.TempDir()
	keep := filepath.Join(dir, "keep.txt")
	write(t, keep, "x")
	t.Chdir(dir)
	q := NewQueue(nil)
	for _, k := range []Kind{Delete, Trash, Wipe} {
		if _, err := q.Start(Request{Kind: k, Sources: []string{""}}); err == nil {
			t.Fatalf("%s of \"\" was started", k)
		}
	}
	if !exists(keep) {
		t.Fatal("keep.txt in the working folder was removed")
	}
}

func TestMaskCannotEscapeTarget(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "src", "dir", "f.txt"), "x")
	out := filepath.Join(root, "out")
	tgt := filepath.Join(out, "tgt")
	if err := os.MkdirAll(tgt, 0o755); err != nil {
		t.Fatal(err)
	}
	q := NewQueue(nil)
	if got := ApplyMask("..*", "dir"); got != "." {
		t.Fatalf(`ApplyMask("..*", "dir") = %q; the test needs a mask that yields "."`, got)
	}
	// A wildcard mask whose result is "." or ".." must be refused per item.
	for _, mask := range []string{"..*", ".?"} {
		name := ApplyMask(mask, "dir")
		if checkName(name) == nil {
			continue // this mask gives a harmless name; nothing to check
		}
		p := start(t, q, Request{Kind: Copy, Sources: []string{filepath.Join(root, "src", "dir")}, Target: tgt, Rename: mask})
		if len(p.Errors) == 0 {
			t.Errorf("mask %q (name %q) was accepted", mask, name)
		}
	}
	if exists(filepath.Join(out, "f.txt")) || exists(filepath.Join(tgt, "f.txt")) {
		t.Fatal("a file landed outside its own folder")
	}
}

func TestParseTargetNeverReturnsDotNames(t *testing.T) {
	for _, in := range []string{"..", ".", "../..", "/x/.."} {
		_, name := ParseTarget(in, 1)
		if name == "." || name == ".." {
			t.Errorf("ParseTarget(%q) returned the name %q", in, name)
		}
	}
}

func TestCancelledOverwriteKeepsOldTarget(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	big := bytes.Repeat([]byte("n"), 4<<20)
	if err := os.WriteFile(filepath.Join(src, "f"), big, 0o644); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dst, "f"), "old contents")
	q := NewQueue(nil)
	id, err := q.Start(Request{Kind: Copy, Sources: []string{filepath.Join(src, "f")}, Target: dst, Overwrite: Overwrite,
		SpeedLimit: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	q.Cancel(id)
	p := q.Wait(id)
	if p.State != StateCancelled {
		t.Fatalf("state %s", p.State)
	}
	if got := read(t, filepath.Join(dst, "f")); got != "old contents" {
		t.Fatalf("old target damaged: %d bytes", len(got))
	}
	entries, _ := os.ReadDir(dst)
	if len(entries) != 1 {
		t.Fatalf("temporary file left behind: %v", entries)
	}
}

func TestOverwriteReplacesTarget(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	write(t, filepath.Join(src, "f"), "new")
	write(t, filepath.Join(dst, "f"), "old contents that are longer")
	q := NewQueue(nil)
	p := start(t, q, Request{Kind: Copy, Sources: []string{filepath.Join(src, "f")}, Target: dst, Overwrite: Overwrite, Verify: true})
	if p.State != StateDone || len(p.Errors) != 0 || p.Verified != 1 {
		t.Fatalf("%+v", p)
	}
	if got := read(t, filepath.Join(dst, "f")); got != "new" {
		t.Fatalf("target = %q", got)
	}
	entries, _ := os.ReadDir(dst)
	if len(entries) != 1 {
		t.Fatalf("temporary file left behind: %v", entries)
	}
}

func TestExcludeOnlyFilterKeepsEmptyFolders(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	write(t, filepath.Join(src, "d", "a.txt"), "a")
	write(t, filepath.Join(src, "d", "b.bak"), "b")
	if err := os.MkdirAll(filepath.Join(src, "d", "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	q := NewQueue(nil)
	p := start(t, q, Request{Kind: Copy, Sources: []string{filepath.Join(src, "d")}, Target: dst, Filter: "|*.bak"})
	if p.State != StateDone {
		t.Fatalf("%+v", p)
	}
	if !exists(filepath.Join(dst, "d", "empty")) {
		t.Error("an exclude-only filter dropped an empty folder")
	}
	if exists(filepath.Join(dst, "d", "b.bak")) || !exists(filepath.Join(dst, "d", "a.txt")) {
		t.Error("filter not applied")
	}
	// An include filter still prunes folders without matching files.
	dst2 := t.TempDir()
	start(t, q, Request{Kind: Copy, Sources: []string{filepath.Join(src, "d")}, Target: dst2, Filter: "*.txt"})
	if exists(filepath.Join(dst2, "d", "empty")) {
		t.Error("an include filter kept an empty folder")
	}
}

func TestWipeLeavesHardLinkedData(t *testing.T) {
	if runtime.GOOS == "windows" || runtime.GOOS == "js" || runtime.GOOS == "wasip1" {
		t.Skip("hard link counts are checked on Unix here")
	}
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	write(t, a, "shared data")
	if err := os.Link(a, b); err != nil {
		t.Skip("hard links not available:", err)
	}
	q := NewQueue(nil)
	p := start(t, q, Request{Kind: Wipe, Sources: []string{a}})
	if p.State != StateDone || len(p.Errors) != 0 {
		t.Fatalf("%+v", p)
	}
	if exists(a) {
		t.Error("a still exists")
	}
	if got := read(t, b); got != "shared data" {
		t.Fatalf("the other name lost its data: %q", got)
	}
}

func TestWipeFailureKeepsFolderName(t *testing.T) {
	dir := t.TempDir()
	folder := filepath.Join(dir, "folder")
	write(t, filepath.Join(folder, "f"), "x")
	q := NewQueue(nil)
	// Right before the folder itself is wiped, a new file appears in it, so the folder can't be removed.
	q.beforeFile = func(p string) {
		if p == folder {
			write(t, filepath.Join(folder, "late"), "y")
		}
	}
	p := start(t, q, Request{Kind: Wipe, Sources: []string{folder}})
	if len(p.Errors) == 0 {
		t.Fatalf("expected an error: %+v", p)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || entries[0].Name() != "folder" {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("folder left under another name: %v", names)
	}
}
