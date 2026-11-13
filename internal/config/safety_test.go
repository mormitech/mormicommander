// Regression tests for two ways a save could damage the user's settings: a
// value with a line break (a Unix folder name may contain one) spilling into
// new INI lines that add sections and keys, and a settings file that is a
// symbolic link being replaced by a regular file instead of updating the
// link's target.
package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNewlineValuesCannotInjectKeys(t *testing.T) {
	c := openTemp(t, "")
	evil := "/tmp/evil\n[Shortcuts]\nF8=cm_Delete"
	c.LeftHistory.Add("/tmp/good")
	c.LeftHistory.Add(evil)
	c.LeftHistory.Add("/tmp/cr\rx")
	if got := c.LeftHistory.Items(); len(got) != 1 || got[0] != "/tmp/good" {
		t.Fatalf("history kept unsafe entries: %q", got)
	}
	c.File.Set("Configuration", "Evil", "a\nb=c")
	c.File.Set("Configuration", "Ev\nil", "x")
	c.File.Set("Bad\nSection", "k", "v")
	// ReplaceSection bypasses Set; rendering must still keep it on one line.
	c.File.ReplaceSection("Test", []KV{{"0", evil}})
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	c2, err := Open(c.Loc)
	if err != nil {
		t.Fatal(err)
	}
	if b, ok := c2.Keymap.Lookup(Hotkey{Key: "F8"}); ok && strings.EqualFold(b.Command, "cm_Delete") {
		t.Fatalf("F8 rebound to %q through an injected line", b.Command)
	}
	if c2.File.Has("Configuration", "Evil") || c2.File.HasSection("Bad\nSection") {
		t.Fatal("Set stored a value with a line break")
	}
	if got := c2.LeftHistory.Items(); len(got) != 1 || got[0] != "/tmp/good" {
		t.Fatalf("history after reload: %q", got)
	}
	data, _ := os.ReadFile(c.Loc.Ini)
	if strings.Contains(string(data), "\nF8=cm_Delete") {
		t.Fatalf("ini contains injected line:\n%s", data)
	}
}

func TestSafeValue(t *testing.T) {
	for s, want := range map[string]bool{"": true, "/a b": true, "a\nb": false, "a\rb": false} {
		if got := SafeValue(s); got != want {
			t.Errorf("SafeValue(%q) = %v, want %v", s, got, want)
		}
	}
}

func TestSaveFollowsSymlinkedIni(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	root := t.TempDir()
	cfg := filepath.Join(root, "cfg")
	dots := filepath.Join(root, "dotfiles")
	for _, d := range []string{cfg, dots} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	real := filepath.Join(dots, "mc.ini")
	if err := os.WriteFile(real, []byte("[Configuration]\nA=1\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(cfg, IniName)
	if err := os.Symlink("../dotfiles/mc.ini", link); err != nil {
		t.Fatal(err)
	}
	c, err := Open(Locations{Dir: cfg, Ini: link, FtpIni: filepath.Join(cfg, FtpIniName)})
	if err != nil {
		t.Fatal(err)
	}
	c.File.Set("Configuration", "B", "2")
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Lstat(link)
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("settings link replaced by a regular file (err %v)", err)
	}
	data, _ := os.ReadFile(real)
	if !strings.Contains(string(data), "B=2") {
		t.Fatalf("link target not updated:\n%s", data)
	}
	if st, _ := os.Stat(real); st.Mode().Perm() != 0o640 {
		t.Fatalf("target mode = %v, want 0640", st.Mode().Perm())
	}

	// A dangling link is followed too: the target file gets created.
	dang := filepath.Join(cfg, "dangling.ini")
	newTarget := filepath.Join(dots, "new.ini")
	if err := os.Symlink(newTarget, dang); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(dang, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Lstat(dang); fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("dangling link replaced")
	}
	if b, _ := os.ReadFile(newTarget); string(b) != "x" {
		t.Fatalf("dangling target content %q", b)
	}
}
