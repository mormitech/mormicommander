package main

// Tests for the duplicate actions of Find files (feature_find_dups.go, board issue #32): a copy is replaced with a hard
// link only when its contents really match the kept file, a pair that is already linked is left alone, and Missing
// reports the results that are gone. Everything happens in temporary folders.

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindLinkDuplicates(t *testing.T) {
	dir := t.TempDir()
	keep := filepath.Join(dir, "keep.txt")
	dup := filepath.Join(dir, "sub", "dup.txt")
	other := filepath.Join(dir, "other.txt") // same size, different bytes
	short := filepath.Join(dir, "short.txt")
	findWriteFile(t, keep, "same contents here")
	findWriteFile(t, dup, "same contents here")
	findWriteFile(t, other, "SAME contents here")
	findWriteFile(t, short, "same")
	f := newTestFind(t)

	res := f.LinkDuplicates([]FindDupPair{{Keep: keep, Dup: dup}, {Keep: keep, Dup: other}, {Keep: keep, Dup: short},
		{Keep: keep, Dup: filepath.Join(dir, "gone.txt")}, {Keep: keep, Dup: dir}, {Keep: "sftp://h/a", Dup: dup}})
	if len(res) != 6 {
		t.Fatalf("got %d results", len(res))
	}
	if !res[0].Linked || res[0].Error != "" || res[0].Saved != int64(len("same contents here")) {
		t.Fatalf("the real copy was not linked: %+v", res[0])
	}
	ki, _ := os.Stat(keep)
	di, _ := os.Stat(dup)
	if !os.SameFile(ki, di) {
		t.Fatal("the copy is not a hard link to the kept file")
	}
	for i, r := range res[1:] {
		if r.Linked || r.Error == "" {
			t.Fatalf("pair %d should have been refused: %+v", i+1, r)
		}
	}
	if b, _ := os.ReadFile(other); string(b) != "SAME contents here" {
		t.Fatalf("a file with different contents was changed: %q", b)
	}
	if b, _ := os.ReadFile(short); string(b) != "same" {
		t.Fatalf("a file of another size was changed: %q", b)
	}

	// doing it again: already the same file, nothing saved, no temporary names left behind
	again := f.LinkDuplicates([]FindDupPair{{Keep: keep, Dup: dup}})
	if !again[0].Linked || !again[0].Already || again[0].Saved != 0 {
		t.Fatalf("an existing link was not recognised: %+v", again[0])
	}
	ents, _ := os.ReadDir(filepath.Join(dir, "sub"))
	if len(ents) != 1 {
		t.Fatalf("left-overs next to the copy: %v", ents)
	}
}

func TestFindSameContent(t *testing.T) {
	dir := t.TempDir()
	big := make([]byte, 200000)
	for i := range big {
		big[i] = byte(i * 7)
	}
	a, b, c := filepath.Join(dir, "a"), filepath.Join(dir, "b"), filepath.Join(dir, "c")
	_ = os.WriteFile(a, big, 0o644)
	_ = os.WriteFile(b, big, 0o644)
	big[150000]++
	_ = os.WriteFile(c, big, 0o644)
	if same, err := findSameContent(a, b); err != nil || !same {
		t.Fatalf("equal files: %v %v", same, err)
	}
	if same, err := findSameContent(a, c); err != nil || same {
		t.Fatalf("different files: %v %v", same, err)
	}
	empty1, empty2 := filepath.Join(dir, "e1"), filepath.Join(dir, "e2")
	_ = os.WriteFile(empty1, nil, 0o644)
	_ = os.WriteFile(empty2, nil, 0o644)
	if same, err := findSameContent(empty1, empty2); err != nil || !same {
		t.Fatalf("empty files: %v %v", same, err)
	}
}

func TestFindMissing(t *testing.T) {
	dir := t.TempDir()
	here := filepath.Join(dir, "here")
	findWriteFile(t, here, "x")
	gone := filepath.Join(dir, "gone")
	got := newTestFind(t).Missing([]string{here, gone, "sftp://h/x"})
	if len(got) != 1 || got[0] != gone {
		t.Fatalf("Missing = %v", got)
	}
}
