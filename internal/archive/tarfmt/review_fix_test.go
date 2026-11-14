// Tests for the engine review findings of issue #91 that concern the tar
// engine: "move to archive" with followed symbolic links must never delete
// the files a link points to, NoPaths must not silently fold two files into
// one name before their originals are deleted, a file that grows while it
// is being packed is stored whole (and not deleted when it changed), and a
// damaged gzip trailer is reported when unpacking a .tar.gz. Everything runs
// in t.TempDir().
package tarfmt

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	archive "mormicommander/internal/archive/arcapi"
)

// A symlink inside the selection that points at a folder outside it: with
// FollowSymlinks and "move to archive" the link goes, the outside folder and
// its files stay.
func TestMoveToArchiveFollowedLinkKeepsTarget(t *testing.T) {
	needSymlinks(t)
	base := t.TempDir()
	outside := filepath.Join(base, "outside")
	precious := writeFile(t, filepath.Join(outside, "precious.txt"), []byte("do not lose me"))
	lonely := writeFile(t, filepath.Join(base, "lone.txt"), []byte("single"))
	sel := filepath.Join(base, "sel")
	own := writeFile(t, filepath.Join(sel, "own.txt"), []byte("mine"))
	if err := os.Symlink(outside, filepath.Join(sel, "dirlink")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(lonely, filepath.Join(sel, "filelink")); err != nil {
		t.Fatal(err)
	}
	arc := filepath.Join(base, "out.tar.gz")
	if err := Create(context.Background(), arc, []string{sel}, CreateOptions{FollowSymlinks: true, RemoveSources: true}); err != nil {
		t.Fatal(err)
	}
	if readFile(t, precious) != "do not lose me" {
		t.Fatal("file behind a followed folder link was changed")
	}
	if readFile(t, lonely) != "single" {
		t.Fatal("file behind a followed file link was deleted")
	}
	if _, err := os.Lstat(own); err == nil {
		t.Error("packed file inside the selection was not removed")
	}
	for _, l := range []string{"dirlink", "filelink"} {
		if _, err := os.Lstat(filepath.Join(sel, l)); err == nil {
			t.Errorf("link %s was not removed", l)
		}
	}
	// The archive holds the followed content.
	dest := t.TempDir()
	if err := mustOpen(t, arc).Extract(context.Background(), nil, dest, archive.ExtractOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(dest, "sel", "dirlink", "precious.txt")); got != "do not lose me" {
		t.Errorf("archived copy = %q", got)
	}
}

// NoPaths with two files of the same name: "move to archive" refuses before
// anything is written or deleted. Without moving, the old behaviour stays.
func TestMoveToArchiveNoPathsDuplicateRefused(t *testing.T) {
	base := t.TempDir()
	a := writeFile(t, filepath.Join(base, "a", "same.txt"), []byte("first"))
	b := writeFile(t, filepath.Join(base, "b", "same.txt"), []byte("second"))
	arc := filepath.Join(base, "flat.tar")
	err := Create(context.Background(), arc, []string{filepath.Join(base, "a"), filepath.Join(base, "b")},
		CreateOptions{NoPaths: true, RemoveSources: true})
	if err == nil || !strings.Contains(err.Error(), "same.txt") {
		t.Fatalf("err = %v, want a duplicate-name error", err)
	}
	if readFile(t, a) != "first" || readFile(t, b) != "second" {
		t.Fatal("sources touched")
	}
	if _, err := os.Stat(arc); err == nil {
		t.Error("archive written although the pack was refused")
	}
	if err := Create(context.Background(), arc, []string{filepath.Join(base, "a"), filepath.Join(base, "b")},
		CreateOptions{NoPaths: true}); err != nil {
		t.Fatalf("plain NoPaths pack: %v", err)
	}
}

// The header size comes from the open file, so data appended after the
// folder scan is stored too instead of being cut off.
func TestPackerStoresFileThatGrewAfterScan(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, filepath.Join(dir, "log.txt"), []byte("start\n"))
	items, err := collect(context.Background(), []string{p}, CreateOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("more lines\n")
	f.Close()
	var buf bytes.Buffer
	if err := writeTar(context.Background(), &buf, FormatTar, 0, items, progressFn(nil, 0, nil)); err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(&buf)
	if _, err := tr.Next(); err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(tr)
	if string(body) != "start\nmore lines\n" {
		t.Errorf("stored %q", body)
	}
	if s := items[0].packed; s == nil || !s.done || s.grew || s.size != int64(len(body)) {
		t.Errorf("stamp = %+v", s)
	}
}

// A file that changed after it was stored (or kept growing) is kept by
// "move to archive" and reported; an unchanged one is removed.
func TestRemoveItemsKeepsChangedFiles(t *testing.T) {
	dir := t.TempDir()
	same := writeFile(t, filepath.Join(dir, "same.txt"), []byte("x"))
	changed := writeFile(t, filepath.Join(dir, "changed.txt"), []byte("y"))
	grew := writeFile(t, filepath.Join(dir, "grew.txt"), []byte("z"))
	items, err := collect(context.Background(), []string{same, changed, grew}, CreateOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		fi, _ := os.Stat(it.src)
		*it.packed = stamp{done: true, size: fi.Size(), mtime: fi.ModTime(), grew: it.src == grew}
	}
	if err := os.WriteFile(changed, []byte("changed later"), 0o644); err != nil {
		t.Fatal(err)
	}
	err = removeItems(items)
	if err == nil || !strings.Contains(err.Error(), "changed.txt") || !strings.Contains(err.Error(), "grew.txt") {
		t.Errorf("err = %v", err)
	}
	if _, err := os.Stat(same); err == nil {
		t.Error("unchanged file not removed")
	}
	if _, err := os.Stat(changed); err != nil {
		t.Error("changed file removed")
	}
	if _, err := os.Stat(grew); err != nil {
		t.Error("growing file removed")
	}
}

// A .tar.gz whose CRC32 trailer is damaged unpacks its members but the
// extraction reports the damage instead of claiming success.
func TestExtractTarGzChecksTrailer(t *testing.T) {
	data := compressBytes(t, CompGzip, makeTar(t, []member{{name: "a.txt", body: "hello"}}))
	good := filepath.Join(t.TempDir(), "good.tar.gz")
	writeFile(t, good, data)
	if err := mustOpen(t, good).Extract(context.Background(), nil, t.TempDir(), archive.ExtractOptions{}); err != nil {
		t.Fatalf("intact archive: %v", err)
	}
	bad := append([]byte(nil), data...)
	bad[len(bad)-8] ^= 0xff // first byte of the CRC32
	badPath := filepath.Join(t.TempDir(), "bad.tar.gz")
	writeFile(t, badPath, bad)
	err := mustOpen(t, badPath).Extract(context.Background(), nil, t.TempDir(), archive.ExtractOptions{})
	if err == nil || !strings.Contains(err.Error(), "damaged") {
		t.Errorf("err = %v, want a damaged-archive error", err)
	}
	// Junk after a valid gzip member is tolerated, as before.
	junk := append(append([]byte(nil), data...), []byte("trailing junk!")...)
	junkPath := filepath.Join(t.TempDir(), "junk.tar.gz")
	writeFile(t, junkPath, junk)
	if err := mustOpen(t, junkPath).Extract(context.Background(), nil, t.TempDir(), archive.ExtractOptions{}); err != nil {
		t.Errorf("archive with trailing junk: %v", err)
	}
}

// Stored permission bits are masked with the process umask, as tar does
// for a normal user: a world-writable member does not stay world-writable.
func TestExtractAppliesUmask(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no umask on Windows")
	}
	data := makeTar(t, []member{
		{name: "d/", typ: tar.TypeDir, mode: 0o777},
		{name: "d/open.sh", body: "#!/bin/sh\n", mode: 0o777},
	})
	arc := filepath.Join(t.TempDir(), "m.tar")
	writeFile(t, arc, data)
	dest := t.TempDir()
	if err := mustOpen(t, arc).Extract(context.Background(), nil, dest, archive.ExtractOptions{}); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"d", filepath.Join("d", "open.sh")} {
		st, err := os.Stat(filepath.Join(dest, p))
		if err != nil {
			t.Fatal(err)
		}
		if got, want := st.Mode().Perm(), archive.ApplyUmask(0o777); got != want {
			t.Errorf("%s: mode %o, want %o", p, got, want)
		}
	}
}
