// Tests that the loop-safe walker terminates on trees with links pointing at
// their own ancestors, reports those loops, skips duplicate folders when
// asked, does not descend into links when not following them, and stops on
// cancellation.

package links

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"sort"
	"testing"
)

func loopTree(t *testing.T) string {
	t.Helper()
	dir := realTemp(t)
	writeFile(t, filepath.Join(dir, "a", "f1"), "1")
	writeFile(t, filepath.Join(dir, "b", "f2"), "2")
	symlinkOrSkip(t, "..", filepath.Join(dir, "a", "up"))                    // loop
	symlinkOrSkip(t, filepath.Join(dir, "b"), filepath.Join(dir, "a", "b2")) // duplicate of b
	symlinkOrSkip(t, filepath.Join(dir, "gone"), filepath.Join(dir, "broken"))
	return dir
}

func collect(t *testing.T, root string, opt WalkOptions) (seen []string, loops, dups, broken int) {
	t.Helper()
	err := Walk(context.Background(), root, opt, func(p string, info fs.FileInfo, err error) error {
		rel, _ := filepath.Rel(root, p)
		switch {
		case errors.Is(err, ErrLoop):
			loops++
		case errors.Is(err, ErrAlreadyVisited):
			dups++
		case errors.Is(err, ErrBroken):
			broken++
		case err != nil:
			t.Errorf("%s: %v", rel, err)
		default:
			seen = append(seen, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(seen)
	return
}

func TestWalkFollowLinks(t *testing.T) {
	root := loopTree(t)
	seen, loops, dups, broken := collect(t, root, WalkOptions{FollowLinks: true})
	want := []string{".", "a", "a/b2", "a/b2/f2", "a/f1", "b", "b/f2"}
	if !equal(seen, want) {
		t.Errorf("seen = %v, want %v", seen, want)
	}
	if loops != 1 || dups != 0 || broken != 1 {
		t.Errorf("loops=%d dups=%d broken=%d", loops, dups, broken)
	}

	seen, loops, dups, _ = collect(t, root, WalkOptions{FollowLinks: true, SkipDuplicates: true})
	// "a" sorts before "b", so b is first reached through the link a/b2 and
	// the real b is the duplicate.
	want = []string{".", "a", "a/b2", "a/b2/f2", "a/f1"}
	if !equal(seen, want) || loops != 1 || dups != 1 {
		t.Errorf("skip dups: seen = %v loops=%d dups=%d", seen, loops, dups)
	}
}

func TestWalkNoFollow(t *testing.T) {
	root := loopTree(t)
	seen, loops, _, broken := collect(t, root, WalkOptions{})
	want := []string{".", "a", "a/b2", "a/f1", "a/up", "b", "b/f2", "broken"}
	if !equal(seen, want) || loops != 0 || broken != 0 {
		t.Errorf("seen = %v loops=%d broken=%d", seen, loops, broken)
	}
}

func TestWalkSkipAndCancel(t *testing.T) {
	root := loopTree(t)
	var n int
	err := Walk(context.Background(), root, WalkOptions{FollowLinks: true}, func(p string, info fs.FileInfo, err error) error {
		n++
		if info != nil && info.IsDir() && filepath.Base(p) == "a" {
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil || n == 0 {
		t.Fatalf("err=%v n=%d", err, n)
	}
	stopped := 0
	err = Walk(context.Background(), root, WalkOptions{}, func(string, fs.FileInfo, error) error {
		stopped++
		return filepath.SkipAll
	})
	if err != nil || stopped != 1 {
		t.Errorf("SkipAll: err=%v calls=%d", err, stopped)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Walk(ctx, root, WalkOptions{}, func(string, fs.FileInfo, error) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Errorf("cancel: err = %v", err)
	}
	var gotErr error
	_ = Walk(context.Background(), filepath.Join(root, "missing"), WalkOptions{}, func(_ string, _ fs.FileInfo, err error) error {
		gotErr = err
		return nil
	})
	if !errors.Is(gotErr, fs.ErrNotExist) {
		t.Errorf("missing root: %v", gotErr)
	}
}

func TestLoopDetector(t *testing.T) {
	dir := realTemp(t)
	writeFile(t, filepath.Join(dir, "x", "f"), "")
	symlinkOrSkip(t, filepath.Join(dir, "x"), filepath.Join(dir, "xl"))
	d := NewLoopDetector(true)
	leave, err := d.Enter(filepath.Join(dir, "x"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Enter(filepath.Join(dir, "xl")); !errors.Is(err, ErrLoop) {
		t.Errorf("same folder while inside: %v", err)
	}
	leave()
	if _, err := d.Enter(filepath.Join(dir, "xl")); !errors.Is(err, ErrAlreadyVisited) {
		t.Errorf("same folder after leaving: %v", err)
	}
	d2 := NewLoopDetector(false)
	l2, _ := d2.Enter(filepath.Join(dir, "x"))
	l2()
	if _, err := d2.Enter(filepath.Join(dir, "xl")); err != nil {
		t.Errorf("without dup skipping: %v", err)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
