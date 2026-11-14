package fileops

// Tests for review findings of issue #91 (second engines review).

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
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

// "A folder into itself" was checked on the spelling of the paths only. A target reached through a link to a
// subfolder of the source, or spelled in another case on a case-insensitive disk, slipped through and the folder was
// copied into its own subfolder.
func TestCopyIntoItselfOtherSpelling(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "f", "sub", "x"), "x")
	link := filepath.Join(dir, "link")
	if err := os.Symlink(filepath.Join(dir, "f", "sub"), link); err != nil {
		t.Skip(err)
	}
	targets := []string{link, filepath.Join(link, "deeper")}
	write(t, filepath.Join(dir, "probe"), "x")
	if exists(filepath.Join(dir, "PROBE")) {
		targets = append(targets, filepath.Join(dir, "F", "sub"))
	}
	for _, target := range targets {
		q := NewQueue(nil)
		p := start(t, q, Request{Kind: Copy, Sources: []string{filepath.Join(dir, "f")}, Target: target})
		if p.State != StateFailed || len(p.Errors) == 0 || !strings.Contains(p.Errors[0], "into itself") {
			t.Errorf("target %s: %s %v", target, p.State, p.Errors)
		}
		if exists(filepath.Join(dir, "f", "sub", "f")) || exists(filepath.Join(dir, "f", "sub", "deeper")) {
			t.Fatalf("target %s: the folder was copied into itself", target)
		}
	}
}

// A link copied over an existing file removed that file first and then made the link. Where the link can't be made
// (FAT and exFAT have no links) the old file was gone and nothing took its place, although an overwrite promises to
// leave the old target as it was when the copy fails.
func TestFailedLinkOverwriteKeepsTarget(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	if err := os.Symlink("somewhere", filepath.Join(src, "l")); err != nil {
		t.Skip(err)
	}
	write(t, filepath.Join(dst, "l"), "keep me")
	old := symlink
	symlink = func(string, string) error { return &os.LinkError{Op: "symlink", Err: syscall.ENOTSUP} }
	defer func() { symlink = old }()
	q := NewQueue(nil)
	p := start(t, q, Request{Kind: Copy, Sources: []string{filepath.Join(src, "l")}, Target: dst, Overwrite: Overwrite})
	if p.State != StateFailed {
		t.Errorf("state %s", p.State)
	}
	if got := read(t, filepath.Join(dst, "l")); got != "keep me" {
		t.Errorf("target holds %q", got)
	}
	if entries, _ := os.ReadDir(dst); len(entries) != 1 {
		t.Errorf("left behind: %v", entries)
	}
}

// The new link still replaces the old file when it can be made.
func TestLinkOverwriteReplacesTarget(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	if err := os.Symlink("somewhere", filepath.Join(src, "l")); err != nil {
		t.Skip(err)
	}
	write(t, filepath.Join(dst, "l"), "old")
	q := NewQueue(nil)
	p := start(t, q, Request{Kind: Copy, Sources: []string{filepath.Join(src, "l")}, Target: dst, Overwrite: Overwrite})
	if p.State != StateDone {
		t.Fatalf("state %s %v", p.State, p.Errors)
	}
	if got, err := os.Readlink(filepath.Join(dst, "l")); err != nil || got != "somewhere" {
		t.Errorf("link = %q, %v", got, err)
	}
	if entries, _ := os.ReadDir(dst); len(entries) != 1 {
		t.Errorf("left behind: %v", entries)
	}
}
