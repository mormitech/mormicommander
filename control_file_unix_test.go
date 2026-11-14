//go:build unix

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// The control file holds the token that lets a program drive the whole app. Its default place is the shared temporary
// folder (/tmp on Linux), where another user can make the file first, readable for all, or as a link to a file of
// this user's. os.WriteFile kept such a file's permissions (the token readable by everyone) and followed the link
// (overwriting the file it points to). The file is now always a new private file put in place by a rename.
func TestControlFileIsNewAndPrivate(t *testing.T) {
	dir := t.TempDir()
	ctl := filepath.Join(dir, "ctl.json")
	if err := os.WriteFile(ctl, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	os.Chmod(ctl, 0o644)
	a := NewApp(startOpts{ControlFile: ctl})
	if _, err := a.startControl("0"); err != nil {
		t.Fatal(err)
	}
	st, err := os.Lstat(ctl)
	if err != nil || st.Mode().Perm()&0o077 != 0 {
		t.Errorf("control file over a readable leftover: %v %v", st.Mode(), err)
	}
	a.stopControl()

	victim := filepath.Join(dir, "victim.txt")
	os.WriteFile(victim, []byte("keep me"), 0o600)
	link := filepath.Join(dir, "link.json")
	if err := os.Symlink(victim, link); err != nil {
		t.Fatal(err)
	}
	b := NewApp(startOpts{ControlFile: link})
	b.startControl("0")
	defer b.stopControl()
	if got, _ := os.ReadFile(victim); string(got) != "keep me" {
		t.Errorf("the file a link at the control file's place points to was overwritten: %q", got)
	}
}
