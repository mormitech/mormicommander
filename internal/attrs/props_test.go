// Tests for Stat, the properties totals and the file type line, all on files
// created in a temporary folder.
package attrs

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func write(t *testing.T, path string, data []byte, perm os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, perm); err != nil {
		t.Fatal(err)
	}
}

func TestStatBasics(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "a.txt")
	write(t, f, bytes.Repeat([]byte("x"), 10000), 0o644)
	info, err := Stat(f)
	if err != nil {
		t.Fatal(err)
	}
	if info.Kind != KindFile || info.Name != "a.txt" || info.Size != 10000 || info.IsDir() {
		t.Errorf("basic fields wrong: %+v", info)
	}
	if info.SizeOnDisk < 10000 && info.SizeOnDisk != -1 {
		// Compressing file systems may store less; accept but log it.
		t.Logf("size on disk %d smaller than size", info.SizeOnDisk)
	}
	if info.SizeOnDisk == -1 {
		t.Error("size on disk unknown")
	}
	if info.Modified.IsZero() || !info.HasAccessed {
		t.Error("times missing")
	}
	if Caps().ReadCreated && runtime.GOOS != "linux" && !info.HasCreated {
		t.Error("creation time missing")
	}
	if runtime.GOOS != "windows" {
		if info.Perm() != 0o644 || info.UID < 0 || info.Owner == "" || info.Group == "" || info.Links != 1 || info.Inode == 0 {
			t.Errorf("unix fields wrong: %+v", info)
		}
		if info.Win&AttrReadOnly != 0 || ColumnString(info) != "-rw-r--r--" {
			t.Errorf("derived attrs %v column %q", info.Win, ColumnString(info))
		}
	} else if !info.WinNative || len(ColumnString(info)) != 4 {
		t.Errorf("windows attrs not native: %+v", info)
	}

	d, err := Stat(dir)
	if err != nil || d.Kind != KindDir || d.Win&AttrDirectory == 0 {
		t.Errorf("dir stat = %+v, %v", d, err)
	}
	if _, err := Stat(filepath.Join(dir, "missing")); err == nil {
		t.Error("missing file gave no error")
	}
}

func TestStatHiddenAndReadOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("dot names are not hidden on Windows")
	}
	dir := t.TempDir()
	h := filepath.Join(dir, ".secret")
	write(t, h, []byte("x"), 0o444)
	info, err := Stat(h)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Hidden || info.Win&AttrHidden == 0 || info.Win&AttrReadOnly == 0 {
		t.Errorf("hidden/read-only not derived: %+v", info)
	}
}

func TestStatSymlinks(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	write(t, target, []byte("hello"), 0o644)
	link := filepath.Join(dir, "link")
	broken := filepath.Join(dir, "broken")
	dlink := filepath.Join(dir, "dirlink")
	if err := os.Symlink("target.txt", link); err != nil {
		t.Skipf("symlinks not available: %v", err)
	}
	if err := os.Symlink("nowhere", broken); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(dir, dlink); err != nil {
		t.Fatal(err)
	}
	li, err := Stat(link)
	if err != nil {
		t.Fatal(err)
	}
	wantResolved, _ := filepath.EvalSymlinks(target)
	if li.Kind != KindSymlink || li.LinkTarget != "target.txt" || li.LinkBroken || li.LinkResolved != wantResolved {
		t.Errorf("link info = %+v (want resolved %q)", li, wantResolved)
	}
	if ti := DescribeType(li); ti.Description != "Symbolic link to file" {
		t.Errorf("link type = %q", ti.Description)
	}
	bi, err := Stat(broken)
	if err != nil {
		t.Fatal(err)
	}
	if !bi.LinkBroken || bi.LinkResolved != filepath.Join(dir, "nowhere") {
		t.Errorf("broken link = %+v", bi)
	}
	if ti := DescribeType(bi); !strings.Contains(ti.Description, "target missing") {
		t.Errorf("broken type = %q", ti.Description)
	}
	di, _ := Stat(dlink)
	if ti := DescribeType(di); ti.Description != "Symbolic link to folder" {
		t.Errorf("dir link type = %q", ti.Description)
	}
	fi, err := StatFollow(link)
	if err != nil || fi.Kind != KindFile || fi.Size != 5 {
		t.Errorf("StatFollow = %+v, %v", fi, err)
	}
	if runtime.GOOS != "windows" && !strings.HasPrefix(ColumnString(li), "l") {
		t.Errorf("link column = %q", ColumnString(li))
	}
}

