// Regression tests on real Unix folders for two ways a comparison could hang: a named pipe (FIFO) compared by
// content, whose open call blocks forever when nobody writes to it, and a recursive comparison through symbolic links
// that point back up the tree ("up -> .."), which without loop detection grows exponentially.

//go:build unix

package dircompare

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestFIFOByContentDoesNotHang(t *testing.T) {
	l, r := t.TempDir(), t.TempDir()
	for _, d := range []string{l, r} {
		if err := syscall.Mkfifo(filepath.Join(d, "p"), 0o644); err != nil {
			t.Skipf("mkfifo: %v", err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	var res *Result
	go func() {
		var err error
		res, err = Compare(ctx, OS{}, l, OS{}, r, Options{ByContent: true, TimeTolerance: time.Hour})
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Compare hung on a FIFO pair")
	}
	if len(res.Items) != 1 || !res.Items[0].Left.Special || res.Items[0].Status != Equal {
		t.Errorf("FIFO pair: %+v", res.Items)
	}
}

func TestOSOpenRefusesFIFO(t *testing.T) {
	p := filepath.Join(t.TempDir(), "p")
	if err := syscall.Mkfifo(p, 0o644); err != nil {
		t.Skipf("mkfifo: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		f, err := OS{}.Open(context.Background(), p)
		if f != nil {
			f.Close()
		}
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, ErrNotRegular) {
			t.Errorf("want ErrNotRegular, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("OS.Open blocked on a FIFO")
	}
}

func TestRecursiveSymlinkLoop(t *testing.T) {
	l, r := t.TempDir(), t.TempDir()
	for _, d := range []string{l, r} {
		if err := os.MkdirAll(filepath.Join(d, "a"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "a", "f"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		for _, n := range []string{"up1", "up2"} {
			if err := os.Symlink("..", filepath.Join(d, "a", n)); err != nil {
				t.Fatal(err)
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := time.Now()
	res, err := Compare(ctx, OS{}, l, OS{}, r, Options{Subdirs: SubdirsRecursive, TimeTolerance: time.Hour})
	if err != nil {
		t.Fatalf("Compare: %v (after %v)", err, time.Since(start))
	}
	all := res.Flatten()
	if len(all) > 10 {
		t.Errorf("walked %d items; the loop was not cut", len(all))
	}
	loops := 0
	for _, it := range all {
		if it.Status == Error && errors.Is(it.Err, ErrSymlinkLoop) {
			loops++
		}
	}
	if loops != 2 {
		t.Errorf("want the two up-links reported as loops, got %d; items %+v", loops, all)
	}
}
