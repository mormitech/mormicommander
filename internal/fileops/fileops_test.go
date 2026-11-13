package fileops

// The basic tests of the engine: copying a tree with dates and links, copy with a new name, refusing to copy a folder
// into itself, the classic overwrite answers, the "older" rule, cancelling (also while a question is open), moves,
// deletes, the trash hook and the checks on a request.

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// recorder keeps the events a queue emits.
type recorder struct {
	mu     sync.Mutex
	events []string
	asks   chan Progress
}

func newRecorder() *recorder { return &recorder{asks: make(chan Progress, 16)} }

func (r *recorder) emit(ev string, data any) {
	r.mu.Lock()
	r.events = append(r.events, ev)
	r.mu.Unlock()
	if ev == EventAsk {
		r.asks <- data.(Progress)
	}
}

func (r *recorder) count(ev string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, e := range r.events {
		if e == ev {
			n++
		}
	}
	return n
}

func write(t *testing.T, p, s string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return string(b)
}

func exists(p string) bool { _, err := os.Lstat(p); return err == nil }

func start(t *testing.T, q *Queue, r Request) Progress {
	t.Helper()
	id, err := q.Start(r)
	if err != nil {
		t.Fatal(err)
	}
	return q.Wait(id)
}

func TestCopyTree(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	write(t, filepath.Join(src, "a.txt"), "hello")
	write(t, filepath.Join(src, "dir", "b.txt"), "world")
	write(t, filepath.Join(src, "dir", "deep", "c.txt"), "!")
	old := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	os.Chtimes(filepath.Join(src, "a.txt"), old, old)
	if runtime.GOOS != "windows" {
		os.Symlink("b.txt", filepath.Join(src, "dir", "link"))
		os.Symlink(".", filepath.Join(src, "dir", "loop")) // a loop: copied as a link, never followed
	}
	rec := newRecorder()
	q := NewQueue(rec.emit)
	p := start(t, q, Request{Kind: Copy, Sources: []string{filepath.Join(src, "a.txt"), filepath.Join(src, "dir")}, Target: dst})
	if p.State != StateDone || len(p.Errors) != 0 {
		t.Fatalf("state %s, errors %v", p.State, p.Errors)
	}
	if read(t, filepath.Join(dst, "a.txt")) != "hello" || read(t, filepath.Join(dst, "dir", "deep", "c.txt")) != "!" {
		t.Error("content differs")
	}
	if fi, _ := os.Stat(filepath.Join(dst, "a.txt")); !fi.ModTime().Equal(old) {
		t.Errorf("date not kept: %v", fi.ModTime())
	}
	if runtime.GOOS != "windows" {
		if l, err := os.Readlink(filepath.Join(dst, "dir", "link")); err != nil || l != "b.txt" {
			t.Errorf("link %q %v", l, err)
		}
	}
	if p.BytesDone != p.Bytes || p.Bytes != 11 || p.FilesDone != p.Files {
		t.Errorf("progress %+v", p)
	}
	if rec.count(EventDone) != 1 || rec.count(EventProgress) == 0 {
		t.Errorf("events %v", rec.events)
	}
	if !exists(filepath.Join(src, "a.txt")) {
		t.Error("copy removed the source")
	}
}

func TestCopyRename(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.txt"), "x")
	q := NewQueue(nil)
	p := start(t, q, Request{Kind: Copy, Sources: []string{filepath.Join(dir, "a.txt")}, Target: dir, Rename: "b.txt"})
	if p.State != StateDone || read(t, filepath.Join(dir, "b.txt")) != "x" {
		t.Fatalf("%+v", p)
	}
}

func TestCopyIntoItselfRefused(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "f", "x"), "x")
	q := NewQueue(nil)
	p := start(t, q, Request{Kind: Copy, Sources: []string{filepath.Join(dir, "f")}, Target: filepath.Join(dir, "f", "sub")})
	if p.State != StateFailed || !strings.Contains(p.Errors[0], "into itself") {
		t.Fatalf("%+v", p)
	}
	if _, err := os.Lstat(filepath.Join(dir, "f", "sub")); !os.IsNotExist(err) {
		t.Fatalf("the refused target was made: %v", err)
	}
	p = start(t, q, Request{Kind: Copy, Sources: []string{filepath.Join(dir, "f", "x")}, Target: filepath.Join(dir, "f")})
	if p.State != StateFailed || !strings.Contains(p.Errors[0], "onto itself") {
		t.Fatalf("%+v", p)
	}
	if read(t, filepath.Join(dir, "f", "x")) != "x" {
		t.Error("source damaged")
	}
}

