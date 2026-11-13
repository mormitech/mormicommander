// Regression test: media columns (img.*, audio.*) must not open named pipes.
// Opening a FIFO that has no writer blocks forever, which would freeze the
// panel in any folder that holds one, so the media readers skip every item
// that is not a regular file.

//go:build unix

package listing

import (
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestMediaFieldsSkipFIFO(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"pipe.jpg", "pipe.mp3"} {
		if err := syscall.Mkfifo(filepath.Join(dir, n), 0o644); err != nil {
			t.Skipf("mkfifo: %v", err)
		}
	}
	items, err := ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	set := ColumnSet{Columns: []Column{
		{Template: "[=img.width]"}, {Template: "[=audio.title]"},
	}}.MustCompile()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range items {
			set.Render(&items[i], &Format{})
		}
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("rendering media fields of a FIFO hung")
	}

	// Mode zero-value is regular, but the open-time check must still reject a
	// FIFO when the item's Mode was wrong or stale.
	stale := Item{Name: "pipe.jpg", Dir: dir}
	done2 := make(chan struct{})
	go func() {
		defer close(done2)
		if got := set.Columns[0].Render(&stale, &Format{}); got != "" {
			t.Errorf("FIFO with stale mode rendered %q", got)
		}
	}()
	select {
	case <-done2:
	case <-time.After(2 * time.Second):
		t.Fatal("open of a FIFO with a stale regular mode hung")
	}
}
