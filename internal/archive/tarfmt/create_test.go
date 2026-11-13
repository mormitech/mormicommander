// Tests for packing: round trips through every format, the pack dialog's
// options, single-file packers, hard links, "move to archive", cancellation,
// and compatibility with the system's own tar program when one is present.
package tarfmt

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	archive "mormicommander/internal/archive/arcapi"
)

// makeSourceTree builds a small folder tree to pack and returns its root.
func makeSourceTree(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "proj")
	writeFile(t, filepath.Join(root, "a.txt"), []byte("alpha"))
	writeFile(t, filepath.Join(root, "sub", "b.txt"), []byte("beta"))
	writeFile(t, filepath.Join(root, "sub", "deeper", "c.txt"), []byte("gamma"))
	if err := os.Mkdir(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.Chmod(filepath.Join(root, "a.txt"), 0o751)
	mt := time.Date(2020, 5, 6, 7, 8, 9, 0, time.UTC)
	os.Chtimes(filepath.Join(root, "a.txt"), mt, mt)
	if runtime.GOOS != "windows" {
		if err := os.Symlink("a.txt", filepath.Join(root, "link")); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestCreateRoundTripAllTarFormats(t *testing.T) {
	src := makeSourceTree(t)
	for _, f := range tarFormats {
		for _, level := range []int{0, 1, 9} {
			t.Run(f.String()+"/"+string(rune('0'+level)), func(t *testing.T) {
				arc := filepath.Join(t.TempDir(), "out"+f.Ext())
				if err := Create(context.Background(), arc, []string{src}, CreateOptions{Level: level}); err != nil {
					t.Fatal(err)
				}
				r := mustOpen(t, arc)
				if r.Format() != f {
					t.Errorf("format %v", r.Format())
				}
				dest := t.TempDir()
				if err := r.Extract(context.Background(), nil, dest, archive.ExtractOptions{}); err != nil {
					t.Fatal(err)
				}
				got := listTree(t, filepath.Join(dest, "proj"))
				want := listTree(t, src)
				if len(got) != len(want) {
					t.Errorf("got %v, want %v", got, want)
				}
				for k, v := range want {
					if got[k] != v {
						t.Errorf("%s = %q, want %q", k, got[k], v)
					}
				}
				fi, _ := os.Stat(filepath.Join(dest, "proj", "a.txt"))
				if runtime.GOOS != "windows" && fi.Mode().Perm() != 0o751 {
					t.Errorf("mode %v", fi.Mode().Perm())
				}
				if !fi.ModTime().Equal(time.Date(2020, 5, 6, 7, 8, 9, 0, time.UTC)) {
					t.Errorf("mtime %v", fi.ModTime())
				}
				if err := r.Test(context.Background(), nil); err != nil {
					t.Error(err)
				}
			})
		}
	}
}

func archiveNames(t *testing.T, arc string) []string {
	t.Helper()
	r := mustOpen(t, arc)
	es, err := r.Entries(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range es {
		n := e.Path
		if e.IsDir {
			n += "/"
		}
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func TestCreateOptions(t *testing.T) {
	src := makeSourceTree(t)
	parent := filepath.Dir(src)
	cases := []struct {
		name    string
		sources []string
		opt     CreateOptions
		want    string
	}{
		{"default names from base", []string{filepath.Join(src, "sub")}, CreateOptions{},
			"sub/,sub/b.txt,sub/deeper/,sub/deeper/c.txt"},
		{"base dir keeps relative path", []string{filepath.Join(src, "sub", "deeper")}, CreateOptions{BaseDir: parent},
			"proj/,proj/sub/,proj/sub/deeper/,proj/sub/deeper/c.txt"},
		{"relative sources under base dir", []string{"a.txt", "sub/b.txt"}, CreateOptions{BaseDir: src},
			"a.txt,sub/,sub/b.txt"},
		{"prefix", []string{filepath.Join(src, "a.txt")}, CreateOptions{Prefix: "/in/side/"},
			"in/,in/side/,in/side/a.txt"},
		{"no paths", []string{filepath.Join(src, "sub")}, CreateOptions{NoPaths: true},
			"b.txt,c.txt"},
		{"no recurse", []string{filepath.Join(src, "sub")}, CreateOptions{NoRecurse: true},
			"sub/,sub/b.txt"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			arc := filepath.Join(t.TempDir(), "o.tar")
			if err := Create(context.Background(), arc, c.sources, c.opt); err != nil {
				t.Fatal(err)
			}
			got := strings.Join(archiveNames(t, arc), ",")
			// Folders above stored names are listed even when only
			// implied, which is what the panel shows too.
			if got != c.want {
				t.Errorf("got %s, want %s", got, c.want)
			}
		})
	}
}

func TestCreateFollowSymlinks(t *testing.T) {
	needSymlinks(t)
	src := makeSourceTree(t)
	arc := filepath.Join(t.TempDir(), "o.tar")
	if err := Create(context.Background(), arc, []string{filepath.Join(src, "link")}, CreateOptions{FollowSymlinks: true}); err != nil {
		t.Fatal(err)
	}
	r := mustOpen(t, arc)
	e, err := r.Stat(context.Background(), "link")
	if err != nil || !e.Mode.IsRegular() || e.Size != 5 {
		t.Errorf("entry = %+v, %v", e, err)
	}
}

func TestCreateHardLinks(t *testing.T) {
	needSymlinks(t)
	dir := filepath.Join(t.TempDir(), "h")
	writeFile(t, filepath.Join(dir, "one"), []byte(strings.Repeat("x", 10000)))
	if err := os.Link(filepath.Join(dir, "one"), filepath.Join(dir, "two")); err != nil {
		t.Skip("no hard links here:", err)
	}
	arc := filepath.Join(t.TempDir(), "h.tar")
	if err := Create(context.Background(), arc, []string{dir}, CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(arc); fi.Size() > 15000 {
		t.Errorf("data stored twice: %d bytes", fi.Size())
	}
	r := mustOpen(t, arc)
	h, err := r.Header(context.Background(), "h/two")
	if err != nil || h.Linkname != "h/one" {
		t.Errorf("header = %+v, %v", h, err)
	}
	dest := t.TempDir()
	if err := r.Extract(context.Background(), nil, dest, archive.ExtractOptions{}); err != nil {
		t.Fatal(err)
	}
	if len(readFile(t, filepath.Join(dest, "h", "two"))) != 10000 {
		t.Error("link data wrong")
	}
}

func TestCreateSingleFormats(t *testing.T) {
	src := makeSourceTree(t)
	for _, f := range singleFormats {
		t.Run(f.String(), func(t *testing.T) {
			arc := filepath.Join(t.TempDir(), "a.txt"+f.Ext())
			if err := Create(context.Background(), arc, []string{filepath.Join(src, "a.txt")}, CreateOptions{}); err != nil {
				t.Fatal(err)
			}
			r := mustOpen(t, arc)
			if r.Format() != f {
				t.Errorf("format %v", r.Format())
			}
			dest := t.TempDir()
			if err := r.Extract(context.Background(), nil, dest, archive.ExtractOptions{}); err != nil {
				t.Fatal(err)
			}
			if readFile(t, filepath.Join(dest, "a.txt")) != "alpha" {
				t.Error("content")
			}
			bad := filepath.Join(t.TempDir(), "x"+f.Ext())
			if err := Create(context.Background(), bad, []string{filepath.Join(src, "sub")}, CreateOptions{}); err == nil {
				t.Error("packing a folder into a single-file format succeeded")
			}
			if _, err := os.Stat(bad); err == nil {
				t.Error("failed pack left a file")
			}
		})
	}
	// gzip keeps the original name and date in its header.
	arc := filepath.Join(t.TempDir(), "renamed.gz")
	if err := Create(context.Background(), arc, []string{filepath.Join(src, "a.txt")}, CreateOptions{Format: FormatGz}); err != nil {
		t.Fatal(err)
	}
	es, _ := mustOpen(t, arc).Entries(context.Background())
	if es[0].Path != "a.txt" || !es[0].ModTime.Equal(time.Date(2020, 5, 6, 7, 8, 9, 0, time.UTC)) {
		t.Errorf("entry %+v", es[0])
	}
}

func TestCreateErrors(t *testing.T) {
	src := makeSourceTree(t)
	dir := t.TempDir()
	if err := Create(context.Background(), filepath.Join(dir, "x.zip"), []string{src}, CreateOptions{}); err == nil {
		t.Error("unknown format accepted")
	}
	if err := Create(context.Background(), filepath.Join(dir, "x.tar"), nil, CreateOptions{}); err == nil {
		t.Error("empty source list accepted")
	}
	if err := Create(context.Background(), filepath.Join(dir, "x.tar"), []string{filepath.Join(dir, "missing")}, CreateOptions{}); err == nil {
		t.Error("missing source accepted")
	}
	if err := Create(context.Background(), filepath.Join(dir, "x.tar"), []string{src}, CreateOptions{Prefix: "../up"}); err == nil {
		t.Error("escaping prefix accepted")
	}
	if err := Create(context.Background(), filepath.Join(dir, "x.tgz"), []string{src}, CreateOptions{Level: 12}); err == nil {
		t.Error("bad level accepted")
	}
	des, _ := os.ReadDir(dir)
	if len(des) != 0 {
		t.Errorf("failed packs left files: %v", des)
	}
}

func TestCreateInsideSourceSkipsItself(t *testing.T) {
	src := makeSourceTree(t)
	arc := filepath.Join(src, "self.tar")
	writeFile(t, arc, []byte("old archive"))
	if err := Create(context.Background(), arc, []string{src}, CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	for _, n := range archiveNames(t, arc) {
		if strings.Contains(n, "self.tar") || strings.Contains(n, ".mctmp-") {
			t.Errorf("archive contains itself: %s", n)
		}
	}
}

func TestCreateReplacesOnlyWhenDone(t *testing.T) {
	src := makeSourceTree(t)
	arc := filepath.Join(t.TempDir(), "keep.tar")
	writeFile(t, arc, []byte("precious"))
	ctx, cancel := context.WithCancel(context.Background())
	err := Create(ctx, arc, []string{src}, CreateOptions{Progress: func(archive.Progress) { cancel() }})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v", err)
	}
	if readFile(t, arc) != "precious" {
		t.Error("existing archive damaged by a cancelled pack")
	}
	des, _ := os.ReadDir(filepath.Dir(arc))
	if len(des) != 1 {
		t.Errorf("leftovers: %v", des)
	}
}

func TestCreateMoveToArchive(t *testing.T) {
	src := makeSourceTree(t)
	keep := writeFile(t, filepath.Join(src, "sub", "deeper", "keep.txt"), []byte("k"))
	arc := filepath.Join(t.TempDir(), "moved.tar.xz")
	var last archive.Progress
	err := Create(context.Background(), arc, []string{filepath.Join(src, "sub"), filepath.Join(src, "a.txt")},
		CreateOptions{RemoveSources: true, NoRecurse: true, Progress: func(p archive.Progress) { last = p }})
	if err != nil {
		t.Fatal(err)
	}
	// NoRecurse: sub/deeper was not packed, so sub must survive with it.
	if _, err := os.Stat(keep); err != nil {
		t.Error("unpacked file was deleted")
	}
	if _, err := os.Stat(filepath.Join(src, "sub", "b.txt")); err == nil {
		t.Error("packed file not removed")
	}
	if _, err := os.Stat(filepath.Join(src, "a.txt")); err == nil {
		t.Error("packed file not removed")
	}
	if _, err := os.Stat(filepath.Join(src, "sub")); err != nil {
		t.Error("non-empty folder removed")
	}
	if last.Files != 3 || last.Bytes != 9 || last.Total != 9 {
		t.Errorf("progress %+v", last)
	}
	names := strings.Join(archiveNames(t, arc), ",")
	if names != "a.txt,sub/,sub/b.txt" {
		t.Errorf("archive = %s", names)
	}
}

// TestSystemTarReadsOurArchives checks that archives made here open in the
// platform's tar program (bsdtar on macOS and Windows, GNU tar on Linux).
func TestSystemTarReadsOurArchives(t *testing.T) {
	tarBin, err := exec.LookPath("tar")
	if err != nil {
		t.Skip("no tar program")
	}
	src := makeSourceTree(t)
	for _, f := range []Format{FormatTar, FormatTarGz, FormatTarBz2, FormatTarXz} {
		arc := filepath.Join(t.TempDir(), "sys"+f.Ext())
		if err := Create(context.Background(), arc, []string{src}, CreateOptions{}); err != nil {
			t.Fatal(err)
		}
		out, err := exec.Command(tarBin, "-tf", arc).CombinedOutput()
		if err != nil {
			if strings.Contains(string(out), "not supported") || strings.Contains(string(out), "Cannot exec") {
				continue
			}
			t.Errorf("%v: tar -tf failed: %v\n%s", f, err, out)
			continue
		}
		if !strings.Contains(string(out), "proj/sub/deeper/c.txt") {
			t.Errorf("%v: listing %s", f, out)
		}
	}
}

// TestWeReadSystemTar checks the other direction.
func TestWeReadSystemTar(t *testing.T) {
	tarBin, err := exec.LookPath("tar")
	if err != nil {
		t.Skip("no tar program")
	}
	src := makeSourceTree(t)
	for _, c := range []struct{ flag, ext string }{{"-cf", ".tar"}, {"-czf", ".tgz"}, {"-cjf", ".tbz"}, {"-cJf", ".txz"}} {
		arc := filepath.Join(t.TempDir(), "sys"+c.ext)
		cmd := exec.Command(tarBin, c.flag, arc, "proj")
		cmd.Dir = filepath.Dir(src)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Logf("%s: system tar cannot create: %v %s", c.ext, err, out)
			continue
		}
		r := mustOpen(t, arc)
		dest := t.TempDir()
		if err := r.Extract(context.Background(), nil, dest, archive.ExtractOptions{}); err != nil {
			t.Fatalf("%s: %v", c.ext, err)
		}
		if readFile(t, filepath.Join(dest, "proj", "sub", "deeper", "c.txt")) != "gamma" {
			t.Errorf("%s: content", c.ext)
		}
	}
}
