// Tests for changing existing archives: delete, rename, make folder, add
// (copy and move into an archive, replacing members), keeping hard-linked
// data when its first name is deleted, and leaving the original untouched
// when a change is refused.
package tarfmt

import (
	"archive/tar"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	archive "mormicommander/internal/archive/arcapi"
)

var modifyBase = []member{
	{name: "keep.txt", body: "keep"},
	{name: "del.txt", body: "del"},
	{name: "dir/", typ: tar.TypeDir},
	{name: "dir/a.txt", body: "a"},
	{name: "dir/sub/b.txt", body: "b"},
	{name: "dir/ln", typ: tar.TypeSymlink, link: "a.txt"},
}

func newArchive(t *testing.T, f Format, ms []member) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "m"+f.Ext())
	writeFile(t, p, compressBytes(t, f.Compression(), makeTar(t, ms)))
	return p
}

func contentOf(t *testing.T, arc, name string) string {
	t.Helper()
	// Close the archive before returning: Windows cannot replace a file
	// that is still open, and callers modify the archive again afterwards.
	r, err := Open(arc)
	if err != nil {
		t.Fatalf("Open(%s): %v", arc, err)
	}
	defer r.Close()
	rc, err := r.OpenEntry(context.Background(), name)
	if err != nil {
		t.Fatalf("OpenEntry(%s): %v", name, err)
	}
	defer rc.Close()
	b, _ := io.ReadAll(rc)
	return string(b)
}

func TestModify(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "new.txt"), []byte("new"))
	writeFile(t, filepath.Join(src, "keep.txt"), []byte("replaced"))
	cases := []struct {
		name string
		ch   Changes
		want string
		body map[string]string
	}{
		{"delete file", Changes{Delete: []string{"del.txt"}},
			"dir/,dir/a.txt,dir/ln,dir/sub/,dir/sub/b.txt,keep.txt", nil},
		{"delete folder", Changes{Delete: []string{"dir/"}},
			"del.txt,keep.txt", nil},
		{"rename file", Changes{Rename: []Rename{{"del.txt", "dir/moved.txt"}}},
			"dir/,dir/a.txt,dir/ln,dir/moved.txt,dir/sub/,dir/sub/b.txt,keep.txt", map[string]string{"dir/moved.txt": "del"}},
		{"rename folder", Changes{Rename: []Rename{{"dir", "other"}}},
			"del.txt,keep.txt,other/,other/a.txt,other/ln,other/sub/,other/sub/b.txt", map[string]string{"other/sub/b.txt": "b"}},
		{"rename to itself", Changes{Rename: []Rename{{"del.txt", "./del.txt"}}},
			"del.txt,dir/,dir/a.txt,dir/ln,dir/sub/,dir/sub/b.txt,keep.txt", nil},
		{"mkdir", Changes{Mkdir: []string{"fresh/inner", "dir"}},
			"del.txt,dir/,dir/a.txt,dir/ln,dir/sub/,dir/sub/b.txt,fresh/,fresh/inner/,keep.txt", nil},
		{"add into folder", Changes{Add: []string{filepath.Join(src, "new.txt")}, AddOptions: CreateOptions{Prefix: "dir/sub"}},
			"del.txt,dir/,dir/a.txt,dir/ln,dir/sub/,dir/sub/b.txt,dir/sub/new.txt,keep.txt", map[string]string{"dir/sub/new.txt": "new"}},
		{"add replaces", Changes{Add: []string{filepath.Join(src, "keep.txt")}},
			"del.txt,dir/,dir/a.txt,dir/ln,dir/sub/,dir/sub/b.txt,keep.txt", map[string]string{"keep.txt": "replaced"}},
		{"combined", Changes{Delete: []string{"dir/sub"}, Rename: []Rename{{"keep.txt", "kept.txt"}}, Add: []string{filepath.Join(src, "new.txt")}},
			"del.txt,dir/,dir/a.txt,dir/ln,kept.txt,new.txt", map[string]string{"kept.txt": "keep", "new.txt": "new"}},
	}
	for _, f := range []Format{FormatTar, FormatTarGz, FormatTarXz, FormatTarZst, FormatTarBz2} {
		for _, c := range cases {
			t.Run(f.String()+"/"+c.name, func(t *testing.T) {
				arc := newArchive(t, f, modifyBase)
				os.Chmod(arc, 0o640)
				if err := Modify(context.Background(), arc, c.ch); err != nil {
					t.Fatal(err)
				}
				if got := strings.Join(archiveNames(t, arc), ","); got != c.want {
					t.Errorf("got  %s\nwant %s", got, c.want)
				}
				for n, b := range c.body {
					if got := contentOf(t, arc, n); got != b {
						t.Errorf("%s = %q, want %q", n, got, b)
					}
				}
				r := mustOpen(t, arc)
				if r.Format() != f {
					t.Errorf("format changed to %v", r.Format())
				}
				if err := r.Test(context.Background(), nil); err != nil {
					t.Error(err)
				}
				if fi, _ := os.Stat(arc); fi.Mode().Perm() != 0o640 && runtime.GOOS != "windows" {
					t.Errorf("archive mode %v", fi.Mode().Perm())
				}
				des, _ := os.ReadDir(filepath.Dir(arc))
				if len(des) != 1 {
					t.Errorf("leftover files: %v", des)
				}
			})
		}
	}
}

