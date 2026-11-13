// Tests for lazy file access, reload detection, follow-tail polling and
// next/previous file navigation. All files live in t.TempDir().

package lister

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFileReadAtCache(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.bin")
	data := make([]byte, 5*blockSize+123)
	for i := range data {
		data[i] = byte(i * 7)
	}
	os.WriteFile(path, data, 0o644)
	f, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if f.Size() != int64(len(data)) || f.Path() != path || f.Info() == nil {
		t.Fatal("size/path/info")
	}
	for _, c := range []struct{ off, n int }{{0, 10}, {blockSize - 5, 10}, {3*blockSize + 1, 2 * blockSize}, {len(data) - 3, 3}} {
		buf := make([]byte, c.n)
		n, err := f.ReadAt(buf, int64(c.off))
		if err != nil || n != c.n || !bytes.Equal(buf, data[c.off:c.off+c.n]) {
			t.Fatalf("ReadAt(%d,%d): %d %v", c.off, c.n, n, err)
		}
	}
	buf := make([]byte, 10)
	if n, err := f.ReadAt(buf, int64(len(data)-4)); n != 4 || err != io.EOF {
		t.Fatalf("short read at end: %d %v", n, err)
	}
	if _, err := f.ReadAt(buf, -1); err == nil {
		t.Fatal("negative offset")
	}
	// Many distinct blocks: the cache must stay bounded.
	big := make([]byte, (maxBlocks+10)*blockSize)
	os.WriteFile(path, big, 0o644)
	f2, _ := Open(path)
	defer f2.Close()
	for i := 0; i < maxBlocks+10; i++ {
		f2.ReadAt(buf, int64(i)*blockSize)
	}
	if f2.lru.Len() > maxBlocks {
		t.Fatalf("cache grew to %d", f2.lru.Len())
	}
	f.Close()
	if _, err := f.ReadAt(buf, 0); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("read after close: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatal("double close")
	}
}

func TestOpenErrors(t *testing.T) {
	dir := t.TempDir()
	if _, err := Open(dir); err == nil {
		t.Fatal("opened a folder")
	}
	if _, err := Open(filepath.Join(dir, "nope")); err == nil {
		t.Fatal("opened a missing file")
	}
	if _, _, err := OpenViewer(filepath.Join(dir, "nope"), Options{}); err == nil {
		t.Fatal("OpenViewer missing")
	}
}

func TestReloadKinds(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	os.WriteFile(path, []byte("one\n"), 0o644)
	f, _ := Open(path)
	defer f.Close()
	buf := make([]byte, 4)
	f.ReadAt(buf, 0) // fill the cache

	ch, err := f.Reload()
	if err != nil || ch.Changed() {
		t.Fatalf("no change expected: %+v %v", ch, err)
	}
	af, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	af.WriteString("two\n")
	af.Close()
	ch, _ = f.Reload()
	if !ch.Grown || ch.OldSize != 4 || ch.NewSize != 8 {
		t.Fatalf("growth %+v", ch)
	}
	got := make([]byte, 8)
	if n, _ := f.ReadAt(got, 0); n != 8 || string(got) != "one\ntwo\n" {
		t.Fatalf("after growth %q", got)
	}
	os.Truncate(path, 2)
	ch, _ = f.Reload()
	if !ch.Truncated || ch.NewSize != 2 {
		t.Fatalf("truncation %+v", ch)
	}
	// Replace the file via rename, as log rotation and editors do.
	tmp := filepath.Join(dir, "new.txt")
	os.WriteFile(tmp, []byte("brand new content\n"), 0o644)
	if err := os.Rename(tmp, path); err != nil {
		t.Fatal(err)
	}
	ch, err = f.Reload()
	if err != nil || !ch.Replaced || ch.NewSize != 18 {
		t.Fatalf("replace %+v %v", ch, err)
	}
	all := make([]byte, 18)
	if n, _ := f.ReadAt(all, 0); n != 18 || !strings.HasPrefix(string(all), "brand") {
		t.Fatalf("after replace %q", all)
	}
	// Same size, new modification time.
	os.WriteFile(path, []byte("BRAND NEW CONTENT\n"), 0o644)
	future := time.Now().Add(time.Hour)
	os.Chtimes(path, future, future)
	ch, _ = f.Reload()
	if !ch.Modified && !ch.Replaced {
		t.Fatalf("modification not seen %+v", ch)
	}
	f.ReadAt(all, 0)
	if !strings.HasPrefix(string(all), "BRAND") {
		t.Fatalf("stale cache %q", all)
	}
	os.Remove(path)
	if _, err := f.Reload(); err == nil {
		t.Fatal("reload of deleted file should fail")
	}
}

