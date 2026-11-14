// Regression tests for zip updates: a rename that only changes case must
// not overwrite another existing entry, a rename must not put an entry
// below a file, and updating an archive through a symbolic link must change
// the real archive and keep the link.
package ziparc

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"mormicommander/internal/archive"
)

func TestRenameRefusals(t *testing.T) {
	cases := []struct {
		name  string
		files []string
		from  string
		to    string
		fail  bool
	}{
		{"case-only onto another entry", []string{"a.txt", "A.TXT"}, "a.txt", "A.TXT", true},
		{"case-only onto itself", []string{"a.txt"}, "a.txt", "A.TXT", false},
		{"below a file", []string{"a.txt", "b.txt"}, "a.txt", "b.txt/a.txt", true},
		{"deep below a file", []string{"a.txt", "b.txt"}, "a.txt", "b.txt/x/a.txt", true},
		{"into a folder", []string{"a.txt", "d/b.txt"}, "a.txt", "d/a.txt", false},
		{"into a new folder", []string{"a.txt"}, "a.txt", "n/a.txt", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			zp := filepath.Join(dir, "t.zip")
			// Pack each name from its own source file, because case twins
			// cannot coexist on a case-insensitive disk.
			var ch archive.Changes
			for i, f := range c.files {
				src := filepath.Join(dir, "src", string(rune('a'+i)))
				writeFile(t, src, []byte(f))
				ch.Add = append(ch.Add, archive.AddItem{Source: src, Dest: f})
			}
			pack(t, zp, ch, archive.PackOptions{})
			_, err := Packer{}.Update(bg, zp, archive.Changes{Rename: []archive.Rename{{From: c.from, To: c.to}}}, archive.PackOptions{})
			if c.fail {
				if err == nil {
					t.Fatal("rename succeeded")
				}
				if !errors.Is(err, archive.ErrExist) {
					t.Errorf("err = %v, want ErrExist", err)
				}
				got := contents(t, mustOpen(t, zp, archive.OpenOptions{}))
				if len(got) != len(c.files) {
					t.Errorf("archive changed: %v", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got := contents(t, mustOpen(t, zp, archive.OpenOptions{}))
			if got[c.to] != c.from {
				t.Errorf("after rename: %v", got)
			}
		})
	}
}

func TestUpdateThroughSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need extra privileges on Windows")
	}
	dir := t.TempDir()
	real := filepath.Join(dir, "real.zip")
	src := filepath.Join(dir, "f.txt")
	writeFile(t, src, []byte("f"))
	pack(t, real, archive.Changes{Add: []archive.AddItem{{Source: src}}}, archive.PackOptions{})
	link := filepath.Join(dir, "link.zip")
	if err := os.Symlink("real.zip", link); err != nil {
		t.Fatal(err)
	}
	pack(t, link, archive.Changes{Mkdir: []string{"newdir"}}, archive.PackOptions{})
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Error("the link was replaced by a regular file")
	}
	got := contents(t, mustOpen(t, real, archive.OpenOptions{}))
	if _, ok := got["newdir/"]; !ok {
		t.Errorf("real archive misses newdir: %v", got)
	}
	if got["f.txt"] != "f" {
		t.Errorf("real archive lost f.txt: %v", got)
	}
}

// An update keeps the archive's other hard links pointing at the updated archive.
func TestUpdateKeepsHardLinks(t *testing.T) {
	dir := t.TempDir()
	zp := filepath.Join(dir, "a.zip")
	src := filepath.Join(dir, "f.txt")
	writeFile(t, src, []byte("f"))
	pack(t, zp, archive.Changes{Add: []archive.AddItem{{Source: src}}}, archive.PackOptions{})
	other := filepath.Join(dir, "b.zip")
	if err := os.Link(zp, other); err != nil {
		t.Skip("no hard links here:", err)
	}
	pack(t, zp, archive.Changes{Mkdir: []string{"newdir"}}, archive.PackOptions{})
	a, _ := os.Stat(zp)
	b, _ := os.Stat(other)
	if !os.SameFile(a, b) {
		t.Fatal("hard link broken by the update")
	}
	got := contents(t, mustOpen(t, other, archive.OpenOptions{}))
	if _, ok := got["newdir/"]; !ok || got["f.txt"] != "f" {
		t.Errorf("linked name misses the update: %v", got)
	}
}
