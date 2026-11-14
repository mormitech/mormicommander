package undo

// Tests of the undo journal: it survives a restart, keeps at most max entries, and every kind of operation is put back
// (or left alone, with a reason) as the disk is now. Everything happens in temporary folders.

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func write(t *testing.T, p, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func exists(p string) bool { _, err := os.Lstat(p); return err == nil }

func same(a, b string) bool { return true }

func TestJournalSurvivesRestartAndTrims(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cfg", "undo.json")
	j := Open(path, 3)
	if _, ok := j.Add(Entry{Kind: Copy}); ok {
		t.Error("an empty entry was recorded")
	}
	for i := 0; i < 5; i++ {
		j.Add(Entry{Kind: Rename, Items: []Item{{From: "/a" + string(rune('0'+i)), To: "/b"}}})
	}
	if l := j.List(); len(l) != 3 || l[0].ID != 3 || l[2].ID != 5 {
		t.Fatalf("trim: %+v", l)
	}
	j2 := Open(path, 3)
	last, ok := j2.Last()
	if !ok || last.ID != 5 || last.Items[0].From != "/a4" {
		t.Fatalf("after restart: %+v %v", last, ok)
	}
	e, _ := j2.Add(Entry{Kind: MkDir, Items: []Item{{From: "/x", To: "/x"}}})
	if e.ID != 6 {
		t.Errorf("numbers go on after a restart: %d", e.ID)
	}
	if !j2.Remove(6) || j2.Remove(6) {
		t.Error("remove")
	}
	if _, ok := j2.Get(4); !ok {
		t.Error("get")
	}
	j2.Clear()
	if len(Open(path, 3).List()) != 0 {
		t.Error("clear was not saved")
	}
	// a broken file gives an empty journal
	write(t, path, "{nonsense")
	if len(Open(path, 0).List()) != 0 {
		t.Error("broken file")
	}
}

func TestUndoRenameAndMove(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.txt"), filepath.Join(dir, "b.txt")
	write(t, b, "x")
	sub := filepath.Join(dir, "sub")
	moved := filepath.Join(dir, "other", "sub")
	write(t, filepath.Join(moved, "f"), "y")

	p := PlanUndo(Entry{Kind: Rename, Items: []Item{{From: a, To: b}}}, same)
	if len(p.Steps) != 1 || p.Steps[0].Op != OpRename || len(p.Problems) != 0 {
		t.Fatalf("rename plan: %+v", p)
	}
	jobs, done, probs := Apply(p.Steps)
	if done != 1 || len(jobs) != 0 || len(probs) != 0 || !exists(a) || exists(b) {
		t.Fatalf("rename undo: %d %v %v", done, jobs, probs)
	}

	// a move on the same volume is a rename back; across volumes it becomes a job
	e := Entry{Kind: Move, Items: []Item{{From: sub, To: moved, Dir: true}}}
	if p := PlanUndo(e, func(x, y string) bool { return false }); len(p.Steps) != 1 || p.Steps[0].Op != OpMove {
		t.Fatalf("cross-volume plan: %+v", p)
	}
	p = PlanUndo(e, same)
	jobs, done, _ = Apply(p.Steps)
	if done != 1 || len(jobs) != 0 || !exists(filepath.Join(sub, "f")) || exists(moved) {
		t.Fatal("move undo")
	}
}

func TestUndoNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.txt"), filepath.Join(dir, "b.txt")
	write(t, b, "renamed")
	write(t, a, "a new file with the old name")
	p := PlanUndo(Entry{Kind: Rename, Items: []Item{{From: a, To: b}}}, same)
	if len(p.Steps) != 0 || len(p.Problems) != 1 || !strings.Contains(p.Problems[0], "is there again") {
		t.Fatalf("plan: %+v", p)
	}
	// the old name taken between the plan and the step
	os.Remove(a)
	p = PlanUndo(Entry{Kind: Rename, Items: []Item{{From: a, To: b}}}, same)
	write(t, a, "late")
	_, done, probs := Apply(p.Steps)
	if done != 0 || len(probs) != 1 {
		t.Fatalf("late clash: %d %v", done, probs)
	}
	if s, _ := os.ReadFile(a); string(s) != "late" {
		t.Error("the file with the old name was overwritten")
	}
	// gone since
	p = PlanUndo(Entry{Kind: Move, Items: []Item{{From: filepath.Join(dir, "x"), To: filepath.Join(dir, "gone")}}}, same)
	if len(p.Steps) != 0 || !strings.Contains(p.Problems[0], "not there any more") {
		t.Fatalf("gone: %+v", p)
	}
}

