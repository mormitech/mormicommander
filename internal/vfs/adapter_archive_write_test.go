// Regression tests for writing into archives through ArchiveFS: several
// writers to one archive at the same time must all land (each write
// rewrites the whole archive, so without the per-archive lock the last
// rename used to win), adding a file over a folder of a tar must fail
// instead of hiding the folder's contents, and one tar write with files for
// several folders must be a single, all-or-nothing rewrite.
package vfs

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
)

func TestArchiveConcurrentUploads(t *testing.T) {
	for _, kind := range []string{"zip", "tar"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			arc := filepath.Join(dir, "a."+kind)
			if kind == "zip" {
				makeZip(t, arc, map[string]string{"seed.txt": "s"})
			} else {
				makeTar(t, arc, map[string]string{"seed.txt": "s"})
			}
			src := t.TempDir()
			a := NewArchiveFS()
			a.TempDir = t.TempDir()
			defer a.Close()
			const n = 8
			var wg sync.WaitGroup
			errs := make([]error, n)
			for i := 0; i < n; i++ {
				local := filepath.Join(src, fmt.Sprintf("f%d.txt", i))
				if err := os.WriteFile(local, []byte("x"), 0o644); err != nil {
					t.Fatal(err)
				}
				wg.Add(1)
				go func(i int, local string) {
					defer wg.Done()
					_, errs[i] = a.Upload(context.Background(), local, a.Join(arc, filepath.Base(local)), TransferOptions{})
				}(i, local)
			}
			wg.Wait()
			for i, err := range errs {
				if err != nil {
					t.Fatalf("upload %d: %v", i, err)
				}
			}
			l, err := a.List(context.Background(), arc)
			if err != nil {
				t.Fatal(err)
			}
			got := names(l)
			sort.Strings(got)
			want := []string{"seed.txt"}
			for i := 0; i < n; i++ {
				want = append(want, fmt.Sprintf("f%d.txt", i))
			}
			sort.Strings(want)
			eq(t, "entries", got, want)
		})
	}
}

func TestArchiveTarAddFileOverFolder(t *testing.T) {
	dir := t.TempDir()
	arc := filepath.Join(dir, "a.tar")
	makeTar(t, arc, map[string]string{"docs/inner.txt": "i"})
	src := t.TempDir()
	local := filepath.Join(src, "docs")
	if err := os.WriteFile(local, []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := NewArchiveFS()
	a.TempDir = t.TempDir()
	defer a.Close()
	if err := a.AddLocal(context.Background(), arc, []string{local}); err == nil {
		t.Fatal("adding a file over a folder succeeded")
	}
	l, err := a.List(context.Background(), a.Join(arc, "docs"))
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "docs", names(l), []string{"inner.txt"})
}

func TestArchiveTarBatchIsOneRewrite(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	arc := filepath.Join(dir, "a.tar")
	makeTar(t, arc, map[string]string{"seed.txt": "s", "old/x.txt": "x"})
	src := t.TempDir()
	good := filepath.Join(src, "local.txt")
	if err := os.WriteFile(good, []byte("g"), 0o644); err != nil {
		t.Fatal(err)
	}
	be := &tarBackend{}

	// A failing item for a second folder must leave the archive untouched,
	// including the delete and the first folder's addition.
	before, err := os.ReadFile(arc)
	if err != nil {
		t.Fatal(err)
	}
	bad := arcChanges{
		Delete: []string{"seed.txt"},
		Add: []arcAdd{
			{Source: good, Dest: "a/renamed.txt"},
			{Source: filepath.Join(src, "missing.txt"), Dest: "b/y.txt"},
		},
	}
	if err := be.update(ctx, arc, bad); err == nil {
		t.Fatal("batch with a missing source succeeded")
	}
	after, err := os.ReadFile(arc)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("failed batch changed the archive")
	}

	// A good batch with new names and several folders lands completely.
	ok := arcChanges{
		Delete: []string{"seed.txt"},
		Rename: [][2]string{{"old", "new"}},
		Add: []arcAdd{
			{Source: good, Dest: "a/renamed.txt"},
			{Source: good, Dest: "b/c/other.txt"},
			{Source: good, Dest: "top.txt"},
		},
	}
	if err := be.update(ctx, arc, ok); err != nil {
		t.Fatal(err)
	}
	a := NewArchiveFS()
	a.TempDir = t.TempDir()
	defer a.Close()
	for _, c := range []struct {
		dir  string
		want []string
	}{
		{"", []string{"a/", "b/", "new/", "top.txt"}},
		{"a", []string{"renamed.txt"}},
		{"b/c", []string{"other.txt"}},
		{"new", []string{"x.txt"}},
	} {
		p := arc
		if c.dir != "" {
			p = a.Join(arc, c.dir)
		}
		l, err := a.List(ctx, p)
		if err != nil {
			t.Fatalf("%s: %v", c.dir, err)
		}
		got := names(l)
		sort.Strings(got)
		eq(t, c.dir, got, c.want)
	}
	if got := readAllFS(t, a, a.Join(arc, "b/c/other.txt")); got != "g" {
		t.Errorf("content = %q", got)
	}
}

// readAllFS reads a whole file from an ArchiveFS.
func readAllFS(t *testing.T, a *ArchiveFS, p string) string {
	t.Helper()
	rc, err := a.Open(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
