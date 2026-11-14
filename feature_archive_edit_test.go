package main

// Tests for editing a file inside an archive (feature_archive_edit.go): the entry is unpacked to a temporary copy, a
// save of the copy asks once, "update" packs the copy back over the entry, "don't update" leaves the archive alone,
// a failed update offers the change again, a touch without new content asks nothing, and an archive that can't be
// changed still gives a copy with the reason. Everything runs in temporary folders with a router of its own; no
// editor is started.

import (
	"archive/zip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mormicommander/internal/vfs"
)

// arcEditZip writes a zip with the given files (name → content) at dst.
func arcEditZip(t *testing.T, dst string, files map[string]string) {
	t.Helper()
	f, err := os.Create(dst)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for n, c := range files {
		w, err := zw.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		io.WriteString(w, c)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
}

// arcEditRead reads an entry of a zip.
func arcEditRead(t *testing.T, arc, name string) string {
	t.Helper()
	zr, err := zip.OpenReader(arc)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name == name {
			rc, _ := f.Open()
			b, _ := io.ReadAll(rc)
			rc.Close()
			return string(b)
		}
	}
	t.Fatalf("%s has no %s", arc, name)
	return ""
}

func arcEditRouter(t *testing.T) *vfs.Router {
	t.Helper()
	t.Setenv("TMPDIR", t.TempDir()) // (the temporary copies go below the test's own folder)
	r := vfs.NewRouter()
	r.Archives.TempDir = t.TempDir()
	t.Cleanup(func() { r.Close() })
	return r
}

// arcEditSave writes new content to the copy with a clearly later time, like an editor saving it.
func arcEditSave(t *testing.T, p, content string, at time.Time) {
	t.Helper()
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	os.Chtimes(p, at, at)
}

func TestArchiveEditUpdateAndDecline(t *testing.T) {
	r := arcEditRouter(t)
	arc := filepath.Join(t.TempDir(), "a.zip")
	arcEditZip(t, arc, map[string]string{"notes.txt": "hello", "other.txt": "keep"})
	entry := filepath.Join(arc, "notes.txt")

	local, root, ro, err := arcEditPrepare(r, entry)
	if err != nil {
		t.Fatal(err)
	}
	if ro != "" {
		t.Fatalf("a zip counted as read-only: %s", ro)
	}
	if root != arc {
		t.Fatalf("archive = %q, want %q", root, arc)
	}
	if b, _ := os.ReadFile(local); string(b) != "hello" {
		t.Fatalf("copy has %q", b)
	}

	var asked []arcEditChange
	w := newArcEditWatcher(func(l, e string) error {
		_, err := r.Upload(context.Background(), l, r.Clean(e), vfs.TransferOptions{PreserveTime: true})
		return err
	}, func(c arcEditChange) { asked = append(asked, c) })
	id, err := w.add(r.Clean(entry), root, local)
	if err != nil {
		t.Fatal(err)
	}
	if got := w.scan(); len(got) != 0 {
		t.Fatalf("asked before any save: %+v", got)
	}

	base := time.Now().Add(time.Hour)
	arcEditSave(t, local, "hello, edited", base)
	got := w.scan()
	if len(got) != 1 || got[0].ID != id || got[0].Name != "notes.txt" || got[0].Archive != arc || len(asked) != 1 {
		t.Fatalf("scan after save: %+v (events %d)", got, len(asked))
	}
	if again := w.scan(); len(again) != 0 {
		t.Fatalf("asked twice about one save: %+v", again)
	}
	if err := w.answer(id, true); err != nil {
		t.Fatal(err)
	}
	if s := arcEditRead(t, arc, "notes.txt"); s != "hello, edited" {
		t.Fatalf("archive has %q after update", s)
	}
	if s := arcEditRead(t, arc, "other.txt"); s != "keep" {
		t.Fatalf("other entry changed: %q", s)
	}
	if again := w.scan(); len(again) != 0 {
		t.Fatalf("asked again after the update: %+v", again)
	}

	// a second save, declined: the archive keeps the first edit and nothing is asked until the next save
	arcEditSave(t, local, "second edit", base.Add(time.Minute))
	if got := w.scan(); len(got) != 1 {
		t.Fatalf("second save not noticed: %+v", got)
	}
	if err := w.answer(id, false); err != nil {
		t.Fatal(err)
	}
	if s := arcEditRead(t, arc, "notes.txt"); s != "hello, edited" {
		t.Fatalf("declined change reached the archive: %q", s)
	}
	if again := w.scan(); len(again) != 0 {
		t.Fatalf("asked again after declining: %+v", again)
	}

	// a touch (new time, same content) is not a change
	later := base.Add(2 * time.Minute)
	os.Chtimes(local, later, later)
	if got := w.scan(); len(got) != 0 {
		t.Fatalf("a touch asked: %+v", got)
	}
}