func TestUndoCopy(t *testing.T) {
	dir := t.TempDir()
	src, cp := filepath.Join(dir, "a.txt"), filepath.Join(dir, "b", "a.txt")
	write(t, src, "abc")
	write(t, cp, "abc")
	fi, _ := os.Lstat(cp)
	it := Item{From: src, To: cp, Size: fi.Size(), MTime: fi.ModTime().UnixMilli()}
	folder := filepath.Join(dir, "b", "folder")
	os.MkdirAll(folder, 0o755)
	e := Entry{Kind: Copy, Items: []Item{it, {From: filepath.Join(dir, "folder"), To: folder, Dir: true}}, Lost: 1}
	p := PlanUndo(e, same)
	if len(p.Steps) != 2 || p.Steps[0].Op != OpRemove || p.Steps[0].To != folder || len(p.Problems) != 1 {
		t.Fatalf("plan: %+v", p)
	}
	jobs, done, _ := Apply(p.Steps)
	if done != 0 || len(jobs) != 2 || !exists(cp) {
		t.Fatal("copies are removed by jobs, not here")
	}
	// a copy changed since is left alone
	write(t, cp, "abcdef")
	if p := PlanUndo(Entry{Kind: Copy, Items: []Item{it}}, same); len(p.Steps) != 0 || !strings.Contains(p.Problems[0], "changed") {
		t.Fatalf("changed copy: %+v", p)
	}
	old := time.Now().Add(-time.Hour)
	write(t, cp, "abc")
	os.Chtimes(cp, old, old)
	if p := PlanUndo(Entry{Kind: Copy, Items: []Item{it}}, same); len(p.Steps) != 0 {
		t.Fatalf("copy with another date: %+v", p)
	}
}

func TestUndoMkDir(t *testing.T) {
	dir := t.TempDir()
	top, deep := filepath.Join(dir, "a"), filepath.Join(dir, "a", "b", "c")
	os.MkdirAll(deep, 0o755)
	e := Entry{Kind: MkDir, Items: []Item{{From: top, To: deep, Dir: true}}}
	// not empty: left alone
	write(t, filepath.Join(top, "b", "note.txt"), "x")
	if p := PlanUndo(e, same); len(p.Steps) != 0 || !strings.Contains(p.Problems[0], "not empty") {
		t.Fatalf("not empty: %+v", p)
	}
	os.Remove(filepath.Join(top, "b", "note.txt"))
	p := PlanUndo(e, same)
	_, done, probs := Apply(p.Steps)
	if done != 1 || len(probs) != 0 || exists(top) || !exists(dir) {
		t.Fatalf("mkdir undo: %d %v", done, probs)
	}
	// a folder outside what was made is never removed
	if p := PlanUndo(Entry{Kind: MkDir, Items: []Item{{From: filepath.Join(dir, "x"), To: dir}}}, same); len(p.Steps) != 0 {
		t.Fatalf("outside: %+v", p)
	}
	if err := removeChain(dir, filepath.Join(dir, "x")); err == nil {
		t.Error("removeChain outside")
	}
}

