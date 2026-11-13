// Tests for opening, listing, browsing, streaming and testing archives in
// every supported format, including mis-named files, duplicate members and
// damaged archives.
package tarfmt

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	archive "mormicommander/internal/archive/arcapi"
)

var sampleMembers = []member{
	{name: "top.txt", body: "top"},
	{name: "./dir/", typ: tar.TypeDir, mode: 0o750},
	{name: "dir/a.txt", body: "alpha", mode: 0o600},
	{name: "dir/sub/deep.txt", body: "deep"}, // dir/sub is implied
	{name: "dir/link", typ: tar.TypeSymlink, link: "a.txt"},
	{name: "dir/hard", typ: tar.TypeLink, link: "dir/a.txt"},
	{name: "empty/", typ: tar.TypeDir},
}

func TestOpenAllTarFormats(t *testing.T) {
	plain := makeTar(t, sampleMembers)
	for _, f := range tarFormats {
		t.Run(f.String(), func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "x"+f.Ext())
			writeFile(t, p, compressBytes(t, f.Compression(), plain))
			r := mustOpen(t, p)
			if r.Format() != f {
				t.Fatalf("format %v, want %v", r.Format(), f)
			}
			es, err := r.Entries(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			got := map[string]archive.Entry{}
			for _, e := range es {
				got[e.Path] = e
			}
			for _, want := range []string{"top.txt", "dir", "dir/a.txt", "dir/sub", "dir/sub/deep.txt", "dir/link", "dir/hard", "empty"} {
				if _, ok := got[want]; !ok {
					t.Errorf("missing entry %q (have %v)", want, es)
				}
			}
			if len(got) != 8 {
				t.Errorf("%d entries, want 8", len(got))
			}
			if e := got["dir/a.txt"]; e.Size != 5 || e.IsDir || e.Mode.Perm() != 0o600 || !e.ModTime.Equal(testTime) {
				t.Errorf("dir/a.txt = %+v", e)
			}
			if e := got["dir"]; !e.IsDir || !e.Mode.IsDir() || e.Mode.Perm() != 0o750 {
				t.Errorf("dir = %+v", e)
			}
			if e := got["dir/sub"]; !e.IsDir {
				t.Errorf("implied dir/sub = %+v", e)
			}
			if e := got["dir/link"]; e.Mode.Type() != fs.ModeSymlink {
				t.Errorf("dir/link mode %v", e.Mode)
			}

			root, err := r.ReadDir(context.Background(), "")
			if err != nil {
				t.Fatal(err)
			}
			if names := entryNames(root); names != "dir,empty,top.txt" {
				t.Errorf("root = %s", names)
			}
			sub, err := r.ReadDir(context.Background(), "/dir/")
			if err != nil {
				t.Fatal(err)
			}
			if names := entryNames(sub); names != "dir/a.txt,dir/hard,dir/link,dir/sub" {
				t.Errorf("dir = %s", names)
			}
			if _, err := r.ReadDir(context.Background(), "top.txt"); err == nil {
				t.Error("ReadDir on a file succeeded")
			}
			if _, err := r.ReadDir(context.Background(), "nope"); !errors.Is(err, archive.ErrNotFound) {
				t.Errorf("ReadDir(nope) = %v", err)
			}

			for name, want := range map[string]string{"dir/sub/deep.txt": "deep", "dir/hard": "alpha", "top.txt": "top"} {
				rc, err := r.OpenEntry(context.Background(), name)
				if err != nil {
					t.Fatalf("OpenEntry(%s): %v", name, err)
				}
				b, _ := io.ReadAll(rc)
				rc.Close()
				if string(b) != want {
					t.Errorf("OpenEntry(%s) = %q", name, b)
				}
			}
			if _, err := r.OpenEntry(context.Background(), "dir"); err == nil {
				t.Error("OpenEntry on a folder succeeded")
			}
			if _, err := r.OpenEntry(context.Background(), "dir/link"); err == nil {
				t.Error("OpenEntry on a symlink succeeded")
			}
			h, err := r.Header(context.Background(), "dir/link")
			if err != nil || h.Linkname != "a.txt" {
				t.Errorf("Header = %+v, %v", h, err)
			}
			if _, err := r.Header(context.Background(), "dir/sub"); err == nil {
				t.Error("implied folder has a header")
			}
			if err := r.Test(context.Background(), nil); err != nil {
				t.Errorf("Test: %v", err)
			}
		})
	}
}

