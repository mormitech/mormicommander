package dirsize

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// makeTree creates files (path → size) and empty folders (paths ending in "/") under root.
func makeTree(t *testing.T, root string, spec map[string]int) {
	t.Helper()
	for p, n := range spec {
		full := filepath.Join(root, filepath.FromSlash(p))
		if strings.HasSuffix(p, "/") {
			if err := os.MkdirAll(full, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, make([]byte, n), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCalculateCounts(t *testing.T) {
	tests := []struct {
		name               string
		spec               map[string]int
		files, dirs, bytes int64
	}{
		{"empty", map[string]int{}, 0, 0, 0},
		{"flat", map[string]int{"a": 10, "b": 20, "c": 0}, 3, 0, 30},
		{"nested", map[string]int{"a": 1, "x/b": 2, "x/y/c": 3, "x/y/z/d": 4, "e/": 0}, 4, 4, 10},
		{"wide", func() map[string]int {
			m := map[string]int{}
			for i := 0; i < 40; i++ {
				for j := 0; j < 5; j++ {
					m[fmt.Sprintf("d%02d/s%d/f", i, j)] = i + j
				}
			}
			return m
		}(), 200, 240, func() int64 {
			var s int64
			for i := 0; i < 40; i++ {
				for j := 0; j < 5; j++ {
					s += int64(i + j)
				}
			}
			return s
		}()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			makeTree(t, root, tc.spec)
			for _, workers := range []int{1, 4, 32} {
				r, err := Calculate(context.Background(), root, Options{Workers: workers})
				if err != nil {
					t.Fatal(err)
				}
				if r.Files != tc.files || r.Dirs != tc.dirs || r.Bytes != tc.bytes || !r.IsDir || !r.Complete || r.ErrorCount != 0 {
					t.Errorf("workers=%d: got %v (complete=%v errors=%d), want %d files, %d dirs, %d bytes",
						workers, r, r.Complete, r.ErrorCount, tc.files, tc.dirs, tc.bytes)
				}
				if r.Allocated < 0 {
					t.Errorf("negative allocated %d", r.Allocated)
				}
			}
		})
	}
}

func TestCalculateFileRootAndMissing(t *testing.T) {
	root := t.TempDir()
	f := filepath.Join(root, "f.bin")
	makeTree(t, root, map[string]int{"f.bin": 5000})
	r, err := Calculate(context.Background(), f, Options{})
	if err != nil || r.IsDir || r.Files != 1 || r.Bytes != 5000 || r.Dirs != 0 {
		t.Fatalf("file root: %v, %v", r, err)
	}
	if runtime.GOOS != "windows" && r.Allocated < 4096 {
		t.Errorf("5000-byte file allocated %d, want at least one block", r.Allocated)
	}

	missing := filepath.Join(root, "nope")
	r, err = Calculate(context.Background(), missing, Options{})
	if err == nil || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing path: err = %v", err)
	}
	if r.ErrorCount != 1 || len(r.Errors) != 1 || r.Errors[0].Path != missing {
		t.Errorf("missing path errors: %+v", r)
	}
}

func TestHardlinks(t *testing.T) {
	root := t.TempDir()
	makeTree(t, root, map[string]int{"a/data": 10000, "b/other": 7})
	if err := os.Link(filepath.Join(root, "a/data"), filepath.Join(root, "a/data2")); err != nil {
		t.Skipf("hard links not supported: %v", err)
	}
	if err := os.Link(filepath.Join(root, "a/data"), filepath.Join(root, "b/data3")); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		opts  Options
		bytes int64
		dupes int64
	}{
		{"once", Options{}, 10007, 2},
		{"twice", Options{CountHardlinksTwice: true}, 30007, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, err := Calculate(context.Background(), root, tc.opts)
			if err != nil {
				t.Fatal(err)
			}
			if r.Files != 4 || r.Bytes != tc.bytes || r.HardlinkDupes != tc.dupes {
				t.Errorf("got files=%d bytes=%d dupes=%d, want 4/%d/%d", r.Files, r.Bytes, r.HardlinkDupes, tc.bytes, tc.dupes)
			}
			if tc.dupes > 0 && r.HardlinkBytes != 20000 {
				t.Errorf("HardlinkBytes = %d, want 20000", r.HardlinkBytes)
			}
		})
	}

	// Across selected folders: a/ and b/ each contain a name of the same data; the total counts it once.
	s, err := CalculateAll(context.Background(), []string{filepath.Join(root, "a"), filepath.Join(root, "b")}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if s.Items[0].Bytes != 10000 || s.Items[1].Bytes != 10007 {
		t.Errorf("items: %v / %v", s.Items[0], s.Items[1])
	}
	if s.Total.Bytes != 10007 || s.Total.Files != 4 || s.Total.Dirs != 2 || s.Total.HardlinkDupes != 2 {
		t.Errorf("total: %v dupes=%d", s.Total, s.Total.HardlinkDupes)
	}
}

func TestSymlinks(t *testing.T) {
	root := t.TempDir()
	makeTree(t, root, map[string]int{"real/f": 100, "real/sub/g": 50, "outside/h": 1000})
	if err := os.Symlink(filepath.Join(root, "outside"), filepath.Join(root, "real/link")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}
	must(t, os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "real/sub/loop")))         // loop to an ancestor
	must(t, os.Symlink(filepath.Join(root, "outside/h"), filepath.Join(root, "real/filelink")))    // link to a file
	must(t, os.Symlink(filepath.Join(root, "does-not-exist"), filepath.Join(root, "real/broken"))) // broken link

	r, err := Calculate(context.Background(), filepath.Join(root, "real"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Files != 2 || r.Bytes != 150 || r.Dirs != 1 || r.Symlinks != 4 {
		t.Errorf("not following: %v symlinks=%d", r, r.Symlinks)
	}

	r, err = Calculate(context.Background(), filepath.Join(root, "real"), Options{FollowSymlinks: true})
	if err != nil {
		t.Fatal(err)
	}
	// f, sub/g, outside/h through the dir link, and h again through the file link (same data, counted once).
	if r.Files != 4 || r.Bytes != 1150 || r.Dirs != 2 || r.HardlinkDupes != 1 {
		t.Errorf("following: %v symlinks=%d dupes=%d", r, r.Symlinks, r.HardlinkDupes)
	}
	if r.SkippedMountCount != 1 || r.Symlinks != 2 { // the loop and the broken link
		t.Errorf("following: skipped=%d %v symlinks=%d", r.SkippedMountCount, r.SkippedMounts, r.Symlinks)
	}

	// The root itself is followed even without FollowSymlinks.
	must(t, os.Symlink(filepath.Join(root, "outside"), filepath.Join(root, "rootlink")))
	r, err = Calculate(context.Background(), filepath.Join(root, "rootlink"), Options{})
	if err != nil || !r.IsDir || r.Bytes != 1000 {
		t.Errorf("root link: %v, %v", r, err)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestSparseSizeOnDisk(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("files are not sparse on Windows unless marked")
	}
	root := t.TempDir()
	p := filepath.Join(root, "sparse")
	f, err := os.Create(p)
	must(t, err)
	must(t, f.Truncate(64<<20))
	must(t, f.Close())
	r, err := Calculate(context.Background(), root, Options{})
	must(t, err)
	if r.Bytes != 64<<20 {
		t.Fatalf("bytes = %d", r.Bytes)
	}
	if r.Allocated >= r.Bytes {
		t.Skipf("file system does not keep the file sparse (allocated %d)", r.Allocated)
	}
	if SpaceLine(r) == "" || !strings.Contains(SpaceLine(r), "on disk") {
		t.Errorf("SpaceLine = %q, want an on-disk part", SpaceLine(r))
	}
}

func TestUnreadableFolder(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permission bits do not stop this user")
	}
	root := t.TempDir()
	makeTree(t, root, map[string]int{"ok/a": 3, "locked/b": 4})
	locked := filepath.Join(root, "locked")
	must(t, os.Chmod(locked, 0))
	t.Cleanup(func() { os.Chmod(locked, 0o755) })
	r, err := Calculate(context.Background(), root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if r.ErrorCount != 1 || len(r.Errors) != 1 || r.Errors[0].Path != locked || r.Bytes != 3 || r.Dirs != 2 {
		t.Errorf("got %v errors=%+v", r, r.Errors)
	}
}

func TestSameVolumeSkipsOtherDevices(t *testing.T) {
	root := t.TempDir()
	makeTree(t, root, map[string]int{"a": 1, "sub/b": 2, "sub2/c/d": 3})
	r, err := Calculate(context.Background(), root, Options{SameVolume: true})
	if err != nil || r.Bytes != 6 || r.SkippedMountCount != 0 {
		t.Fatalf("same volume on one volume: %v skipped=%d err=%v", r, r.SkippedMountCount, err)
	}
	if runtime.GOOS == "windows" {
		return
	}
	// Pretend the root is on another device: every subfolder is then a mount point and is not entered.
	w := newWalker(context.Background(), Options{SameVolume: true})
	w.env = newStatEnv(root, false)
	w.checkDev, w.rootDev = true, ^uint64(0)
	w.dir(root)
	w.wg.Wait()
	got := w.result(Result{})
	if got.Bytes != 1 || got.Dirs != 2 || got.SkippedMountCount != 2 || len(got.SkippedMounts) != 2 {
		t.Errorf("fake mount points: %v skipped=%v", got, got.SkippedMounts)
	}
}

func TestCancel(t *testing.T) {
	root := t.TempDir()
	makeTree(t, root, map[string]int{"a/x": 1, "b/y": 2, "c/z": 3})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r, err := Calculate(ctx, root, Options{})
	if !errors.Is(err, ErrCanceled) || !errors.Is(err, context.Canceled) || r.Complete {
		t.Fatalf("pre-cancelled: %v complete=%v", err, r.Complete)
	}

	// Cancel after the first item of Alt+Shift+Enter: the rest stays empty and incomplete.
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	var done []int
	paths := []string{filepath.Join(root, "a"), filepath.Join(root, "b"), filepath.Join(root, "c")}
	s, err := CalculateAll(ctx, paths, Options{ItemDone: func(i int, r Result) {
		done = append(done, i)
		cancel()
	}})
	if !errors.Is(err, ErrCanceled) {
		t.Fatalf("err = %v", err)
	}
	if len(done) != 1 || !s.Items[0].Complete || s.Items[0].Bytes != 1 || s.Items[1].Complete || s.Items[1].Path != paths[1] || s.Total.Complete {
		t.Errorf("done=%v items=%+v total complete=%v", done, s.Items, s.Total.Complete)
	}
}

func TestProgressAndItemDone(t *testing.T) {
	root := t.TempDir()
	spec := map[string]int{}
	for i := 0; i < 30; i++ {
		spec[fmt.Sprintf("d%d/e/f%d", i%3, i)] = 10
	}
	makeTree(t, root, spec)
	paths := []string{filepath.Join(root, "d0"), filepath.Join(root, "d1"), filepath.Join(root, "d2")}
	var mu sync.Mutex
	var calls []Progress
	var items []Result
	s, err := CalculateAll(context.Background(), paths, Options{
		ProgressInterval: time.Millisecond,
		Progress:         func(p Progress) { mu.Lock(); calls = append(calls, p); mu.Unlock() },
		ItemDone:         func(i int, r Result) { items = append(items, r) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 || items[2].Path != paths[2] {
		t.Fatalf("ItemDone results: %+v", items)
	}
	last := calls[len(calls)-1]
	if !last.Done || last.ItemsDone != 3 || last.ItemsTotal != 3 || last.Bytes != 300 || last.Files != 30 || last.Dirs != 6 {
		t.Errorf("final progress %+v", last)
	}
	if s.Total.Bytes != 300 || s.Total.Dirs != 6 || s.Total.Files != 30 {
		t.Errorf("total %v", s.Total)
	}
	for _, p := range calls[:len(calls)-1] {
		if p.Done {
			t.Errorf("Done set before the end: %+v", p)
		}
	}
}

func TestJob(t *testing.T) {
	root := t.TempDir()
	makeTree(t, root, map[string]int{"a/b/c": 42})
	j := Start(context.Background(), []string{root}, Options{})
	s, err := j.Wait()
	if err != nil || s.Items[0].Bytes != 42 {
		t.Fatalf("job: %+v, %v", s, err)
	}
	select {
	case <-j.Done():
	default:
		t.Error("Done not closed after Wait")
	}
	if p := j.Progress(); !p.Done || p.Bytes != 42 {
		t.Errorf("job progress %+v", p)
	}
	j.Cancel() // harmless after the end

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	j = Start(ctx, []string{root}, Options{})
	if _, err := j.Wait(); !errors.Is(err, ErrCanceled) {
		t.Errorf("cancelled job err = %v", err)
	}
}
