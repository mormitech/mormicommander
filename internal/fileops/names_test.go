package fileops

// Tests of the name helpers: TC rename masks, the "Only files of this type" filter, auto-rename names and the
// splitting of the copy dialog's target field.

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApplyMask(t *testing.T) {
	for _, c := range []struct{ mask, name, want string }{
		{"", "a.txt", "a.txt"},
		{"*.bak", "report.txt", "report.bak"},
		{"*.bak", "archive.tar.gz", "archive.tar.bak"},
		{"*.*", "a.txt", "a.txt"},
		{"*", "a.txt", "a.txt"},
		{"new_*.*", "a.txt", "new_a.txt"},
		{"????.txt", "abcdefg.doc", "abcd.txt"},
		{"*_old", "notes", "notes_old"},
		{"*.", "a.txt", "a"},
		{"b.txt", "a.txt", "b.txt"},
		{"*.bak", ".bashrc", ".bashrc.bak"},
	} {
		if got := ApplyMask(c.mask, c.name); got != c.want {
			t.Errorf("ApplyMask(%q, %q) = %q, want %q", c.mask, c.name, got, c.want)
		}
	}
}

func TestFilter(t *testing.T) {
	for _, c := range []struct {
		filter, name string
		dir, want    bool
	}{
		{"", "anything", false, true},
		{"*.txt", "a.TXT", false, true},
		{"*.txt", "a.md", false, false},
		{"*.txt;*.md", "a.md", false, true},
		{"*.txt *.md", "a.md", false, true},
		{"*.*", "README", false, true},
		{"*.txt|old*", "old.txt", false, false},
		{"*.txt|old*", "new.txt", false, true},
		{"*.txt", "sub", true, true}, // include patterns don't stop the walk
		{"*|vendor/", "vendor", true, false},
		{"*|vendor/", "vendor", false, true}, // a file called vendor is fine
		{`"my file.txt"`, "my file.txt", false, true},
		{`"my file.txt"`, "file.txt", false, false},
	} {
		f := ParseFilter(c.filter)
		got := f.MatchFile(c.name)
		if c.dir {
			got = f.MatchDir(c.name)
		}
		if got != c.want {
			t.Errorf("filter %q on %q (dir %v) = %v, want %v", c.filter, c.name, c.dir, got, c.want)
		}
	}
	if !ParseFilter("").Empty() || ParseFilter("*.txt").Empty() {
		t.Error("Empty wrong")
	}
}

func TestUniqueName(t *testing.T) {
	dir := t.TempDir()
	if got := UniqueName(dir, "a.txt"); got != "a.txt" {
		t.Errorf("free name changed: %q", got)
	}
	write(t, filepath.Join(dir, "a.txt"), "")
	write(t, filepath.Join(dir, "a(2).txt"), "")
	if got := UniqueName(dir, "a.txt"); got != "a(3).txt" {
		t.Errorf("got %q", got)
	}
	os.Mkdir(filepath.Join(dir, "folder"), 0o755)
	if got := UniqueName(dir, "folder"); got != "folder(2)" {
		t.Errorf("got %q", got)
	}
}

func TestParseTarget(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "sub"), 0o755)
	sep := string(filepath.Separator)
	for _, c := range []struct {
		in         string
		n          int
		wantDir    string
		wantRename string
	}{
		{filepath.Join(dir, "sub"), 1, filepath.Join(dir, "sub"), ""},
		{filepath.Join(dir, "new") + sep, 1, filepath.Join(dir, "new"), ""},
		{filepath.Join(dir, "b.txt"), 1, dir, "b.txt"},
		{filepath.Join(dir, "sub", "*.bak"), 3, filepath.Join(dir, "sub"), "*.bak"},
		{filepath.Join(dir, "newdir"), 2, filepath.Join(dir, "newdir"), ""},
		{"", 1, "", ""},
	} {
		d, r := ParseTarget(c.in, c.n)
		if d != c.wantDir || r != c.wantRename {
			t.Errorf("ParseTarget(%q, %d) = %q, %q; want %q, %q", c.in, c.n, d, r, c.wantDir, c.wantRename)
		}
	}
}
