// These tests cover packing a single file to .gz when its name has
// characters outside Latin-1 (issue #91). The gzip header can only hold
// Latin-1, so the pack must still succeed with the name left out of the
// header, and the data must come back intact.
package tarfmt

import (
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestGzipHeaderNameLatin1(t *testing.T) {
	cases := map[string]string{
		"plain.txt":  "plain.txt",
		"café.txt":   "café.txt",
		"файл.txt":   "",
		"日本.txt":     "",
		"a\x00b":     "",
		"emoji😀.txt": "",
	}
	for in, want := range cases {
		if got := gzipHeaderName(in); got != want {
			t.Errorf("gzipHeaderName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCreateGzNonLatinName(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"файл.txt", "café.txt"} {
		src := filepath.Join(dir, name)
		if err := os.WriteFile(src, []byte("hello "+name), 0o644); err != nil {
			t.Fatal(err)
		}
		arc := src + ".gz"
		if err := Create(context.Background(), arc, []string{src}, CreateOptions{}); err != nil {
			t.Fatalf("packing %q: %v", name, err)
		}
		f, err := os.Open(arc)
		if err != nil {
			t.Fatal(err)
		}
		zr, err := gzip.NewReader(f)
		if err != nil {
			f.Close()
			t.Fatal(err)
		}
		data, err := io.ReadAll(zr)
		f.Close()
		if err != nil || string(data) != "hello "+name {
			t.Fatalf("%q: got %q, %v", name, data, err)
		}
		if want := gzipHeaderName(name); zr.Name != want {
			t.Errorf("%q: header name %q, want %q", name, zr.Name, want)
		}
	}
}
