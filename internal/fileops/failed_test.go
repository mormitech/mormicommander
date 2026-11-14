package fileops

// Tests of the transfer error list (board issue #15): with the skip-and-continue error policy a copy, move or delete
// runs on past files that fail, counts the errors without a cap (ErrorCount), and lists each failed path with its
// target and plain-English reason (Failed). SkipErrors switches a job that would ask Retry / Skip over to skipping,
// as the app does when a job goes to the background with "Skip errors in background" on.

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// unreadable makes the file at p unreadable for the rest of the test (and readable again afterwards).
func unreadable(t *testing.T, p string) {
	t.Helper()
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs an unreadable file")
	}
	if err := os.Chmod(p, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(p, 0o644) })
}

func TestCopySkipsFailedFilesAndListsThem(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	dir := filepath.Join(src, "d")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := range 100 {
		write(t, filepath.Join(dir, fmt.Sprintf("f%03d", i)), "x")
	}
	unreadable(t, filepath.Join(dir, "f010"))
	unreadable(t, filepath.Join(dir, "f050"))
	q := NewQueue(nil)
	asks := 0
	q.AskFunc = func(Progress) Answer { asks++; return Answer{Choice: ChoiceCancel} }
	p := start(t, q, Request{Kind: Copy, Sources: []string{dir}, Target: dst, OnError: ErrorsSkip})
	if asks != 0 {
		t.Fatalf("asked %d questions", asks)
	}
	if p.State != StateFailed || p.FilesDone != 98 || p.ErrorCount != 2 || len(p.Failed) != 2 {
		t.Fatalf("%+v", p)
	}
	for _, f := range p.Failed {
		name := filepath.Base(f.Source)
		if name != "f010" && name != "f050" {
			t.Errorf("unexpected failure %+v", f)
		}
		if f.Target != filepath.Join(dst, "d", name) {
			t.Errorf("target %q for %s", f.Target, f.Source)
		}
		if f.Reason != "permission denied" || f.Message == "" {
			t.Errorf("reason %q, message %q", f.Reason, f.Message)
		}
	}
	if _, err := os.Stat(filepath.Join(dst, "d", "f099")); err != nil {
		t.Errorf("the rest was not copied: %v", err)
	}
}

func TestErrorCountIsNotCapped(t *testing.T) {
	q := NewQueue(nil)
	id, _ := q.Start(Request{Kind: Delete, Sources: []string{filepath.Join(t.TempDir(), "missing")}})
	q.Wait(id)
	j := q.get(id)
	for i := range maxErrors + 5 {
		j.failAt(fmt.Sprintf("/x/%d", i), "", os.ErrNotExist, "Can't delete %d.", i)
	}
	p := j.snapshot()
	if p.ErrorCount != maxErrors+6 || len(p.Errors) != maxErrors || len(p.Failed) != maxErrors+6 {
		t.Fatalf("count %d, errors %d, failed %d", p.ErrorCount, len(p.Errors), len(p.Failed))
	}
	if p.Failed[1].Reason != "it is not there any more" {
		t.Errorf("reason %q", p.Failed[1].Reason)
	}
}

func TestDeleteMissingIsListed(t *testing.T) {
	gone := filepath.Join(t.TempDir(), "gone")
	q := NewQueue(nil)
	p := start(t, q, Request{Kind: Delete, Sources: []string{gone}})
	if p.State != StateFailed || len(p.Failed) != 1 || p.Failed[0].Source != gone || p.Failed[0].Target != "" {
		t.Fatalf("%+v", p)
	}
}

func TestSkipErrorsStopsAsking(t *testing.T) {
	src := t.TempDir()
	var srcs []string
	for _, n := range []string{"a", "b", "c"} {
		f := filepath.Join(src, n)
		write(t, f, n)
		srcs = append(srcs, f)
	}
	unreadable(t, srcs[0])
	unreadable(t, srcs[2])
	q := NewQueue(nil)
	asks := 0
	q.AskFunc = func(p Progress) Answer {
		asks++
		q.SkipErrors(p.ID) // as when the job goes to the background while the first question is up
		return Answer{Choice: ChoiceSkip}
	}
	id, err := q.Start(Request{Kind: Copy, Sources: srcs, Target: t.TempDir(), OnError: ErrorsAsk})
	if err != nil {
		t.Fatal(err)
	}
	p := q.Wait(id)
	if asks != 1 || p.Skipped != 1 || p.ErrorCount != 1 || len(p.Failed) != 1 || filepath.Base(p.Failed[0].Source) != "c" || p.FilesDone != 1 {
		t.Fatalf("asks %d, %+v", asks, p)
	}
	if q.SkipErrors(id) || q.SkipErrors(9999) {
		t.Error("SkipErrors accepted for a finished or unknown job")
	}
}
