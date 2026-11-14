package fileops

// Tests for review findings of issue #91 (second engines review).

import (
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// An overwrite writes to a hidden temporary file next to the target first. Its name used to be the whole target name
// plus a prefix and a suffix, so a target whose name was close to the 255-byte limit could not be overwritten at all
// ("file name too long").
func TestOverwriteLongName(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	name := strings.Repeat("n", 240) + ".txt"
	write(t, filepath.Join(src, name), "new")
	write(t, filepath.Join(dst, name), "old")
	q := NewQueue(nil)
	p := start(t, q, Request{Kind: Copy, Sources: []string{filepath.Join(src, name)}, Target: dst, Overwrite: Overwrite})
	if p.State != StateDone || len(p.Errors) > 0 {
		t.Fatalf("state %s, errors %v", p.State, p.Errors)
	}
	if got := read(t, filepath.Join(dst, name)); got != "new" {
		t.Errorf("target holds %q, want the new contents", got)
	}
}

// A case-only rename goes through a temporary name, which had the same length problem.
func TestCaseOnlyRenameLongName(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "probe"), "x")
	if !exists(filepath.Join(dir, "PROBE")) {
		t.Skip("the file system is case-sensitive")
	}
	name := strings.Repeat("n", 240) + ".txt"
	write(t, filepath.Join(dir, name), "data")
	upper := strings.ToUpper(name)
	got, err := RenameItem(filepath.Join(dir, name), upper)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(got) != upper || read(t, got) != "data" {
		t.Errorf("renamed to %s", got)
	}
}

func TestTempStemCutsBetweenCharacters(t *testing.T) {
	s := tempStem(strings.Repeat("é", 100))
	if len(s) > 64 || !utf8.ValidString(s) {
		t.Errorf("stem %q (%d bytes)", s, len(s))
	}
	if got := tempStem("short.txt"); got != "short.txt" {
		t.Errorf("short name became %q", got)
	}
}
