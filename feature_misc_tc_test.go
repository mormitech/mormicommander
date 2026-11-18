package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mormicommander/internal/config"
	lst "mormicommander/internal/listing"
)

// Section 42 (issue #90).

// F-745: descript.ion comments follow renames in place (Shift+F6, multi-rename), across folders, and through swaps.
func TestMoveCommentsNow(t *testing.T) {
	dir, other := t.TempDir(), t.TempDir()
	c, _ := lst.LoadComments(dir)
	c.Set("a.txt", "first")
	c.Set("b.txt", "second")
	c.Set("keep.txt", "stays")
	c.Set("away.txt", "travels")
	if err := c.Save(dir); err != nil {
		t.Fatal(err)
	}
	moveCommentsNow(map[string]string{
		filepath.Join(dir, "a.txt"):    filepath.Join(dir, "b.txt"), // a swap: a -> b and b -> a
		filepath.Join(dir, "b.txt"):    filepath.Join(dir, "a.txt"),
		filepath.Join(dir, "away.txt"): filepath.Join(other, "there.txt"),
		filepath.Join(dir, "none.txt"): filepath.Join(dir, "none2.txt"), // no comment: nothing happens
	})
	got, _ := lst.LoadComments(dir)
	for name, want := range map[string]string{"a.txt": "second", "b.txt": "first", "keep.txt": "stays", "away.txt": "", "none2.txt": ""} {
		if g := got.Get(name); g != want {
			t.Errorf("%s: comment %q, want %q", name, g, want)
		}
	}
	there, _ := lst.LoadComments(other)
	if g := there.Get("there.txt"); g != "travels" {
		t.Errorf("moved to another folder: %q", g)
	}
}

func TestMoveCommentsNowNoCommentsWritesNothing(t *testing.T) {
	dir := t.TempDir()
	moveCommentsNow(map[string]string{filepath.Join(dir, "a"): filepath.Join(dir, "b")})
	if _, err := os.Stat(filepath.Join(dir, lst.CommentFileName)); !os.IsNotExist(err) {
		t.Fatalf("a descript.ion was written for a folder without comments: %v", err)
	}
}

// F-749: the Dock badge label stays short and on one line.
func TestDockBadgeText(t *testing.T) {
	for in, want := range map[string]string{"42%": "42%", "": "", " 1\n00% ": "1 00%", "123456789012": "12345678"} {
		if g := dockBadgeText(in); g != want {
			t.Errorf("dockBadgeText(%q) = %q, want %q", in, g, want)
		}
	}
}

// F-853: the commander variables go into the environment programs are started with.
func TestExportCommanderEnv(t *testing.T) {
	got := map[string]string{}
	exportCommanderEnv(config.Vars{config.VarCommanderPath: "/p", config.VarCommanderIni: "/p/mormicommander.ini", "OTHER": "x"},
		func(k, v string) error { got[k] = v; return nil })
	if len(got) != 2 || got["COMMANDER_PATH"] != "/p" || got["COMMANDER_INI"] != "/p/mormicommander.ini" {
		t.Fatalf("exported %v", got)
	}
}

// F-756: a folder that can't be read for lack of permission gives a message that says so (and what to do on macOS),
// not a crash.
func TestListErrorPermission(t *testing.T) {
	msg := listError("/secret", fs.ErrPermission)
	if !strings.Contains(msg, "You don't have permission to open /secret.") {
		t.Fatalf("message %q", msg)
	}
}