func TestDescribeType(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name     string
		data     []byte
		perm     os.FileMode
		wantDesc string
		wantMIME string
		wantExec bool
	}{
		{"photo.png", []byte("\x89PNG\r\n\x1a\nxxxx"), 0o644, "PNG image", "image/png", false},
		{"notes.txt", []byte("hello"), 0o644, "Text document", "text/plain", false},
		{"main.go", []byte("package main"), 0o644, "Go source file", "text/x-go", false},
		{"archive.zip", []byte("PK\x03\x04"), 0o644, "ZIP archive", "application/zip", false},
		{"README", []byte("plain words here\n"), 0o644, "Text document", "text/plain", false},
		{"blob", []byte{0, 1, 2, 3, 0xff}, 0o644, "File", "application/octet-stream", false},
		{"thing.qqzz", []byte{0, 1}, 0o644, "QQZZ file", "application/octet-stream", false},
		{"elfprog", []byte("\x7fELF\x02\x01\x01"), 0o755, "ELF executable", "application/x-elf", true},
		{"macho", []byte{0xcf, 0xfa, 0xed, 0xfe, 7, 0, 0, 1}, 0o755, "Mach-O executable", "application/x-mach-binary", true},
		{"tool.exe", []byte("MZ\x90\x00"), 0o644, "Windows program", "application/vnd.microsoft.portable-executable", true},
		{"run", []byte("#!/usr/bin/env python3\nprint(1)\n"), 0o755, "Python script", "text/x-script", true},
		{"build.sh", []byte("#!/bin/sh\necho\n"), 0o644, "Shell script", "text/x-shellscript", false},
		{"empty", nil, 0o644, "File", "", false},
	}
	for _, tt := range tests {
		p := filepath.Join(dir, tt.name)
		write(t, p, tt.data, tt.perm)
		if err := os.Chmod(p, tt.perm); err != nil {
			t.Fatal(err)
		}
		info, err := Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		ti := DescribeType(info)
		wantExec := tt.wantExec
		if runtime.GOOS == "windows" && tt.perm&0o111 != 0 && tt.name != "tool.exe" {
			wantExec = ti.Executable // no execute bits on Windows
		}
		if ti.Description != tt.wantDesc || ti.MIME != tt.wantMIME || ti.Executable != wantExec {
			t.Errorf("%s: got %q %q exec=%v, want %q %q exec=%v", tt.name, ti.Description, ti.MIME, ti.Executable, tt.wantDesc, tt.wantMIME, wantExec)
		}
	}
	app := filepath.Join(dir, "Thing.app")
	if err := os.Mkdir(app, 0o755); err != nil {
		t.Fatal(err)
	}
	ai, _ := Stat(app)
	if ti := DescribeType(ai); ti.Description != "Application bundle" || !ti.Executable {
		t.Errorf("app bundle = %+v", ti)
	}
	di, _ := Stat(dir)
	if ti := DescribeType(di); ti.Description != "Folder" {
		t.Errorf("folder = %+v", ti)
	}
}

func TestComputeTotals(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a", "one.bin"), make([]byte, 1000), 0o644)
	write(t, filepath.Join(dir, "a", "b", "two.bin"), make([]byte, 3000), 0o644)
	write(t, filepath.Join(dir, "three.bin"), make([]byte, 500), 0o644)
	hard := runtime.GOOS != "windows"
	if hard {
		if err := os.Link(filepath.Join(dir, "three.bin"), filepath.Join(dir, "a", "three-again.bin")); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("three.bin", filepath.Join(dir, "lnk")); err != nil && runtime.GOOS != "windows" {
		t.Fatal(err)
	}
	calls := 0
	tot, err := ComputeTotals(context.Background(), []string{dir}, func(Totals) { calls++ })
	if err != nil {
		t.Fatal(err)
	}
	wantFiles := int64(3)
	if hard {
		wantFiles = 4 // the hard link is a second name, counted as a file
	}
	if tot.Files != wantFiles || tot.Folders != 3 || tot.Size != 4500 || tot.Errors != 0 {
		t.Errorf("totals = %+v", tot)
	}
	if runtime.GOOS != "windows" && tot.Links != 1 {
		t.Errorf("links = %d", tot.Links)
	}
	if tot.SizeOnDisk <= 0 || calls == 0 {
		t.Errorf("size on disk %d, progress calls %d", tot.SizeOnDisk, calls)
	}
	// Selection of single files plus a missing one.
	tot2, err := ComputeTotals(context.Background(), []string{filepath.Join(dir, "three.bin"), filepath.Join(dir, "nope")}, nil)
	if err != nil || tot2.Files != 1 || tot2.Size != 500 || tot2.Errors != 1 {
		t.Errorf("selection totals = %+v, %v", tot2, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ComputeTotals(ctx, []string{dir}, nil); err == nil {
		t.Error("canceled context not reported")
	}
}

func TestCaps(t *testing.T) {
	c := Caps()
	switch runtime.GOOS {
	case "windows":
		if !c.NativeAttrs || c.UnixPerms || c.Owner || c.SettableAttrs&AttrSystem == 0 {
			t.Errorf("windows caps = %+v", c)
		}
	case "darwin":
		if !c.UnixPerms || !c.CreatedTime || !c.HiddenFlag || c.SettableAttrs != AttrReadOnly|AttrHidden {
			t.Errorf("darwin caps = %+v", c)
		}
	case "linux":
		if !c.UnixPerms || c.CreatedTime || c.HiddenFlag || c.SettableAttrs != AttrReadOnly {
			t.Errorf("linux caps = %+v", c)
		}
	}
}

func TestKindString(t *testing.T) {
	for k, want := range map[Kind]string{KindFile: "File", KindDir: "Folder", KindSymlink: "Symbolic link",
		KindNamedPipe: "Named pipe", KindSocket: "Socket", KindDevice: "Block device",
		KindCharDevice: "Character device", KindIrregular: "Special file"} {
		if k.String() != want {
			t.Errorf("%d: %q", k, k.String())
		}
	}
}