func TestViewerReloadNonFile(t *testing.T) {
	v := mustViewer(t, []byte("abc"), Options{})
	ch, err := v.Reload()
	if err != nil || ch.Changed() || ch.NewSize != 3 {
		t.Fatalf("%+v %v", ch, err)
	}
	if v.Source().Size() != 3 {
		t.Fatal("Source")
	}
}

func TestFollowTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	os.WriteFile(path, []byte("start\n"), 0o644)
	v, f, err := OpenViewer(path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var mu sync.Mutex
	var changes []Change
	var lastRows []string
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- v.FollowTail(ctx, 10*time.Millisecond, func(ch Change) {
			top, _ := v.LastPage(2)
			rows, _ := v.Rows(top, 2)
			mu.Lock()
			changes = append(changes, ch)
			lastRows = rowTexts(rows)
			mu.Unlock()
		}, nil)
	}()
	waitFor := func(cond func() bool) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			mu.Lock()
			ok := cond()
			mu.Unlock()
			if ok {
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		t.Fatalf("timeout; changes %+v rows %q", changes, lastRows)
	}
	af, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	af.WriteString("appended line\n")
	af.Close()
	waitFor(func() bool { return len(lastRows) == 2 && lastRows[1] == "appended line" })

	os.WriteFile(path, []byte("rotated\n"), 0o644)
	waitFor(func() bool {
		for _, c := range changes {
			if c.Truncated {
				return len(lastRows) == 1 && lastRows[0] == "rotated"
			}
		}
		return false
	})
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("FollowTail returned %v", err)
	}
}

func TestFollowTailReportsErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gone.log")
	os.WriteFile(path, []byte("x"), 0o644)
	v, f, _ := OpenViewer(path, Options{})
	defer f.Close()
	os.Remove(path)
	errs := make(chan error, 10)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go v.FollowTail(ctx, 5*time.Millisecond, nil, func(err error) {
		select {
		case errs <- err:
		default:
		}
	})
	select {
	case err := <-errs:
		if err == nil {
			t.Fatal("nil error")
		}
	case <-ctx.Done():
		t.Fatal("no error reported for a deleted file")
	}
}

func TestNeighbors(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"file10.txt", "file2.txt", "File1.txt", ".hidden", "b.txt"} {
		os.WriteFile(filepath.Join(dir, n), nil, 0o644)
	}
	os.Mkdir(filepath.Join(dir, "sub"), 0o755)
	next, err := NeighborFile(filepath.Join(dir, "File1.txt"), 1)
	if err != nil || filepath.Base(next) != "file2.txt" {
		t.Fatalf("next %q %v", next, err)
	}
	next, _ = NeighborFile(filepath.Join(dir, "file2.txt"), 1)
	if filepath.Base(next) != "file10.txt" {
		t.Fatalf("natural order: %q", next)
	}
	prev, _ := NeighborFile(filepath.Join(dir, "File1.txt"), -1)
	if filepath.Base(prev) != "b.txt" {
		t.Fatalf("prev %q", prev)
	}
	if _, err := NeighborFile(filepath.Join(dir, "file10.txt"), 1); !errors.Is(err, ErrNoNeighbor) {
		t.Fatalf("end: %v", err)
	}
	if _, err := NeighborFile(filepath.Join(dir, "missing", "x"), 1); err == nil {
		t.Fatal("missing folder")
	}
	list := []string{"a", "b", "c"}
	if n, _ := NeighborIn(list, "b", -1); n != "a" {
		t.Fatal("NeighborIn")
	}
	if _, err := NeighborIn(list, "zz", 1); !errors.Is(err, ErrNoNeighbor) {
		t.Fatal("NeighborIn unknown")
	}
}

func TestNaturalLess(t *testing.T) {
	in := []string{"img12.png", "img2.png", "Img1.png", "img02b.png", "a", "A10", "a9"}
	sort.Slice(in, func(i, j int) bool { return NaturalLess(in[i], in[j]) })
	want := "a,a9,A10,Img1.png,img2.png,img02b.png,img12.png"
	if got := strings.Join(in, ","); got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}
