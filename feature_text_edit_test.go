package main

// The built-in text editor (F-594): what it reads, how it saves (line ends and byte order mark kept), and that a save
// never overwrites a file another program changed unless asked to.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTextEditDecodeEncode(t *testing.T) {
	for _, in := range []string{"a\nb\n", "a\r\nb\r\n", "\xEF\xBB\xBFx\r\ny", "", "ünïcode"} {
		text, crlf, bom, err := decodeTextFile([]byte(in))
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if strings.Contains(text, "\r") {
			t.Fatalf("%q: text keeps CR: %q", in, text)
		}
		if got := string(encodeTextFile(text, crlf, bom)); got != in {
			t.Fatalf("round trip of %q gave %q", in, got)
		}
	}
	for _, bad := range []string{"a\x00b", "\xff\xfeA\x00"} {
		if _, _, _, err := decodeTextFile([]byte(bad)); err == nil {
			t.Fatalf("%q was taken as text", bad)
		}
	}
	// the page's text with LF is written back with CRLF when the file had it
	if got := string(encodeTextFile("x\ny", true, false)); got != "x\r\ny" {
		t.Fatalf("CRLF save: %q", got)
	}
}

func TestTextEditReadWrite(t *testing.T) {
	e := &ListerEdit{app: &App{}}
	dir := t.TempDir()
	p := filepath.Join(dir, "a.txt")
	os.WriteFile(p, []byte("one\r\ntwo\r\n"), 0o600)
	f, err := e.ReadText(p)
	if err != nil {
		t.Fatal(err)
	}
	if f.Text != "one\ntwo\n" || !f.CRLF || f.BOM || f.Name != "a.txt" {
		t.Fatalf("read: %+v", f)
	}
	f2, err := e.WriteText(f, "one\ntwo\nthree\n", false)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "one\r\ntwo\r\nthree\r\n" {
		t.Fatalf("saved %q", b)
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o600 {
		t.Fatalf("permissions changed to %v", fi.Mode().Perm())
	}
	// another program changes the file: the next save is refused until forced
	later := time.Now().Add(time.Hour)
	os.WriteFile(p, []byte("theirs"), 0o600)
	os.Chtimes(p, later, later)
	if _, err := e.WriteText(f2, "mine", false); err == nil || !strings.Contains(err.Error(), "changed on disk") {
		t.Fatalf("overwrote a changed file: %v", err)
	}
	if b, _ := os.ReadFile(p); string(b) != "theirs" {
		t.Fatalf("changed file was touched: %q", b)
	}
	if _, err := e.WriteText(f2, "mine", true); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "mine" {
		t.Fatalf("forced save: %q", b)
	}
	// what it does not take
	bin := filepath.Join(dir, "b.bin")
	os.WriteFile(bin, []byte{0, 1, 2}, 0o644)
	if _, err := e.ReadText(bin); err == nil || !strings.Contains(err.Error(), "external editor") {
		t.Fatalf("binary file: %v", err)
	}
	if _, err := e.ReadText(dir); err == nil {
		t.Fatal("a folder was opened")
	}
	if _, err := e.ReadText(filepath.Join(dir, "none")); err == nil || !strings.Contains(err.Error(), "not there") {
		t.Fatalf("missing file: %v", err)
	}
	if _, err := e.ReadText("ftp://h/x.txt"); err == nil {
		t.Fatal("a server file was opened")
	}
	big := filepath.Join(dir, "big.txt")
	os.WriteFile(big, make([]byte, textEditMax+1), 0o644)
	if _, err := e.ReadText(big); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("big file: %v", err)
	}
	// Shift+F4 with the built-in editor: the file is created, nothing is started
	n, err := e.CreateNew(dir, "new.txt")
	if err != nil || !n.Created || n.Name != "new.txt" {
		t.Fatalf("CreateNew: %+v %v", n, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "new.txt")); err != nil {
		t.Fatal(err)
	}
}
