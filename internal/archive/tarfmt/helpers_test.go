// Shared helpers for the tarfmt tests: building tar archives in memory
// member by member (including hostile ones a normal tar program would never
// write), compressing them with every supported codec, and comparing folder
// trees. Everything is written under t.TempDir().
package tarfmt

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

type member struct {
	name  string
	body  string
	typ   byte
	link  string
	mode  int64
	mtime time.Time
}

var testTime = time.Date(2021, 3, 4, 5, 6, 7, 0, time.UTC)

func makeTar(t *testing.T, ms []member) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, m := range ms {
		h := &tar.Header{Name: m.name, Typeflag: m.typ, Linkname: m.link, Mode: m.mode, ModTime: m.mtime}
		if h.Typeflag == 0 {
			h.Typeflag = tar.TypeReg
		}
		if h.Mode == 0 {
			h.Mode = 0o644
			if h.Typeflag == tar.TypeDir {
				h.Mode = 0o755
			}
		}
		if h.ModTime.IsZero() {
			h.ModTime = testTime
		}
		if h.Typeflag == tar.TypeReg {
			h.Size = int64(len(m.body))
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Size > 0 {
			if _, err := tw.Write([]byte(m.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func compressBytes(t *testing.T, c Compression, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := compress(c, &buf, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func writeFile(t *testing.T, p string, data []byte) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func mustOpen(t *testing.T, p string) *Reader {
	t.Helper()
	r, err := Open(p)
	if err != nil {
		t.Fatalf("Open(%s): %v", p, err)
	}
	t.Cleanup(func() { r.Close() })
	return r
}

func needSymlinks(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need extra privileges on Windows")
	}
}

// listTree returns every path under dir (slash form) mapped to "dir",
// "link:<target>" or the file content.
func listTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.Walk(dir, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		switch {
		case fi.Mode()&os.ModeSymlink != 0:
			l, _ := os.Readlink(p)
			out[rel] = "link:" + filepath.ToSlash(l)
		case fi.IsDir():
			out[rel] = "dir"
		default:
			b, _ := os.ReadFile(p)
			out[rel] = string(b)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

var tarFormats = []Format{FormatTar, FormatTarGz, FormatTarBz2, FormatTarXz, FormatTarZst, FormatTarLzma}
var singleFormats = []Format{FormatGz, FormatBz2, FormatXz, FormatZst, FormatLzma}

// compressNamed makes a gzip file whose header stores name and testTime.
func compressNamed(t *testing.T, name, body string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	zw.Name = name
	zw.ModTime = testTime
	if _, err := zw.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
