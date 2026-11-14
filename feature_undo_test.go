package main

// Tests of the undo journal as the app feeds it (board issue #13): renames, new folders and real copy and move jobs
// of internal/fileops in temporary folders land in the journal with what really arrived where, and Undo.Run puts them
// back or hands back the jobs that finish the undo. No job of an undo is run here, so the real trash is never used.

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"mormicommander/internal/fileops"
	"mormicommander/internal/undo"
)

// newUndoJobs makes a Jobs with its queue wired to a fresh in-memory undo journal, as the app does.
func newUndoJobs(t *testing.T) (*Jobs, *Undo) {
	t.Helper()
	a := NewApp(startOpts{})
	a.settings = loadSettings(filepath.Join(t.TempDir(), "settings.json"))
	u := newUndo(a, undo.Open("", 0))
	a.undo = u
	j := &Jobs{app: a, q: fileops.NewQueue(nil)}
	j.q.Log = func(e fileops.Entry) { a.undo.observe(e) }
	a.jobs = j
	return j, u
}

// runAndWait starts a job and waits until it has ended and its journal entry (if any) is written.
func runAndWait(t *testing.T, j *Jobs, u *Undo, r fileops.Request) fileops.Progress {
	t.Helper()
	id, err := j.Start(r)
	if err != nil {
		t.Fatal(err)
	}
	p := j.q.Wait(id)
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		u.mu.Lock()
		_, busy := u.jobs[id]
		u.mu.Unlock()
		if !busy {
			break
		}
	}
	return p
}

func mkfile(t *testing.T, p, s string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestUndoRecordsRenameAndMkDir(t *testing.T) {
	j, u := newUndoJobs(t)
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	mkfile(t, a, "x")
	b, err := j.Rename(a, "b.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := j.MkDir(dir, "n/m"); err != nil {
		t.Fatal(err)
	}
	l := u.List()
	if len(l) != 2 || l[0].Kind != undo.MkDir || l[1].Kind != undo.Rename {
		t.Fatalf("list: %+v", l)
	}
	e, _ := u.j.Last()
	if e.Items[0].From != filepath.Join(dir, "n") || e.Items[0].To != filepath.Join(dir, "n", "m") {
		t.Fatalf("mkdir entry: %+v", e)
	}
	// newest first: the folders go, then the old name comes back
	pv, err := u.Preview(0)
	if err != nil || pv.ID != e.ID || pv.Steps != 1 || pv.Count != 2 {
		t.Fatalf("preview: %+v %v", pv, err)
	}
	if r, err := u.Run(0); err != nil || r.Done != 1 || len(r.Jobs) != 0 {
		t.Fatalf("run: %+v %v", r, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "n")); err == nil {
		t.Error("the new folders are still there")
	}
	if r, err := u.Run(0); err != nil || r.Done != 1 {
		t.Fatalf("run 2: %+v %v", r, err)
	}
	if _, err := os.Stat(a); err != nil {
		t.Error("the old name did not come back")
	}
	if _, err := os.Stat(b); err == nil {
		t.Error("the new name is still there")
	}
	if pv, _ := u.Preview(0); pv.ID != 0 {
		t.Errorf("journal not empty: %+v", pv)
	}
	if _, err := u.Run(0); err == nil {
		t.Error("an empty journal undid something")
	}
}

func TestUndoRecordsMoveJob(t *testing.T) {
	j, u := newUndoJobs(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	mkfile(t, filepath.Join(src, "f.txt"), "1")
	mkfile(t, filepath.Join(src, "folder", "g.txt"), "2")
	mkfile(t, filepath.Join(dst, "g.txt"), "old")
	mkfile(t, filepath.Join(src, "g.txt"), "new")
	runAndWait(t, j, u, fileops.Request{Kind: fileops.Move, Target: dst, Overwrite: fileops.Overwrite,
		Sources: []string{filepath.Join(src, "f.txt"), filepath.Join(src, "folder"), filepath.Join(src, "g.txt")}})
	e, ok := u.j.Last()
	if !ok || e.Kind != undo.Move || len(e.Items) != 2 || e.Lost != 1 {
		t.Fatalf("entry: %+v", e)
	}
	r, err := u.Run(0)
	if err != nil || r.Done != 2 || len(r.Problems) != 1 {
		t.Fatalf("run: %+v %v", r, err)
	}
	for _, p := range []string{filepath.Join(src, "f.txt"), filepath.Join(src, "folder", "g.txt")} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s did not come back", p)
		}
	}
	if s, _ := os.ReadFile(filepath.Join(dst, "g.txt")); string(s) != "new" {
		t.Error("the replaced file was touched by the undo")
	}
}

func TestUndoRecordsCopyJob(t *testing.T) {
	j, u := newUndoJobs(t)
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	mkfile(t, a, "abc")
	// Shift+F5 into the same folder: the copy gets a free name, and the journal has that name
	runAndWait(t, j, u, fileops.Request{Kind: fileops.Copy, Target: dir, Overwrite: fileops.AutoRenameSource, Sources: []string{a}})
	e, ok := u.j.Last()
	if !ok || e.Kind != undo.Copy || len(e.Items) != 1 || e.Items[0].To == a || e.Items[0].Size != 3 {
		t.Fatalf("entry: %+v", e)
	}
	pv, _ := u.Preview(0)
	if pv.Steps != 1 {
		t.Fatalf("preview: %+v", pv)
	}
	r, err := u.Run(0)
	if err != nil || len(r.Jobs) != 1 || !r.Jobs[0].Request.NoJournal || r.Jobs[0].Request.Sources[0] != e.Items[0].To {
		t.Fatalf("run: %+v %v", r, err)
	}
	if k := r.Jobs[0].Request.Kind; k != fileops.Trash && k != fileops.Delete {
		t.Errorf("the copies are removed by a %s job", k)
	}
	if len(u.j.List()) != 0 {
		t.Error("the entry stayed in the journal")
	}
}

func TestUndoJournalSettingAndNoJournal(t *testing.T) {
	j, u := newUndoJobs(t)
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	mkfile(t, a, "x")
	// the undo's own jobs are not recorded
	runAndWait(t, j, u, fileops.Request{Kind: fileops.Copy, Target: filepath.Join(dir, "c"), Sources: []string{a}, NoJournal: true})
	if len(u.j.List()) != 0 {
		t.Fatal("a NoJournal job was recorded")
	}
	// the setting off: nothing is recorded
	j.app.settings.put("undoJournal", false)
	if _, err := j.Rename(a, "b.txt"); err != nil {
		t.Fatal(err)
	}
	runAndWait(t, j, u, fileops.Request{Kind: fileops.Copy, Target: filepath.Join(dir, "d"), Sources: []string{filepath.Join(dir, "b.txt")}})
	if len(u.j.List()) != 0 {
		t.Fatal("recorded with undoJournal off")
	}
}

func TestUndoJobsForCrossVolumeMove(t *testing.T) {
	jobs := undoJobs([]undo.Step{
		{Op: undo.OpMove, From: filepath.FromSlash("/a/x.txt"), To: filepath.FromSlash("/b/x.txt")},
		{Op: undo.OpMove, From: filepath.FromSlash("/a/y.txt"), To: filepath.FromSlash("/b/z.txt")},
	})
	if len(jobs) != 2 || jobs[0].Request.Rename != "" || jobs[1].Request.Rename != "y.txt" || jobs[0].Request.Overwrite != fileops.Skip ||
		jobs[0].Request.Target != filepath.FromSlash("/a") || !jobs[1].Request.NoJournal {
		t.Fatalf("%+v", jobs)
	}
}
