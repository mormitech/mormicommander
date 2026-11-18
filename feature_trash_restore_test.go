package main

// Tests of putting back items picked in a trash folder (F-501). Only temporary folders are used; the real trash is
// never read or changed: an item outside the system's trash folders is refused, so nothing here can move a real file.

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"mormicommander/internal/undo"
)

func TestTrashOriginalFromJournal(t *testing.T) {
	j, u := newUndoJobs(t)
	dir := t.TempDir()
	from, in := filepath.Join(dir, "work", "a.txt"), filepath.Join(dir, "Trash", "a.txt")
	u.j.Add(undo.Entry{Kind: undo.Trash, Items: []undo.Item{{From: filepath.Join(dir, "old", "a.txt"), To: in}}})
	u.j.Add(undo.Entry{Kind: undo.Rename, Items: []undo.Item{{From: filepath.Join(dir, "x"), To: in}}})
	u.j.Add(undo.Entry{Kind: undo.Trash, Items: []undo.Item{{From: from, To: in}}})
	got := j.TrashOriginals([]string{in, filepath.Join(dir, "Trash", "unknown.txt"), "relative"})
	if len(got) != 3 || got[0].Original != from {
		t.Fatalf("originals: %+v", got)
	}
	if got[1].Original != "" || got[1].Error == "" || got[2].Original != "" || got[2].Error == "" {
		t.Fatalf("unknown items: %+v", got)
	}
}

// An item that is not in a trash folder of its old place's volume is never moved, even when the journal names it.
func TestRestoreFromTrashRefusesOutsideTheTrash(t *testing.T) {
	j, u := newUndoJobs(t)
	dir := t.TempDir()
	in, from := filepath.Join(dir, "notatrash", "a.txt"), filepath.Join(dir, "work", "a.txt")
	mkfile(t, in, "keep")
	u.j.Add(undo.Entry{Kind: undo.Trash, Items: []undo.Item{{From: from, To: in}}})
	res := j.RestoreFromTrash([]string{in})
	if len(res) != 1 || res[0].Error == "" {
		t.Fatalf("result: %+v", res)
	}
	if b, err := os.ReadFile(in); err != nil || string(b) != "keep" {
		t.Fatal("the file was moved")
	}
	if _, err := os.Lstat(from); err == nil {
		t.Fatal("something appeared at the old place")
	}
}

// On Linux the home trash follows XDG_DATA_HOME, so a whole put-back can run in a temporary folder.
func TestRestoreFromTrashXDG(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the freedesktop.org trash is Linux's")
	}
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	j, _ := newUndoJobs(t)
	if j.TrashFolder() != filepath.Join(data, "Trash", "files") {
		t.Fatalf("trash folder %q", j.TrashFolder())
	}
	src := filepath.Join(data, "work", "n.txt") // same volume as the home trash
	in := filepath.Join(data, "Trash", "files", "n.txt")
	mkfile(t, in, "n")
	mkfile(t, filepath.Join(data, "Trash", "info", "n.txt.trashinfo"), "[Trash Info]\nPath="+src+"\nDeletionDate=2026-01-02T03:04:05\n")
	res := j.RestoreFromTrash([]string{in})
	if len(res) != 1 || res[0].Error != "" || res[0].Original != src {
		t.Fatalf("result: %+v", res)
	}
	if b, _ := os.ReadFile(src); string(b) != "n" {
		t.Fatal("not put back")
	}
	// put back never overwrites: a second item for the same place stays in the trash
	mkfile(t, in, "again")
	mkfile(t, filepath.Join(data, "Trash", "info", "n.txt.trashinfo"), "[Trash Info]\nPath="+src+"\nDeletionDate=2026-01-02T03:04:06\n")
	if res := j.RestoreFromTrash([]string{in}); res[0].Error == "" {
		t.Fatal("put back over an existing file")
	}
	if b, _ := os.ReadFile(src); string(b) != "n" {
		t.Fatal("the file at the old place was changed")
	}
}
