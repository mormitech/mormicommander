// Regression tests for named pipes (FIFOs) on Unix systems. Opening a pipe that has
// no writer blocks inside the open call, so a content search or a duplicate search by
// content that tried to read one would hang forever, and Stop could not end it. The
// search must skip such entries without opening them.

//go:build unix

package find

import (
	"context"
	"os"
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

func TestContentSearchSkipsFIFO(t *testing.T) {
	root := mkTree(t, map[string]string{"a.txt": "a needle here", "b.txt": "nothing"})
	mkfifo(t, filepath.Join(root, "p"))
	if err := os.Symlink("p", filepath.Join(root, "plink")); err != nil {
		t.Fatal(err)
	}
	for _, notContaining := range []bool{false, true} {
		o := DefaultOptions()
		o.SearchIn = []string{root}
		o.Content.Text = "needle"
		o.Content.NotContaining = notContaining
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		j, err := Start(ctx, o)
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		var got []string
		go func() {
			for r := range j.Results() {
				got = append(got, r.Rel)
			}
		}()
		select {
		case <-j.Done():
		case <-time.After(5 * time.Second):
			j.Stop()
			cancel()
			t.Fatal("content search hung on a FIFO")
		}
		st, err := j.Wait()
		cancel()
		if err != nil {
			t.Fatalf("search error: %v", err)
		}
		want := "a.txt"
		if notContaining {
			want = "b.txt"
		}
		if st.Matches != 1 {
			t.Errorf("notContaining=%v: %d matches, want only %s", notContaining, st.Matches, want)
		}
	}
}

func TestDuplicateSearchSkipsFIFO(t *testing.T) {
	root := mkTree(t, map[string]string{"x/a": "", "y/a": ""})
	mkfifo(t, filepath.Join(root, "x", "p"))
	mkfifo(t, filepath.Join(root, "y", "p"))
	o := DefaultOptions()
	o.SearchIn = []string{root}
	o.Duplicates = DupOptions{Enabled: true, Size: true, Content: true}
	done := make(chan []Result, 1)
	go func() {
		res, _, err := Search(context.Background(), o)
		if err != nil {
			t.Error(err)
		}
		done <- res
	}()
	select {
	case res := <-done:
		for _, r := range res {
			if r.Name == "p" {
				t.Errorf("FIFO listed as a duplicate: %s", r.Rel)
			}
		}
	case <-time.After(5 * time.Second):
		t.Fatal("duplicate search hung on a FIFO")
	}
}

func TestOSFSOpenRefusesFIFO(t *testing.T) {
	p := filepath.Join(t.TempDir(), "p")
	mkfifo(t, p)
	done := make(chan error, 1)
	go func() {
		rc, err := OSFS{}.Open(p)
		if rc != nil {
			rc.Close()
		}
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Error("OSFS.Open opened a FIFO")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("OSFS.Open blocked on a FIFO")
	}
}