func TestConflictAnswers(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	for _, n := range []string{"1", "2", "3", "4"} {
		write(t, filepath.Join(src, n), "new"+n)
		write(t, filepath.Join(dst, n), "old"+n)
	}
	rec := newRecorder()
	q := NewQueue(rec.emit)
	var srcs []string
	for _, n := range []string{"1", "2", "3", "4"} {
		srcs = append(srcs, filepath.Join(src, n))
	}
	id, err := q.Start(Request{Kind: Copy, Sources: srcs, Target: dst})
	if err != nil {
		t.Fatal(err)
	}
	answers := []Answer{{Choice: ChoiceOverwrite}, {Choice: ChoiceSkip}, {Choice: ChoiceRename, NewName: "3-copy"}, {Choice: ChoiceSkipAll}}
	for i, a := range answers {
		select {
		case ask := <-rec.asks:
			if ask.State != StateAsking || ask.Ask == nil || ask.Ask.DstSize != 4 {
				t.Fatalf("ask %d: %+v", i, ask)
			}
			if err := q.Answer(id, a); err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("no question %d", i)
		}
	}
	p := q.Wait(id)
	if p.State != StateDone {
		t.Fatalf("%+v", p)
	}
	want := map[string]string{"1": "new1", "2": "old2", "3": "old3", "3-copy": "new3", "4": "old4"}
	for n, s := range want {
		if got := read(t, filepath.Join(dst, n)); got != s {
			t.Errorf("%s = %q, want %q", n, got, s)
		}
	}
	if p.Skipped != 2 {
		t.Errorf("skipped %d", p.Skipped)
	}
	if err := q.Answer(id, Answer{Choice: ChoiceSkip}); err == nil {
		t.Error("answer to a finished job accepted")
	}
}

func TestPolicyOlder(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	write(t, filepath.Join(src, "newer"), "src")
	write(t, filepath.Join(src, "older"), "src")
	write(t, filepath.Join(dst, "newer"), "dst")
	write(t, filepath.Join(dst, "older"), "dst")
	past, future := time.Now().Add(-time.Hour), time.Now().Add(time.Hour)
	os.Chtimes(filepath.Join(dst, "newer"), past, past)     // target older than source: overwritten
	os.Chtimes(filepath.Join(dst, "older"), future, future) // target newer: kept
	q := NewQueue(nil)
	start(t, q, Request{Kind: Copy, Sources: []string{filepath.Join(src, "newer"), filepath.Join(src, "older")}, Target: dst, Overwrite: Older})
	if read(t, filepath.Join(dst, "newer")) != "src" || read(t, filepath.Join(dst, "older")) != "dst" {
		t.Error("older rule not followed")
	}
}

func TestCancel(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	for i := 0; i < 20; i++ {
		write(t, filepath.Join(src, "d", string(rune('a'+i))), "x")
	}
	q := NewQueue(nil)
	started := make(chan struct{})
	var once sync.Once
	release := make(chan struct{})
	q.beforeFile = func(string) {
		once.Do(func() { close(started) })
		<-release
	}
	id, _ := q.Start(Request{Kind: Copy, Sources: []string{filepath.Join(src, "d")}, Target: dst})
	<-started
	if !q.Cancel(id) {
		t.Fatal("cancel refused")
	}
	close(release)
	p := q.Wait(id)
	if p.State != StateCancelled {
		t.Fatalf("%+v", p)
	}
	if q.Cancel(id) {
		t.Error("cancel of a finished job accepted")
	}
	if es, _ := os.ReadDir(filepath.Join(dst, "d")); len(es) == 20 {
		t.Error("cancel did nothing")
	}
}