func TestModifyRefusals(t *testing.T) {
	cases := []struct {
		name string
		ch   Changes
		is   error
	}{
		{"delete missing", Changes{Delete: []string{"nope"}}, archive.ErrNotFound},
		{"delete root", Changes{Delete: []string{"/"}}, nil},
		{"rename missing", Changes{Rename: []Rename{{"nope", "x"}}}, archive.ErrNotFound},
		{"rename onto existing", Changes{Rename: []Rename{{"del.txt", "keep.txt"}}}, fs.ErrExist},
		{"rename into itself", Changes{Rename: []Rename{{"dir", "dir/sub/x"}}}, nil},
		{"rename escaping", Changes{Rename: []Rename{{"del.txt", "../x"}}}, nil},
		{"mkdir escaping", Changes{Mkdir: []string{"../x"}}, nil},
		{"add missing", Changes{Add: []string{"/definitely/not/here"}}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			arc := newArchive(t, FormatTarGz, modifyBase)
			before, _ := os.ReadFile(arc)
			err := Modify(context.Background(), arc, c.ch)
			if err == nil {
				t.Fatal("change accepted")
			}
			if c.is != nil && !errors.Is(err, c.is) {
				t.Errorf("err = %v, want %v", err, c.is)
			}
			after, _ := os.ReadFile(arc)
			if string(before) != string(after) {
				t.Error("archive changed by a refused modification")
			}
			des, _ := os.ReadDir(filepath.Dir(arc))
			if len(des) != 1 {
				t.Errorf("leftover files: %v", des)
			}
		})
	}
}

func TestModifySingleFileUnsupported(t *testing.T) {
	p := writeFile(t, filepath.Join(t.TempDir(), "a.gz"), compressBytes(t, CompGzip, []byte("x")))
	if err := Modify(context.Background(), p, Changes{Mkdir: []string{"d"}}); !errors.Is(err, archive.ErrNotSupported) {
		t.Errorf("err = %v", err)
	}
}

func TestModifyKeepsHardLinkData(t *testing.T) {
	arc := newArchive(t, FormatTar, []member{
		{name: "orig", body: "shared"},
		{name: "l1", typ: tar.TypeLink, link: "orig"},
		{name: "l2", typ: tar.TypeLink, link: "orig"},
	})
	if err := Modify(context.Background(), arc, Changes{Delete: []string{"orig"}}); err != nil {
		t.Fatal(err)
	}
	if got := contentOf(t, arc, "l1"); got != "shared" {
		t.Errorf("l1 = %q", got)
	}
	if got := contentOf(t, arc, "l2"); got != "shared" {
		t.Errorf("l2 = %q", got)
	}
	h, err := mustOpen(t, arc).Header(context.Background(), "l2")
	if err != nil || h.Typeflag != tar.TypeLink || h.Linkname != "l1" {
		t.Errorf("l2 header = %+v, %v", h, err)
	}
	des, _ := os.ReadDir(filepath.Dir(arc))
	if len(des) != 1 {
		t.Errorf("leftover files: %v", des)
	}
}

func TestModifyRenameUpdatesHardLinks(t *testing.T) {
	arc := newArchive(t, FormatTar, []member{
		{name: "d/orig", body: "data"},
		{name: "l", typ: tar.TypeLink, link: "d/orig"},
	})
	if err := Modify(context.Background(), arc, Changes{Rename: []Rename{{"d", "e"}}}); err != nil {
		t.Fatal(err)
	}
	if got := contentOf(t, arc, "l"); got != "data" {
		t.Errorf("l = %q", got)
	}
}

func TestModifyMoveIntoArchive(t *testing.T) {
	src := t.TempDir()
	f := writeFile(t, filepath.Join(src, "moveme.txt"), []byte("m"))
	arc := newArchive(t, FormatTarGz, modifyBase)
	if err := Modify(context.Background(), arc, Changes{Add: []string{f}, AddOptions: CreateOptions{RemoveSources: true}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(f); err == nil {
		t.Error("source not removed")
	}
	if contentOf(t, arc, "moveme.txt") != "m" {
		t.Error("not added")
	}
}

func TestModifyPreservesLongNamesAndPAX(t *testing.T) {
	long := strings.Repeat("very-long-folder-name/", 12) + "file-with-ünïcödé.txt"
	arc := newArchive(t, FormatTarXz, []member{{name: long, body: "L"}, {name: "x", body: "x"}})
	if err := Modify(context.Background(), arc, Changes{Delete: []string{"x"}}); err != nil {
		t.Fatal(err)
	}
	if contentOf(t, arc, long) != "L" {
		t.Error("long unicode name lost")
	}
	if err := Modify(context.Background(), arc, Changes{Rename: []Rename{{long, "short.txt"}}}); err != nil {
		t.Fatal(err)
	}
	if contentOf(t, arc, "short.txt") != "L" {
		t.Error("rename of PAX-named member failed")
	}
}

func TestModifyCancelLeavesOriginal(t *testing.T) {
	arc := newArchive(t, FormatTarGz, modifyBase)
	before, _ := os.ReadFile(arc)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Modify(ctx, arc, Changes{Delete: []string{"del.txt"}}); err == nil {
		t.Error("cancelled modify succeeded")
	}
	after, _ := os.ReadFile(arc)
	if string(before) != string(after) {
		t.Error("archive changed")
	}
}
