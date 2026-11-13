// Tests for telling console programs from window programs, using hand-made PE headers and real local files.
package launch

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// minimalPE builds the smallest PE file debug/pe accepts: a DOS header, the PE signature, a file header and a
// 64-bit optional header with the given subsystem.
func minimalPE(subsystem uint16) []byte {
	b := make([]byte, 64)
	copy(b, "MZ")
	binary.LittleEndian.PutUint32(b[0x3c:], 64)
	b = append(b, 'P', 'E', 0, 0)
	fh := make([]byte, 20)
	binary.LittleEndian.PutUint16(fh[0:], 0x8664) // AMD64
	binary.LittleEndian.PutUint16(fh[2:], 0)      // no sections
	binary.LittleEndian.PutUint16(fh[16:], 240)   // size of the 64-bit optional header
	binary.LittleEndian.PutUint16(fh[18:], 0x22)
	b = append(b, fh...)
	oh := make([]byte, 240)
	binary.LittleEndian.PutUint16(oh[0:], 0x20b)
	binary.LittleEndian.PutUint16(oh[68:], subsystem)
	binary.LittleEndian.PutUint32(oh[108:], 16)
	return append(b, oh...)
}

func TestExecKindOf(t *testing.T) {
	dir := t.TempDir()
	files := map[string][]byte{
		"gui.exe":     minimalPE(2),
		"console.exe": minimalPE(3),
		"script.sh":   []byte("#!/bin/sh\necho\n"),
		"run.bat":     []byte("@echo off\n"),
		"text.txt":    []byte("hello"),
		"broken.exe":  []byte("MZ broken"),
	}
	for name, data := range files {
		writeFile(t, filepath.Join(dir, name), string(data), 0o755)
	}
	if err := os.MkdirAll(filepath.Join(dir, "Foo.app", "Contents", "MacOS"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "Foo.app", "Contents", "MacOS", "Foo"), "x", 0o755)
	tests := map[string]ExecKind{
		"gui.exe":                    KindGUI,
		"console.exe":                KindConsole,
		"script.sh":                  KindConsole,
		"run.bat":                    KindConsole,
		"text.txt":                   KindUnknown,
		"broken.exe":                 KindUnknown,
		"Foo.app":                    KindGUI,
		"Foo.app/Contents/MacOS/Foo": KindGUI,
		"missing":                    KindUnknown,
		".":                          KindUnknown,
	}
	for name, want := range tests {
		if got := ExecKindOf(filepath.Join(dir, name)); got != want {
			t.Errorf("ExecKindOf(%s) = %v, want %v", name, got, want)
		}
	}
	// The test binary itself is a native program without GUI libraries.
	if exe, err := os.Executable(); err == nil {
		if got := ExecKindOf(exe); got != KindConsole {
			t.Errorf("test binary kind = %v, want console", got)
		}
	}
	if kindFromLibs([]string{"libc.so.6", "libgtk-3.so.0"}) != KindGUI || kindFromLibs([]string{"libc.so.6"}) != KindConsole {
		t.Error("kindFromLibs is wrong")
	}
	if KindGUI.String() != "gui" || KindUnknown.String() != "unknown" {
		t.Error("ExecKind.String is wrong")
	}
}