func TestCancelWhileAsking(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	write(t, filepath.Join(src, "f"), "new")
	write(t, filepath.Join(dst, "f"), "old")
	rec := newRecorder()
	q := NewQueue(rec.emit)
	id, _ := q.Start(Request{Kind: Copy, Sources: []string{filepath.Join(src, "f")}, Target: dst})
	<-rec.asks
	q.Cancel(id)
	if p := q.Wait(id); p.State != StateCancelled {
		t.Fatalf("%+v", p)
	}
	if read(t, filepath.Join(dst, "f")) != "old" {
		t.Error("overwritten after cancel")
	}
}

func TestMove(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	write(t, filepath.Join(src, "a.txt"), "a")
	write(t, filepath.Join(src, "d", "b.txt"), "b")
	q := NewQueue(nil)
	p := start(t, q, Request{Kind: Move, Sources: []string{filepath.Join(src, "a.txt"), filepath.Join(src, "d")}, Target: dst})
	if p.State != StateDone || p.FilesDone != 2 {
		t.Fatalf("%+v", p)
	}
	if exists(filepath.Join(src, "a.txt")) || exists(filepath.Join(src, "d")) {
		t.Error("source still there")
	}
	if read(t, filepath.Join(dst, "d", "b.txt")) != "b" {
		t.Error("moved content differs")
	}
}

func TestMoveMergeKeepsSkipped(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	write(t, filepath.Join(src, "d", "keep"), "src-keep")
	write(t, filepath.Join(src, "d", "go"), "src-go")
	write(t, filepath.Join(dst, "d", "keep"), "dst-keep") // the folder exists: no rename, a merge
	q := NewQueue(nil)
	p := start(t, q, Request{Kind: Move, Sources: []string{filepath.Join(src, "d")}, Target: dst, Overwrite: Skip})
	if p.State != StateDone {
		t.Fatalf("%+v", p)
	}
	if read(t, filepath.Join(dst, "d", "go")) != "src-go" || read(t, filepath.Join(dst, "d", "keep")) != "dst-keep" {
		t.Error("merge wrong")
	}
	if !exists(filepath.Join(src, "d", "keep")) || exists(filepath.Join(src, "d", "go")) {
		t.Error("move removed a skipped file, or kept a moved one")
	}
}

func TestDelete(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "x", "y", "z.txt"), "z")
	write(t, filepath.Join(dir, "f.txt"), "f")
	keep := filepath.Join(dir, "keep")
	write(t, filepath.Join(keep, "k"), "k")
	if runtime.GOOS != "windows" {
		os.Symlink(keep, filepath.Join(dir, "x", "link-to-keep")) // the link goes, what it points to stays
	}
	q := NewQueue(nil)
	p := start(t, q, Request{Kind: Delete, Sources: []string{filepath.Join(dir, "x"), filepath.Join(dir, "f.txt")}})
	if p.State != StateDone {
		t.Fatalf("%+v", p)
	}
	if exists(filepath.Join(dir, "x")) || exists(filepath.Join(dir, "f.txt")) {
		t.Error("not deleted")
	}
	if read(t, filepath.Join(keep, "k")) != "k" {
		t.Error("followed a link")
	}
}

func TestTrashUsesHook(t *testing.T) {
	var got []string
	orig := trashMove
	trashMove = func(p string) error { got = append(got, p); return nil }
	defer func() { trashMove = orig }()
	q := NewQueue(nil)
	p := start(t, q, Request{Kind: Trash, Sources: []string{"/nowhere/a", "/nowhere/b"}})
	if p.State != StateDone || len(got) != 2 || p.FilesDone != 2 {
		t.Fatalf("%+v %v", p, got)
	}
}

func TestValidate(t *testing.T) {
	q := NewQueue(nil)
	for _, r := range []Request{
		{Kind: Copy, Sources: []string{"/a"}},                                  // no target
		{Kind: Copy, Target: "/t"},                                             // nothing selected
		{Kind: "shred", Sources: []string{"/a"}},                               // unknown kind
		{Kind: Copy, Sources: []string{"/a", "/b"}, Target: "/t", Rename: "x"}, // rename for two
		{Kind: Copy, Sources: []string{"/a"}, Target: "/t", Overwrite: "maybe"},
	} {
		if _, err := q.Start(r); err == nil {
			t.Errorf("accepted %+v", r)
		}
	}
	if len(q.List()) != 0 {
		t.Error("bad request made a job")
	}
}
