package fileops

// Tests of delete, wipe and trash beyond fileops_test.go: the per-folder and read-only confirmations, that a folder
// holding a skipped file stays without extra errors, the wipe itself, and the trash fallback to a permanent delete.
// The real trash is never touched: trashMove (and trashBefore, trashLocate) are replaced.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"mormicommander/internal/trash"
)

func TestDeleteConfirmDirs(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a", "x"), "x")
	write(t, filepath.Join(dir, "b", "y"), "y")
	os.Mkdir(filepath.Join(dir, "empty"), 0o755)
	q := NewQueue(nil)
	var asked []string
	q.AskFunc = func(p Progress) Answer {
		asked = append(asked, filepath.Base(p.Ask.Source))
		if len(asked) == 1 {
			return Answer{Choice: ChoiceSkip}
		}
		return Answer{Choice: ChoiceYes}
	}
	p := start(t, q, Request{Kind: Delete, ConfirmDirs: true,
		Sources: []string{filepath.Join(dir, "a"), filepath.Join(dir, "b"), filepath.Join(dir, "empty")}})
	if p.State != StateDone || strings.Join(asked, ",") != "a,b" || p.Skipped != 1 {
		t.Fatalf("%+v asked %v", p, asked)
	}
	if !exists(filepath.Join(dir, "a", "x")) || exists(filepath.Join(dir, "b")) || exists(filepath.Join(dir, "empty")) {
		t.Error("wrong things deleted")
	}
}

func TestDeleteConfirmReadOnlyKeepsParents(t *testing.T) {
	dir := t.TempDir()
	ro := filepath.Join(dir, "d", "sub", "ro")
	write(t, ro, "r")
	write(t, filepath.Join(dir, "d", "other"), "o")
	os.Chmod(ro, 0o444)
	defer os.Chmod(ro, 0o644)
	q := NewQueue(nil)
	q.AskFunc = func(p Progress) Answer {
		if p.Ask.Type != AskDeleteReadOnly {
			t.Errorf("unexpected question %+v", p.Ask)
		}
		return Answer{Choice: ChoiceSkipAll}
	}
	p := start(t, q, Request{Kind: Delete, ConfirmReadOnly: true, Sources: []string{filepath.Join(dir, "d")}})
	if p.State != StateDone || len(p.Errors) != 0 || p.Skipped != 1 {
		t.Fatalf("%+v", p)
	}
	if !exists(ro) || exists(filepath.Join(dir, "d", "other")) {
		t.Error("wrong things deleted")
	}
}

func TestWipe(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "d", "secret"), "password")
	write(t, filepath.Join(dir, "f"), strings.Repeat("s", 3000))
	os.Chmod(filepath.Join(dir, "f"), 0o444)
	q := NewQueue(nil)
	p := start(t, q, Request{Kind: Wipe, Sources: []string{filepath.Join(dir, "d"), filepath.Join(dir, "f")}, WipePasses: 2})
	if p.State != StateDone || p.Bytes != 2*3008 || p.BytesDone != p.Bytes || p.FilesDone != 2 {
		t.Fatalf("%+v", p)
	}
	if es, _ := os.ReadDir(dir); len(es) != 0 {
		t.Errorf("left behind: %v", es)
	}
}

func TestOverwriteReplacesContent(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "f")
	write(t, f, strings.Repeat("A", 5000))
	q := NewQueue(nil)
	id, _ := q.Start(Request{Kind: Delete, Sources: []string{filepath.Join(dir, "missing")}})
	q.Wait(id)
	j := q.get(id)
	fi, _ := os.Stat(f)
	if err := j.overwrite(f, fi); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(f); fi.Size() != 0 {
		t.Errorf("size after overwrite %d", fi.Size())
	}
}

func TestTrashFallbackToDelete(t *testing.T) {
	orig := trashMove
	trashMove = func(context.Context, string) error { return errors.New("this volume has no trash") }
	defer func() { trashMove = orig }()
	dir := t.TempDir()
	write(t, filepath.Join(dir, "d", "x"), "x")
	q := NewQueue(nil)
	var got Conflict
	q.AskFunc = func(p Progress) Answer { got = *p.Ask; return Answer{Choice: ChoiceDelete} }
	p := start(t, q, Request{Kind: Trash, Sources: []string{filepath.Join(dir, "d")}, OnError: ErrorsAsk})
	if p.State != StateDone || p.FilesDone != 1 || exists(filepath.Join(dir, "d")) {
		t.Fatalf("%+v", p)
	}
	if got.Type != AskError || !hasChoice(got.Choices, ChoiceDelete) {
		t.Errorf("question %+v", got)
	}
	write(t, filepath.Join(dir, "e"), "e")
	p = start(t, NewQueue(nil), Request{Kind: Trash, Sources: []string{filepath.Join(dir, "e")}})
	if p.State != StateFailed || !exists(filepath.Join(dir, "e")) {
		t.Fatalf("default error policy: %+v", p)
	}
}

func TestDeleteReadOnlyFile(t *testing.T) {
	if runtime.GOOS != "windows" && os.Geteuid() == 0 {
		t.Skip()
	}
	dir := t.TempDir()
	f := filepath.Join(dir, "ro")
	write(t, f, "r")
	os.Chmod(f, 0o444)
	p := start(t, NewQueue(nil), Request{Kind: Delete, Sources: []string{f}})
	if p.State != StateDone || exists(f) {
		t.Fatalf("%+v", p)
	}
}

// TestTrashFindsPlaces: with FindTrashed, each "trash" log entry says where the item went (board issue #13, the undo
// journal); an item that can't be found has an empty place, and without FindTrashed nothing is looked up at all. The
// trash calls are fakes, so nothing is trashed or read in the real trash.
func TestTrashFindsPlaces(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.txt"), filepath.Join(dir, "b.txt")
	om, ob, ol := trashMove, trashBefore, trashLocate
	defer func() { trashMove, trashBefore, trashLocate = om, ob, ol }()
	trashMove = func(context.Context, string) error { return nil }
	var noted []string
	trashBefore = func(p string) (trash.Mark, bool) { noted = append(noted, p); return trash.Mark{}, true }
	n := 0
	trashLocate = func(trash.Mark) (string, bool) {
		n++
		if n == 1 {
			return "/Trash/a 2.txt", true
		}
		return "", false
	}
	run := func(find bool) map[string]string {
		got := map[string]string{}
		q := NewQueue(nil)
		var mu sync.Mutex
		q.Log = func(e Entry) {
			mu.Lock()
			defer mu.Unlock()
			if e.Action == "trash" {
				got[e.Source] = e.Trashed
			}
		}
		if p := start(t, q, Request{Kind: Trash, Sources: []string{a, b}, FindTrashed: find}); p.State != StateDone {
			t.Fatalf("%+v", p)
		}
		return got
	}
	if got := run(false); len(got) != 2 || got[a] != "" || len(noted) != 0 {
		t.Fatalf("without FindTrashed: %v, looked up %v", got, noted)
	}
	got := run(true)
	if got[a] != "/Trash/a 2.txt" || got[b] != "" || len(got) != 2 || len(noted) != 2 {
		t.Fatalf("with FindTrashed: %v, noted %v", got, noted)
	}
	if s := (Entry{Action: "trash", Source: a, Trashed: "/Trash/a 2.txt"}).String(); strings.Contains(s, "Trash/a 2") {
		t.Errorf("the place in the trash is not part of the log line: %q", s)
	}
}
