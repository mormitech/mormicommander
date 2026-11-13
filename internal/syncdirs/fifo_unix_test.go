// Regression tests for named pipes (FIFOs) on Unix systems. Opening a pipe
// without a writer blocks inside the open call, out of reach of the context,
// so a compare by content or a copy that tried to read one would never
// return. Pipes must be listed but never opened or copied.

//go:build unix

package syncdirs

import (
	"context"
	"errors"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func mkfifo(t *testing.T, p string) {
	t.Helper()
	if err := syscall.Mkfifo(p, 0o644); err != nil {
		t.Skipf("mkfifo: %v", err)
	}
}

// within fails the test when f does not return in time.
func within(t *testing.T, what string, f func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { f(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("%s hung on a FIFO", what)
	}
}

func TestFIFOPairByContent(t *testing.T) {
	l, r := trees(t, map[string]spec{}, map[string]spec{})
	mkfifo(t, filepath.Join(l, "p"))
	mkfifo(t, filepath.Join(r, "p"))
	o := DefaultOptions()
	o.ByContent = true
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var res *Result
	var err error
	within(t, "Compare", func() { res, err = Compare(ctx, DirFS(l), DirFS(r), o, nil) })
	if err != nil {
		t.Fatal(err)
	}
	it := res.Find("p")
	if it == nil || it.Left.Kind != KindSpecial || it.Action != NoAction || it.Note == "" {
		t.Fatalf("FIFO pair: %+v", it)
	}
	if it.CanApply(CopyLeftToRight) || it.CanApply(CopyRightToLeft) {
		t.Error("a FIFO must not be copyable")
	}
	if !it.CanApply(DeleteRight) {
		t.Error("a FIFO must still be deletable")
	}
}

func TestLeftOnlyFIFOIsNotCopied(t *testing.T) {
	l, r := trees(t, map[string]spec{"a.txt": file("a", 0)}, map[string]spec{})
	mkfifo(t, filepath.Join(l, "p"))
	res := mustCompare(t, l, r, DefaultOptions())
	if it := res.Find("p"); it == nil || it.Action != NoAction || it.Category != LeftOnly {
		t.Fatalf("left-only FIFO: %+v", it)
	}
	var rep *Report
	within(t, "Execute", func() { rep = syncAll(t, res, l, r, AllOps(), ExecOptions{}) })
	if rep.Done != 1 || len(rep.Errors) != 0 {
		t.Errorf("report %+v", rep)
	}
	// Even a forced copy op on a FIFO is refused without opening it.
	op := Op{Kind: OpCopyFile, Side: RightSide, Src: "p", Dst: "p", Item: res.Find("p")}
	p := &Plan{Ops: []Op{op}, NeedsWriteRight: true}
	within(t, "forced Execute", func() {
		rep, _ = Execute(context.Background(), p, DirFS(l), DirFS(r), ExecOptions{Force: true})
	})
	if len(rep.Errors) != 1 || !errors.Is(rep.Errors[0].Err, ErrNotRegular) {
		t.Errorf("forced FIFO copy: %+v", rep)
	}
	within(t, "OSFS.Open", func() {
		if _, err := DirFS(l).Open("p"); !errors.Is(err, ErrNotRegular) {
			t.Errorf("OSFS.Open on a FIFO: %v", err)
		}
	})
}
