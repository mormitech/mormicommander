// Regression test: the "same file" shortcut that lets a case-only rename
// through on a case-insensitive disk must not match a hard link. Renaming
// "a" onto "b" when "b" is another link to the same file would do nothing
// while reporting success, so it has to be reported as "already exists".

package multirename

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestHardLinkTargetIsAConflict(t *testing.T) {
	dir, items := makeFiles(t, "a")
	if err := os.Link(filepath.Join(dir, "a"), filepath.Join(dir, "b")); err != nil {
		t.Skipf("hard links not supported: %v", err)
	}
	res, err := Preview(items, DefaultSettings(), PreviewOptions{Names: []string{"b"}})
	if err != nil {
		t.Fatal(err)
	}
	if !res.HasErrors() {
		t.Fatalf("preview accepted a rename onto a hard link: %+v", res.Entries)
	}
	// Even with the preview's disk check off, Execute must refuse.
	res, err = Preview(items, DefaultSettings(), PreviewOptions{Names: []string{"b"}, SkipDiskCheck: true})
	if err != nil || res.HasErrors() {
		t.Fatalf("preview: %v %+v", err, res)
	}
	if _, err := Execute(res, ExecOptions{}); !errors.Is(err, ErrTargetExists) {
		t.Fatalf("Execute err = %v, want ErrTargetExists", err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "a")); err != nil {
		t.Fatalf("a should still exist: %v", err)
	}
}
