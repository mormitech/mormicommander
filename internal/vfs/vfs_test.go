package vfs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func byName(es []Entry) map[string]Entry {
	m := map[string]Entry{}
	for _, e := range es {
		m[e.Name] = e
	}
	return m
}

func TestLocalList(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644)
	os.WriteFile(filepath.Join(dir, ".hidden"), nil, 0o644)
	os.Mkdir(filepath.Join(dir, "sub"), 0o755)
	if runtime.GOOS != "windows" {
		os.WriteFile(filepath.Join(dir, "run.sh"), []byte("#!/bin/sh\n"), 0o755)
		os.Symlink(filepath.Join(dir, "sub"), filepath.Join(dir, "link"))
		os.Symlink(filepath.Join(dir, "missing"), filepath.Join(dir, "broken"))
	}
	l, err := Local{}.List(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	m := byName(l.Entries)
	if e := m["a.txt"]; e.Size != 5 || e.Flags&FlagDir != 0 || e.MTime == 0 {
		t.Errorf("a.txt: %+v", e)
	}
	if m[".hidden"].Flags&FlagHidden == 0 {
		t.Error("dotfile not hidden")
	}
	if m["sub"].Flags&FlagDir == 0 {
		t.Error("sub is no folder")
	}
	if runtime.GOOS != "windows" {
		if f := m["link"].Flags; f&FlagLink == 0 || f&FlagDir == 0 {
			t.Errorf("link flags %b", f)
		}
		if f := m["broken"].Flags; f&FlagBroken == 0 {
			t.Errorf("broken flags %b", f)
		}
		if m["run.sh"].Flags&FlagExec == 0 {
			t.Error("run.sh not executable")
		}
	}
	if l.Parent != filepath.Dir(l.Path) {
		t.Errorf("parent %q of %q", l.Parent, l.Path)
	}
	if rows := Rows(l.Entries); len(rows) != len(l.Entries) || len(rows[0]) != 5 {
		t.Errorf("rows %v", rows)
	}
}

func TestLocalListBig(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 3000; i++ { // more than one chunk, so the parallel path runs
		os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%05d", i)), []byte("x"), 0o644)
	}
	l, err := Local{}.List(context.Background(), dir)
	if err != nil || len(l.Entries) != 3000 {
		t.Fatal(err, len(l.Entries))
	}
	for _, e := range l.Entries {
		if e.Size != 1 {
			t.Fatalf("entry not filled: %+v", e)
		}
	}
}

func TestLocalErrors(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "file")
	os.WriteFile(f, nil, 0o644)
	if _, err := (Local{}).List(context.Background(), f); !errors.Is(err, ErrNotFolder) {
		t.Errorf("file: %v", err)
	}
	if _, err := (Local{}).List(context.Background(), filepath.Join(dir, "nope")); !os.IsNotExist(err) {
		t.Errorf("missing: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (Local{}).List(ctx, dir); err == nil {
		t.Error("cancelled listing gave no error")
	}
}

func TestLocalParentRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		if r := (Local{}).Root(`C:\Users\x`); r != `C:\` {
			t.Errorf("root %q", r)
		}
		return
	}
	if p := (Local{}).Parent("/"); p != "" {
		t.Errorf("parent of / = %q", p)
	}
	if p := (Local{}).Parent("/a/b/"); p != "/a" {
		t.Errorf("parent = %q", p)
	}
	if r := (Local{}).Root("/usr/lib"); r != "/" {
		t.Errorf("root %q", r)
	}
	if runtime.GOOS == "darwin" {
		if r := (Local{}).Root("/Volumes/USB/x/y"); r != "/Volumes/USB" {
			t.Errorf("root %q", r)
		}
	}
}

func TestSplitExt(t *testing.T) {
	for in, want := range map[string][2]string{"a.tar.gz": {"a.tar", "gz"}, ".bashrc": {".bashrc", ""}, "README": {"README", ""}, "x.": {"x.", ""}, "b.TXT": {"b", "TXT"}} {
		if b, e := SplitExt(in); b != want[0] || e != want[1] {
			t.Errorf("%s → %q %q", in, b, e)
		}
	}
}

func TestMock(t *testing.T) {
	m := NewMock()
	l, err := m.List(context.Background(), MockHome)
	if err != nil {
		t.Fatal(err)
	}
	es := byName(l.Entries)
	if es["Documents"].Flags&FlagDir == 0 || es[".bashrc"].Flags&FlagHidden == 0 {
		t.Errorf("home: %v", es)
	}
	if f := es["Latest photos"].Flags; f&FlagLink == 0 || f&FlagDir == 0 {
		t.Errorf("link flags %b", f)
	}
	l2, err := m.List(context.Background(), MockHome+"/Latest photos")
	if err != nil || len(l2.Entries) != 48 {
		t.Errorf("through link: %v %d", err, len(l2.Entries))
	}
	big, err := m.List(context.Background(), MockBigFolder)
	if err != nil || len(big.Entries) != MockBigCount {
		t.Errorf("big: %v %d", err, len(big.Entries))
	}
	if _, err := m.List(context.Background(), "/nope"); !os.IsNotExist(err) {
		t.Errorf("missing: %v", err)
	}
	if _, err := m.List(context.Background(), MockHome+"/notes.txt"); !errors.Is(err, ErrNotFolder) {
		t.Errorf("file: %v", err)
	}
	if r := m.Root("/Volumes/USB stick/x"); r != "/Volumes/USB stick" {
		t.Errorf("root %q", r)
	}
	a, _ := m.Stat(MockHome + "/notes.txt")
	b, _ := NewMock().Stat(MockHome + "/notes.txt")
	if a != b || a.Size == 0 {
		t.Errorf("mock not stable: %+v %+v", a, b)
	}
}

type fakeFS struct{ Mock }

func TestRegister(t *testing.T) {
	f := &fakeFS{}
	Register("fake", f)
	if For("fake://host/x", Local{}) != FS(f) {
		t.Error("scheme not found")
	}
	if _, ok := For("/tmp", Local{}).(Local); !ok {
		t.Error("plain path not local")
	}
}
