package branch

// Tests of the branch listing: a small tree in a temporary folder comes back as files with relative names, hidden
// folders hide their files, links to folders are not followed, a selection limits the walk, the limit cuts the list,
// and a cancelled walk stops with the context's error.

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"mormicommander/internal/vfs"
)

func write(t *testing.T, root string, names ...string) {
	t.Helper()
	for _, n := range names {
		p := filepath.Join(root, filepath.FromSlash(n))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(n), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func names(r Result) map[string]vfs.Entry {
	m := map[string]vfs.Entry{}
	for _, e := range r.Entries {
		m[filepath.ToSlash(e.Name)] = e
	}
	return m
}

func sorted(m map[string]vfs.Entry) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestWalkWholeTree(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.txt", "sub/b.txt", "sub/deeper/c.txt", ".git/config", "sub/.cache/x.bin")
	if err := os.MkdirAll(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		// a link back to the top must not make the walk endless (links to folders are not followed)
		if err := os.Symlink(root, filepath.Join(root, "sub", "loop")); err != nil {
			t.Fatal(err)
		}
	}
	calls := 0
	r, err := Walk(context.Background(), root, nil, 0, func(int) { calls++ })
	if err != nil {
		t.Fatal(err)
	}
	got := names(r)
	want := []string{".git/config", "a.txt", "sub/.cache/x.bin", "sub/b.txt", "sub/deeper/c.txt"}
	if s := sorted(got); len(s) != len(want) {
		t.Fatalf("files = %v, want %v", s, want)
	} else {
		for i := range want {
			if s[i] != want[i] {
				t.Fatalf("files = %v, want %v", s, want)
			}
		}
	}
	if r.Truncated || calls == 0 {
		t.Errorf("truncated %v, progress calls %d", r.Truncated, calls)
	}
	if got["sub/b.txt"].Size != int64(len("sub/b.txt")) || got["sub/b.txt"].MTime == 0 {
		t.Errorf("size and date are not the real file's: %+v", got["sub/b.txt"])
	}
	for _, n := range []string{".git/config", "sub/.cache/x.bin"} {
		if got[n].Flags&vfs.FlagHidden == 0 {
			t.Errorf("%s is inside a hidden folder but not marked hidden", n)
		}
	}
	if got["sub/deeper/c.txt"].Flags&vfs.FlagHidden != 0 {
		t.Error("a file in a plain folder is marked hidden")
	}
}

func TestWalkSelection(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.txt", "b.txt", "one/x.txt", "one/two/y.txt", "other/z.txt", ".hid/h.txt")
	r, err := Walk(context.Background(), root, []string{"b.txt", "one", ".hid", "missing"}, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := names(r)
	want := []string{".hid/h.txt", "b.txt", "one/two/y.txt", "one/x.txt"}
	s := sorted(got)
	if len(s) != len(want) {
		t.Fatalf("files = %v, want %v", s, want)
	}
	for i := range want {
		if s[i] != want[i] {
			t.Fatalf("files = %v, want %v", s, want)
		}
	}
	if got[".hid/h.txt"].Flags&vfs.FlagHidden == 0 {
		t.Error("a file of a selected hidden folder is not marked hidden")
	}
}

func TestWalkLimitAndErrors(t *testing.T) {
	root := t.TempDir()
	write(t, root, "1.txt", "2.txt", "d/3.txt", "d/4.txt", "d/e/5.txt")
	r, err := Walk(context.Background(), root, nil, 3, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Entries) != 3 || !r.Truncated {
		t.Errorf("limit 3: %d files, truncated %v", len(r.Entries), r.Truncated)
	}
	if _, err := Walk(context.Background(), filepath.Join(root, "nope"), nil, 0, nil); err == nil {
		t.Error("a missing folder gave no error")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Walk(ctx, root, nil, 0, nil); err == nil {
		t.Error("a cancelled walk gave no error")
	}
}

func TestIsHiddenPath(t *testing.T) {
	for in, want := range map[string]bool{
		"a.txt": false, ".a": false, "x/.a": false, ".git/x": true, "a/.b/c/d": true, "a/b/c": false,
	} {
		if got := isHiddenPath(filepath.FromSlash(in)); got != want {
			t.Errorf("isHiddenPath(%q) = %v, want %v", in, got, want)
		}
	}
}

// When the limit stops a branch view of a selection before every selected item was looked at, the list is
// incomplete and must say so; it said it was complete when the limit was reached exactly at the end of an item.
func TestWalkSelectionLimitTruncates(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.txt", "b.txt", "c.txt", "d/x.txt")
	for _, roots := range [][]string{{"a.txt", "b.txt", "c.txt"}, {"a.txt", "d", "c.txt"}} {
		r, err := Walk(context.Background(), root, roots, 2, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(r.Entries) != 2 || !r.Truncated {
			t.Errorf("roots %v: %d entries, truncated %v; want 2, true", roots, len(r.Entries), r.Truncated)
		}
	}
}