func entryNames(es []archive.Entry) string {
	var s []string
	for _, e := range es {
		s = append(s, e.Path)
	}
	return strings.Join(s, ",")
}

func TestDetectByContent(t *testing.T) {
	plain := makeTar(t, []member{{name: "a", body: "x"}})
	cases := []struct {
		file string
		data []byte
		want Format
	}{
		{"misnamed.bin", compressBytes(t, CompGzip, plain), FormatTarGz},
		{"tar-inside.gz", compressBytes(t, CompGzip, plain), FormatTarGz},
		{"x.tar.gz", compressBytes(t, CompGzip, []byte("not a tar at all")), FormatGz},
		{"x.tgz", compressBytes(t, CompXz, plain), FormatTarXz},
		{"raw.dat", plain, FormatTar},
		{"x.lzma", compressBytes(t, CompLzma, []byte("hello")), FormatLzma},
		{"x.tar.lzma", compressBytes(t, CompLzma, plain), FormatTarLzma},
		{"empty.tar", make([]byte, 1024), FormatTar},
		{"empty.tar", nil, FormatTar},
		{"empty.tgz", compressBytes(t, CompGzip, make([]byte, 1024)), FormatTarGz},
		{"empty.tar.zst", compressBytes(t, CompZstd, nil), FormatTarZst},
		{"empty.gz", compressBytes(t, CompGzip, nil), FormatGz},
	}
	for _, c := range cases {
		p := writeFile(t, filepath.Join(t.TempDir(), c.file), c.data)
		r, err := Open(p)
		if err != nil {
			t.Errorf("%s: %v", c.file, err)
			continue
		}
		if r.Format() != c.want {
			t.Errorf("%s: format %v, want %v", c.file, r.Format(), c.want)
		}
		if _, err := r.Entries(context.Background()); err != nil {
			t.Errorf("%s: Entries: %v", c.file, err)
		}
		r.Close()
	}
}

func TestNotAnArchive(t *testing.T) {
	dir := t.TempDir()
	for name, data := range map[string][]byte{
		"a.txt":    []byte("hello world"),
		"a.tar":    []byte("hello world, definitely not tar"),
		"bad.gz":   {0x1f, 0x8b, 0, 0},
		"bad.lzma": []byte("zz"),
	} {
		if _, err := Open(writeFile(t, filepath.Join(dir, name), data)); err == nil {
			t.Errorf("%s opened", name)
		}
	}
	if _, err := Open(dir); !errors.Is(err, ErrFormat) {
		t.Errorf("Open(dir) = %v", err)
	}
	if _, err := Open(filepath.Join(dir, "missing.tar")); err == nil {
		t.Error("missing file opened")
	}
}