func TestArchiveEditFailedUpdateAsksAgain(t *testing.T) {
	dir := t.TempDir()
	local := filepath.Join(dir, "x.txt")
	os.WriteFile(local, []byte("a"), 0o644)
	fail := true
	w := newArcEditWatcher(func(string, string) error {
		if fail {
			return errors.New("disk full")
		}
		return nil
	}, nil)
	id, err := w.add("/arc.zip/x.txt", "/arc.zip", local)
	if err != nil {
		t.Fatal(err)
	}
	arcEditSave(t, local, "b", time.Now().Add(time.Hour))
	if got := w.scan(); len(got) != 1 {
		t.Fatalf("save not noticed: %+v", got)
	}
	if err := w.answer(id, true); err == nil || !strings.Contains(err.Error(), "disk full") || !strings.Contains(err.Error(), "arc.zip") {
		t.Fatalf("failed update: %v", err)
	}
	if got := w.scan(); len(got) != 1 {
		t.Fatalf("a failed update was not offered again: %+v", got)
	}
	fail = false
	if err := w.answer(id, true); err != nil {
		t.Fatal(err)
	}
	if got := w.scan(); len(got) != 0 {
		t.Fatalf("asked after a good update: %+v", got)
	}
	if err := w.answer(99, true); err == nil {
		t.Fatal("an unknown watch was answered")
	}
	w.remove(id)
	if err := w.answer(id, false); err == nil {
		t.Fatal("a removed watch was answered")
	}
}

func TestArchiveEditSameEntryReplacesWatch(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.txt"), filepath.Join(dir, "b.txt")
	os.WriteFile(a, []byte("1"), 0o644)
	os.WriteFile(b, []byte("1"), 0o644)
	w := newArcEditWatcher(func(string, string) error { return nil }, nil)
	w.add("/z.zip/f.txt", "/z.zip", a)
	id2, _ := w.add("/z.zip/f.txt", "/z.zip", b)
	if len(w.watches) != 1 || w.watches[id2] == nil {
		t.Fatalf("watches = %d", len(w.watches))
	}
}

func TestArchiveEditNestedIsReadOnly(t *testing.T) {
	r := arcEditRouter(t)
	tmp := t.TempDir()
	inner := filepath.Join(tmp, "inner.zip")
	arcEditZip(t, inner, map[string]string{"doc.txt": "nested"})
	b, _ := os.ReadFile(inner)
	outer := filepath.Join(tmp, "outer.zip")
	arcEditZip(t, outer, map[string]string{"inner.zip": string(b)})
	innerPath := filepath.Join(outer, "inner.zip")
	if _, err := r.List(context.Background(), innerPath); err != nil {
		t.Fatal(err)
	}
	local, _, ro, err := arcEditPrepare(r, filepath.Join(innerPath, "doc.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if ro == "" {
		t.Fatal("an archive inside another one was offered for updating")
	}
	if b, _ := os.ReadFile(local); string(b) != "nested" {
		t.Fatalf("copy has %q", b)
	}
}

func TestArchiveEditPrepareRefusesFolders(t *testing.T) {
	r := arcEditRouter(t)
	arc := filepath.Join(t.TempDir(), "a.zip")
	arcEditZip(t, arc, map[string]string{"dir/f.txt": "x"})
	if _, _, _, err := arcEditPrepare(r, filepath.Join(arc, "dir")); err == nil || !strings.Contains(err.Error(), "folder") {
		t.Fatalf("folder: %v", err)
	}
	if _, _, _, err := arcEditPrepare(r, filepath.Join(arc, "missing.txt")); err == nil {
		t.Fatal("a missing entry was unpacked")
	}
}
