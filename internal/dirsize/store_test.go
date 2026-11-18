package dirsize

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreSaveLoad(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "sub", "foldersizes.bin")
	s := OpenStore(file)
	if s.Len() != 0 {
		t.Fatal("a missing file must give an empty store")
	}
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "a", "b")
	s.Put(Remembered{Path: a, Bytes: 100, Allocated: 4096, When: 10})
	s.Put(Remembered{Path: b + string(filepath.Separator), Bytes: 40})
	s.Put(Remembered{Path: "", Bytes: 1})
	s.Put(Remembered{Path: "x", Bytes: -1})
	s.PutResult(Result{Path: filepath.Join(dir, "c"), IsDir: true, Bytes: 7, Complete: false}) // cancelled: ignored
	s.PutResult(Result{Path: filepath.Join(dir, "d"), IsDir: true, Bytes: 9, Complete: true})
	if s.Len() != 3 {
		t.Fatalf("len %d", s.Len())
	}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	s2 := OpenStore(file)
	if r, ok := s2.Get(a); !ok || r.Bytes != 100 || r.Allocated != 4096 || r.When != 10 {
		t.Errorf("a: %+v %v", r, ok)
	}
	if r, ok := s2.Get(b); !ok || r.Bytes != 40 || r.When == 0 {
		t.Errorf("b (trailing separator cleaned, When set): %+v %v", r, ok)
	}
	// Invalidate drops the path, its ancestors and its descendants
	s2.Invalidate(b)
	if s2.Len() != 1 {
		t.Errorf("after invalidate: %d", s2.Len())
	}
	if err := s2.Clear(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Errorf("Clear must remove the file: %v", err)
	}
}

func TestStoreBadFileAndTrim(t *testing.T) {
	file := filepath.Join(t.TempDir(), "foldersizes.bin")
	os.WriteFile(file, []byte("not gzip"), 0o600)
	s := OpenStore(file)
	if s.Len() != 0 {
		t.Fatal("an unreadable file must give an empty store")
	}
	s.Max = 2
	for i, n := range []string{"/x1", "/x2", "/x3"} {
		s.Put(Remembered{Path: n, Bytes: 1, When: int64(i + 1)})
	}
	if _, ok := s.Get("/x1"); ok || s.Len() != 2 {
		t.Errorf("the oldest must go: len %d", s.Len())
	}
	// nothing changed since the last save: Save writes nothing
	s.Save()
	st1, _ := os.Stat(file)
	s.Save()
	st2, _ := os.Stat(file)
	if !st1.ModTime().Equal(st2.ModTime()) {
		t.Error("an unchanged store was written again")
	}
}