func TestDescribe(t *testing.T) {
	for _, c := range []struct {
		e    Entry
		want string
	}{
		{Entry{Kind: Rename, Items: []Item{{From: "/d/a.txt", To: "/d/b.txt"}}}, "the rename of a.txt to b.txt"},
		{Entry{Kind: Copy, Items: []Item{{From: "/d/a.txt", To: "/e/a.txt"}}}, "the copy of a.txt to /e"},
		{Entry{Kind: Move, Items: []Item{{From: "/d/a", To: "/e/a"}, {From: "/d/b", To: "/e/b"}}, Lost: 1}, "the move of 3 items to /e"},
		{Entry{Kind: MkDir, Items: []Item{{From: "/d/n", To: "/d/n/m"}}}, "the new folder n"},
		{Entry{Kind: Trash, Items: []Item{{From: "/d/a.txt", To: "/T/a.txt"}}}, "the move of a.txt to the " + TrashWord()},
		{Entry{Kind: Trash, Items: []Item{{From: "/d/a.txt", To: "/T/a.txt"}}, Unknown: 2}, "the move of 3 items to the " + TrashWord()},
		{Entry{Kind: Trash, Unknown: 1}, "the move of 1 item to the " + TrashWord()},
	} {
		if got := Describe(c.e); got != filepath.FromSlash(c.want) && got != c.want {
			t.Errorf("%v: %q", c.e.Kind, got)
		}
	}
}

// TestUndoTrash: a delete to the trash comes back from the item's place in the trash, never over a new item at the old
// place; items whose place was not found are named in the problems. restoreTrashed is a plain rename here, so the
// real trash is never involved (internal/trash tests Restore itself).
func TestUndoTrash(t *testing.T) {
	old := restoreTrashed
	restoreTrashed = func(trashed, original string) error {
		os.MkdirAll(filepath.Dir(original), 0o755)
		return os.Rename(trashed, original)
	}
	t.Cleanup(func() { restoreTrashed = old })
	dir := t.TempDir()
	a, b := filepath.Join(dir, "work", "a.txt"), filepath.Join(dir, "work", "b.txt")
	ta, tb := filepath.Join(dir, "Trash", "a.txt"), filepath.Join(dir, "Trash", "b 2.txt")
	write(t, ta, "a")
	write(t, tb, "b")
	write(t, b, "a new b")
	e := Entry{Kind: Trash, Items: []Item{{From: a, To: ta}, {From: b, To: tb}}, Unknown: 2}
	p := PlanUndo(e, same)
	if len(p.Steps) != 1 || p.Steps[0].Op != OpRestore || p.Steps[0].From != a || len(p.Problems) != 2 {
		t.Fatalf("plan: %+v", p)
	}
	if !strings.Contains(p.Problems[0], "2 items went to the") || !strings.Contains(p.Problems[1], "is there again") {
		t.Fatalf("problems: %q", p.Problems)
	}
	jobs, done, probs := Apply(p.Steps)
	if done != 1 || len(jobs) != 0 || len(probs) != 0 || !exists(a) || exists(ta) {
		t.Fatalf("apply: %d %v %v", done, jobs, probs)
	}
	if s, _ := os.ReadFile(b); string(s) != "a new b" {
		t.Error("the new b was overwritten")
	}
	// gone from the trash since (emptied, or put back by hand)
	if p := PlanUndo(Entry{Kind: Trash, Items: []Item{{From: filepath.Join(dir, "c"), To: filepath.Join(dir, "Trash", "c")}}}, same); len(p.Steps) != 0 || !strings.Contains(p.Problems[0], "not in the") {
		t.Fatalf("gone: %+v", p)
	}
	// a failed restore is a problem, not a step done
	restoreTrashed = func(string, string) error { return errors.New("something else is at its old place now") }
	write(t, ta, "a again")
	os.Remove(a)
	if _, done, probs := Apply([]Step{{Op: OpRestore, From: a, To: ta}}); done != 0 || len(probs) != 1 || !strings.Contains(probs[0], "old place") {
		t.Fatalf("failed restore: %d %v", done, probs)
	}
	// an entry of unknown items alone is recorded and described
	j := Open("", 0)
	if _, ok := j.Add(Entry{Kind: Trash, Unknown: 1}); !ok {
		t.Error("an entry of unknown items was not recorded")
	}
}
