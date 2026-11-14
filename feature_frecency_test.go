package main

// Tests of the frecency jump's Go side: the app's own frequently used folders and a zoxide database (a test file named
// by MORMICOMMANDER_ZOXIDE_DB, never the user's) come back as one ranked list; the file is read again only when it
// changes; a missing database is no problem and a broken one gives a plain reason.

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeZoxide(t *testing.T, path string, entries map[string]float64) {
	t.Helper()
	var b bytes.Buffer
	w := func(v any) { _ = binary.Write(&b, binary.LittleEndian, v) }
	w(uint32(3))
	w(uint64(len(entries)))
	for p, r := range entries {
		w(uint64(len(p)))
		b.WriteString(p)
		w(r)
		w(uint64(time.Now().Unix()))
	}
	if err := os.WriteFile(path, b.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestFrecencyList(t *testing.T) {
	th, _ := thTest(t)
	old := thInstance
	thInstance = th
	t.Cleanup(func() { thInstance = old })
	for _, p := range []string{"/app/one", "/app/one", "/app/two"} {
		th.Visit(p)
	}
	db := filepath.Join(t.TempDir(), "db.zo")
	t.Setenv("MORMICOMMANDER_ZOXIDE_DB", db)
	f := &Frecency{app: &App{}}

	// no zoxide database: only the app's folders, and no problem
	l := f.List(true, 0)
	if l.Zoxide != "" || l.Problem != "" || l.Imported != 0 || len(l.Items) != 2 || l.Items[0].Path != "/app/one" {
		t.Fatalf("without zoxide: %+v", l)
	}

	writeZoxide(t, db, map[string]float64{"/zox/top": 100, "/app/two": 1})
	l = f.List(true, 0)
	if l.Zoxide != db || l.Problem != "" || len(l.Items) != 3 || l.Items[0].Path != "/zox/top" || l.Imported != 2 {
		t.Fatalf("with zoxide: %+v", l)
	}
	for _, it := range l.Items {
		if it.Path == "/app/two" && it.Source != "both" {
			t.Fatalf("a folder in both lists: %+v", it)
		}
	}
	// asked without zoxide, its folders stay out
	if l := f.List(false, 0); len(l.Items) != 2 || l.Zoxide != "" {
		t.Fatalf("zoxide off: %+v", l)
	}
	if l := f.List(true, 1); len(l.Items) != 1 {
		t.Fatalf("max 1: %+v", l)
	}

	// a changed file is read again; a broken one gives a reason and the app's folders still come
	writeZoxide(t, db, map[string]float64{"/zox/new": 5})
	later := time.Now().Add(2 * time.Second)
	os.Chtimes(db, later, later)
	if l := f.List(true, 0); l.Items[0].Path != "/zox/new" {
		t.Fatalf("after a change: %+v", l)
	}
	os.WriteFile(db, []byte{3, 0, 0, 0, 9}, 0o600)
	os.Chtimes(db, later.Add(time.Second), later.Add(time.Second))
	l = f.List(true, 0)
	if l.Problem == "" || !strings.Contains(l.Problem, db) || len(l.Items) != 2 {
		t.Fatalf("a broken database: %+v", l)
	}

	// "none" reads no database at all
	t.Setenv("MORMICOMMANDER_ZOXIDE_DB", "none")
	if zoxidePath() != "" {
		t.Fatal(`"none" still names a database`)
	}
}

func TestFrecencyKey(t *testing.T) {
	if frecencyKey("/a/b/") != frecencyKey("/a/b") || frecencyKey("/") != "/" {
		t.Fatal("trailing separators make a different folder")
	}
}