func TestSingleFileFormats(t *testing.T) {
	body := strings.Repeat("single file content ", 100)
	for _, f := range singleFormats {
		t.Run(f.String(), func(t *testing.T) {
			p := writeFile(t, filepath.Join(t.TempDir(), "notes.txt"+f.Ext()), compressBytes(t, f.Compression(), []byte(body)))
			r := mustOpen(t, p)
			if r.Format() != f {
				t.Fatalf("format %v", r.Format())
			}
			es, err := r.Entries(context.Background())
			if err != nil || len(es) != 1 || es[0].Path != "notes.txt" {
				t.Fatalf("Entries = %+v, %v", es, err)
			}
			if f == FormatGz && es[0].Size != int64(len(body)) {
				t.Errorf("gz size %d", es[0].Size)
			}
			if f != FormatGz && es[0].Size != -1 {
				t.Errorf("size %d, want -1 (unknown)", es[0].Size)
			}
			rc, err := r.OpenEntry(context.Background(), "notes.txt")
			if err != nil {
				t.Fatal(err)
			}
			b, _ := io.ReadAll(rc)
			rc.Close()
			if string(b) != body {
				t.Error("content mismatch")
			}
			dest := t.TempDir()
			if err := r.Extract(context.Background(), nil, dest, archive.ExtractOptions{}); err != nil {
				t.Fatal(err)
			}
			if readFile(t, filepath.Join(dest, "notes.txt")) != body {
				t.Error("extracted content mismatch")
			}
			if err := r.Test(context.Background(), nil); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestGzipHeaderName(t *testing.T) {
	data := compressNamed(t, "../../real-name.txt", "x")
	p := writeFile(t, filepath.Join(t.TempDir(), "renamed.gz"), data)
	r := mustOpen(t, p)
	es, _ := r.Entries(context.Background())
	if es[0].Path != "real-name.txt" || !es[0].ModTime.Equal(testTime) {
		t.Errorf("entry = %+v", es[0])
	}
}

func TestDuplicateMembersLaterWins(t *testing.T) {
	p := writeFile(t, filepath.Join(t.TempDir(), "d.tar"), makeTar(t, []member{
		{name: "f", body: "old"}, {name: "g", body: "g"}, {name: "./f", body: "newer"},
	}))
	r := mustOpen(t, p)
	es, _ := r.Entries(context.Background())
	if len(es) != 2 {
		t.Fatalf("entries %v", es)
	}
	e, _ := r.Stat(context.Background(), "f")
	if e.Size != 5 {
		t.Errorf("size %d, want 5", e.Size)
	}
	rc, err := r.OpenEntry(context.Background(), "f")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	rc.Close()
	if string(b) != "newer" {
		t.Errorf("OpenEntry = %q", b)
	}
	dest := t.TempDir()
	if err := r.Extract(context.Background(), nil, dest, archive.ExtractOptions{}); err != nil {
		t.Fatal(err)
	}
	if readFile(t, filepath.Join(dest, "f")) != "newer" {
		t.Error("extract did not keep the later copy")
	}
}

func TestTestDetectsDamage(t *testing.T) {
	plain := makeTar(t, []member{{name: "a", body: strings.Repeat("abcdefgh", 4000)}})
	for _, f := range []Format{FormatTarGz, FormatTarBz2, FormatTarXz, FormatTarZst, FormatTar} {
		t.Run(f.String(), func(t *testing.T) {
			data := compressBytes(t, f.Compression(), plain)
			dir := t.TempDir()
			trunc := writeFile(t, filepath.Join(dir, "trunc"+f.Ext()), data[:len(data)*2/3])
			if err := Test(context.Background(), trunc, nil); err == nil {
				t.Error("truncated archive passed the test")
			}
			if f == FormatTar {
				return
			}
			bad := append([]byte(nil), data...)
			bad[len(bad)/2] ^= 0xff
			badP := writeFile(t, filepath.Join(dir, "bad"+f.Ext()), bad)
			if err := Test(context.Background(), badP, nil); err == nil {
				t.Error("corrupted archive passed the test")
			}
		})
	}
}

func TestTestProgressAndCancel(t *testing.T) {
	plain := makeTar(t, []member{{name: "a", body: strings.Repeat("x", 1<<20)}, {name: "b", body: "y"}})
	p := writeFile(t, filepath.Join(t.TempDir(), "p.tgz"), compressBytes(t, CompGzip, plain))
	var last archive.Progress
	if err := Test(context.Background(), p, func(pr archive.Progress) { last = pr }); err != nil {
		t.Fatal(err)
	}
	if last.Files != 2 || last.Bytes < 1<<20 || last.ArchiveRead != last.ArchiveSize || last.ArchiveSize == 0 {
		t.Errorf("last progress %+v", last)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Test(ctx, p, nil); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled Test = %v", err)
	}
	r := mustOpen(t, p)
	if _, err := r.Entries(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled Entries = %v", err)
	}
	// A cancelled listing must not be cached as empty.
	if es, err := r.Entries(context.Background()); err != nil || len(es) != 2 {
		t.Errorf("Entries after cancel = %v, %v", es, err)
	}
}

func TestNewReaderFromMemory(t *testing.T) {
	// Nested archives are opened from memory or a temp copy.
	inner := compressBytes(t, CompGzip, makeTar(t, []member{{name: "in.txt", body: "nested"}}))
	r, err := NewReader(bytes.NewReader(inner), int64(len(inner)), "inner.tgz", testTime)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if r.Size() != int64(len(inner)) {
		t.Error("Size wrong")
	}
	rc, err := r.OpenEntry(context.Background(), "in.txt")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	rc.Close()
	if string(b) != "nested" {
		t.Errorf("got %q", b)
	}
}

func TestUnsafeNamesHiddenFromReadDir(t *testing.T) {
	p := writeFile(t, filepath.Join(t.TempDir(), "u.tar"), makeTar(t, []member{
		{name: "../evil", body: "x"}, {name: "ok", body: "y"},
	}))
	r := mustOpen(t, p)
	es, _ := r.Entries(context.Background())
	if len(es) != 2 {
		t.Errorf("Entries should list everything, got %v", es)
	}
	root, _ := r.ReadDir(context.Background(), "")
	if entryNames(root) != "ok" {
		t.Errorf("ReadDir = %s", entryNames(root))
	}
}
