// Regression tests for saving a compare document through links on Unix: a
// symbolic link must stay a link with its target updated, and a hard-linked
// file must keep sharing the new content with its other names.

//go:build unix

package compare

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveThroughSymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.txt")
	link := filepath.Join(dir, "link.txt")
	other := filepath.Join(dir, "other.txt")
	os.WriteFile(real, []byte("a\nb\n"), 0o640)
	os.WriteFile(other, []byte("a\nc\n"), 0o644)
	if err := os.Symlink("real.txt", link); err != nil {
		t.Fatal(err)
	}
	s, err := OpenFiles(link, other, Options{}, DetectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CopyAllBlocks(Right); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(Left, ""); err != nil {
		t.Fatal(err)
	}
	st, err := os.Lstat(link)
	if err != nil || st.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("link.txt is no longer a symlink: %v %v", st.Mode(), err)
	}
	if b, _ := os.ReadFile(real); string(b) != "a\nc\n" {
		t.Errorf("target not updated: %q", b)
	}
	if st, _ := os.Stat(real); st.Mode().Perm() != 0o640 {
		t.Errorf("target mode changed to %v", st.Mode().Perm())
	}
	// A chain of links and a dangling link: the final name is written.
	chain := filepath.Join(dir, "chain.txt")
	os.Symlink("link.txt", chain)
	if err := WriteFileAtomic(chain, []byte("z")); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(real); string(b) != "z" {
		t.Errorf("chain: target holds %q", b)
	}
	dangling := filepath.Join(dir, "dangling.txt")
	os.Symlink("new.txt", dangling)
	if err := WriteFileAtomic(dangling, []byte("n")); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "new.txt")); string(b) != "n" {
		t.Errorf("dangling link target holds %q", b)
	}
	loop := filepath.Join(dir, "loop.txt")
	os.Symlink("loop.txt", loop)
	if err := WriteFileAtomic(loop, []byte("x")); err == nil {
		t.Error("a link loop must fail")
	}
}

func TestSaveKeepsHardLinks(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	os.WriteFile(a, []byte("old\n"), 0o644)
	if err := os.Link(a, b); err != nil {
		t.Skipf("hard links not supported: %v", err)
	}
	if err := NewDocument("new\n").SaveFile(a); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(b); string(got) != "new\n" {
		t.Errorf("other hard link holds %q", got)
	}
	sa, _ := os.Stat(a)
	sb, _ := os.Stat(b)
	if !os.SameFile(sa, sb) {
		t.Error("hard link broken by save")
	}
}
