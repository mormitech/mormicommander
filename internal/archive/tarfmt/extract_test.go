// Tests for extraction: contents, permissions and dates, selections, the
// unpack dialog's options, overwrite policies, cancellation, and above all
// the protections against archives that try to write outside the target
// folder through "..", absolute names or symlinks.
package tarfmt

import (
	"archive/tar"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	archive "mormicommander/internal/archive/arcapi"
)

func extractAll(t *testing.T, ms []member, opt archive.ExtractOptions, names ...string) (string, error) {
	t.Helper()
	p := writeFile(t, filepath.Join(t.TempDir(), "x.tar"), makeTar(t, ms))
	r := mustOpen(t, p)
	dest := filepath.Join(t.TempDir(), "out")
	return dest, r.Extract(context.Background(), names, dest, opt)
}

func TestExtractContentsModesTimes(t *testing.T) {
	needSymlinks(t)
	dirTime := time.Date(2019, 1, 2, 3, 4, 5, 0, time.UTC)
	dest, err := extractAll(t, []member{
		{name: "d/", typ: tar.TypeDir, mode: 0o750, mtime: dirTime},
		{name: "d/x.sh", body: "#!/bin/sh", mode: 0o755},
		{name: "d/ro.txt", body: "ro", mode: 0o444},
		{name: "d/suid", body: "s", mode: 0o4755},
		{name: "d/ln", typ: tar.TypeSymlink, link: "x.sh"},
		{name: "d/hard", typ: tar.TypeLink, link: "d/x.sh"},
		{name: "fifo", typ: tar.TypeFifo},
		{name: "implied/f", body: "f"},
	}, archive.ExtractOptions{})
	if err != nil {
		t.Fatal(err)
	}
	tree := listTree(t, dest)
	want := map[string]string{
		"d": "dir", "d/x.sh": "#!/bin/sh", "d/ro.txt": "ro", "d/suid": "s", "d/ln": "link:x.sh",
		"d/hard": "#!/bin/sh", "implied": "dir", "implied/f": "f",
	}
	if len(tree) != len(want) {
		t.Errorf("tree = %v", tree)
	}
	for k, v := range want {
		if tree[k] != v {
			t.Errorf("%s = %q, want %q", k, tree[k], v)
		}
	}
	check := func(rel string, mode fs.FileMode, mt time.Time) {
		fi, err := os.Lstat(filepath.Join(dest, rel))
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != mode {
			t.Errorf("%s mode %v, want %v", rel, fi.Mode().Perm(), mode)
		}
		if fi.Mode()&(fs.ModeSetuid|fs.ModeSetgid) != 0 {
			t.Errorf("%s kept setuid/setgid", rel)
		}
		if !fi.ModTime().Equal(mt) {
			t.Errorf("%s mtime %v, want %v", rel, fi.ModTime(), mt)
		}
	}
	check("d", 0o750, dirTime)
	check("d/x.sh", 0o755, testTime)
	check("d/ro.txt", 0o444, testTime)
	check("d/suid", 0o755, testTime)
	a, _ := os.Stat(filepath.Join(dest, "d/x.sh"))
	b, _ := os.Stat(filepath.Join(dest, "d/hard"))
	if !os.SameFile(a, b) {
		t.Error("hard link not restored as a link")
	}
	assertNoTemp(t, dest)
}

func assertNoTemp(t *testing.T, dir string) {
	t.Helper()
	_ = filepath.Walk(dir, func(p string, fi os.FileInfo, err error) error {
		if err == nil && strings.Contains(fi.Name(), ".mctmp-") {
			t.Errorf("temporary file left: %s", p)
		}
		return nil
	})
}

func TestExtractTraversalRefused(t *testing.T) {
	cases := []struct {
		name   string
		ms     []member
		inside map[string]string // expected tree under dest
		errs   int
	}{
		{"dotdot", []member{{name: "../evil", body: "x"}, {name: "ok", body: "y"}}, map[string]string{"ok": "y"}, 1},
		{"deep dotdot", []member{{name: "a/../../evil", body: "x"}}, map[string]string{}, 1},
		{"absolute becomes relative", []member{{name: "/abs/f", body: "x"}}, map[string]string{"abs": "dir", "abs/f": "x"}, 0},
		{"dot prefix", []member{{name: "./a/./b", body: "x"}}, map[string]string{"a": "dir", "a/b": "x"}, 0},
		{"hard link outside", []member{{name: "h", typ: tar.TypeLink, link: "../../etc/passwd"}}, map[string]string{}, 1},
		{"hard link missing target", []member{{name: "h", typ: tar.TypeLink, link: "nothere"}}, map[string]string{}, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			outer := t.TempDir()
			p := writeFile(t, filepath.Join(outer, "x.tar"), makeTar(t, c.ms))
			r := mustOpen(t, p)
			dest := filepath.Join(outer, "sub", "out")
			err := r.Extract(context.Background(), nil, dest, archive.ExtractOptions{})
			if n := countEntryErrors(err); n != c.errs {
				t.Errorf("%d entry errors (%v), want %d", n, err, c.errs)
			}
			tree := listTree(t, dest)
			if len(tree) != len(c.inside) {
				t.Errorf("tree = %v, want %v", tree, c.inside)
			}
			for k, v := range c.inside {
				if tree[k] != v {
					t.Errorf("%s = %q, want %q", k, tree[k], v)
				}
			}
			if _, err := os.Stat(filepath.Join(outer, "sub", "evil")); err == nil {
				t.Error("file escaped the target folder")
			}
			if _, err := os.Stat(filepath.Join(outer, "evil")); err == nil {
				t.Error("file escaped the target folder")
			}
		})
	}
}

func countEntryErrors(err error) int {
	if err == nil {
		return 0
	}
	if j, ok := err.(interface{ Unwrap() []error }); ok {
		n := 0
		for _, e := range j.Unwrap() {
			var ee *archive.EntryError
			if errors.As(e, &ee) {
				n++
			}
		}
		return n
	}
	var ee *archive.EntryError
	if errors.As(err, &ee) {
		return 1
	}
	return -1
}

func TestExtractSymlinkAttacks(t *testing.T) {
	needSymlinks(t)
	cases := []struct {
		name string
		ms   []member
	}{
		{"absolute link then write through it", []member{
			{name: "l", typ: tar.TypeSymlink, link: "OUTSIDE"},
			{name: "l/pwned", body: "x"},
		}},
		{"relative link out then write through it", []member{
			{name: "l", typ: tar.TypeSymlink, link: "../.."},
			{name: "l/pwned", body: "x"},
		}},
		{"link to itself-folder then climb", []member{
			{name: "s", typ: tar.TypeSymlink, link: "."},
			{name: "s/esc", typ: tar.TypeSymlink, link: "../pwned"},
			{name: "esc/pwned", body: "x"},
		}},
		{"name-then-dotdot trick", []member{
			{name: "d/", typ: tar.TypeDir},
			{name: "t", typ: tar.TypeSymlink, link: "d/../.."},
			{name: "t/pwned", body: "x"},
		}},
		{"link replaced file then overwrite", []member{
			{name: "f", typ: tar.TypeSymlink, link: "OUTSIDE/pwned"},
			{name: "f", body: "x"},
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			outer := t.TempDir()
			outside := filepath.Join(outer, "outside")
			if err := os.Mkdir(outside, 0o755); err != nil {
				t.Fatal(err)
			}
			ms := append([]member(nil), c.ms...)
			for i := range ms {
				ms[i].link = strings.ReplaceAll(ms[i].link, "OUTSIDE", outside)
			}
			p := writeFile(t, filepath.Join(outer, "x.tar"), makeTar(t, ms))
			r := mustOpen(t, p)
			dest := filepath.Join(outer, "a", "b", "out")
			_ = r.Extract(context.Background(), nil, dest, archive.ExtractOptions{Conflict: archive.ConflictOverwrite})
			if des, _ := os.ReadDir(outside); len(des) != 0 {
				t.Errorf("wrote into outside folder: %v", des)
			}
			for _, p := range []string{filepath.Join(outer, "pwned"), filepath.Join(outer, "a", "pwned"), filepath.Join(outer, "a", "b", "pwned")} {
				if _, err := os.Lstat(p); err == nil {
					t.Errorf("escaped to %s", p)
				}
			}
			// Every link that was created must point inside dest.
			_ = filepath.Walk(dest, func(p string, fi os.FileInfo, err error) error {
				if err == nil && fi.Mode()&os.ModeSymlink != 0 {
					l, _ := os.Readlink(p)
					if filepath.IsAbs(l) {
						t.Errorf("absolute link created: %s -> %s", p, l)
					}
				}
				return nil
			})
		})
	}
}

func TestExtractThroughPreexistingSymlink(t *testing.T) {
	needSymlinks(t)
	outer := t.TempDir()
	outside := filepath.Join(outer, "outside")
	dest := filepath.Join(outer, "dest")
	os.Mkdir(outside, 0o755)
	os.Mkdir(dest, 0o755)
	if err := os.Symlink(outside, filepath.Join(dest, "link")); err != nil {
		t.Fatal(err)
	}
	p := writeFile(t, filepath.Join(outer, "x.tar"), makeTar(t, []member{{name: "link/f", body: "x"}}))
	r := mustOpen(t, p)
	err := r.Extract(context.Background(), nil, dest, archive.ExtractOptions{Conflict: archive.ConflictOverwrite})
	if countEntryErrors(err) != 1 {
		t.Errorf("err = %v", err)
	}
	if des, _ := os.ReadDir(outside); len(des) != 0 {
		t.Error("wrote through a pre-existing symlink to outside")
	}
}

func TestExtractSafeSymlinks(t *testing.T) {
	needSymlinks(t)
	dest, err := extractAll(t, []member{
		{name: "a/b/f", body: "x"},
		{name: "a/b/up", typ: tar.TypeSymlink, link: "../c"},
		{name: "a/self", typ: tar.TypeSymlink, link: "."},
		{name: "a/deep", typ: tar.TypeSymlink, link: "b/f"},
	}, archive.ExtractOptions{})
	if err != nil {
		t.Fatal(err)
	}
	tree := listTree(t, dest)
	if tree["a/b/up"] != "link:../c" || tree["a/self"] != "link:." || tree["a/deep"] != "link:b/f" {
		t.Errorf("tree = %v", tree)
	}
}

func TestExtractSelectionAndOptions(t *testing.T) {
	ms := []member{
		{name: "src/", typ: tar.TypeDir},
		{name: "src/main.go", body: "main"},
		{name: "src/util/u.go", body: "util"},
		{name: "src/util/u.txt", body: "txt"},
		{name: "docs/readme.txt", body: "readme"},
		{name: "top.go", body: "top"},
	}
	cases := []struct {
		name  string
		names []string
		opt   archive.ExtractOptions
		want  map[string]string
	}{
		{"one file", []string{"top.go"}, archive.ExtractOptions{}, map[string]string{"top.go": "top"}},
		{"folder selects subtree", []string{"src/util"}, archive.ExtractOptions{},
			map[string]string{"src": "dir", "src/util": "dir", "src/util/u.go": "util", "src/util/u.txt": "txt"}},
		{"selection with slashes", []string{"/src/util/"}, archive.ExtractOptions{},
			map[string]string{"src": "dir", "src/util": "dir", "src/util/u.go": "util", "src/util/u.txt": "txt"}},
		{"strip prefix", []string{"src/util/u.go", "src/main.go"}, archive.ExtractOptions{StripPrefix: "src"},
			map[string]string{"main.go": "main", "util": "dir", "util/u.go": "util"}},
		{"strip prefix ignores outside", nil, archive.ExtractOptions{StripPrefix: "src/util"},
			map[string]string{"u.go": "util", "u.txt": "txt"}},
		{"flatten", []string{"src"}, archive.ExtractOptions{Flatten: true},
			map[string]string{"main.go": "main", "u.go": "util", "u.txt": "txt"}},
		{"filter mask", nil, archive.ExtractOptions{Filter: func(e archive.Entry) bool { return strings.HasSuffix(e.Path, ".go") }},
			map[string]string{"src": "dir", "src/util": "dir", "src/main.go": "main", "src/util/u.go": "util", "top.go": "top"}},
		{"nothing matches", []string{"nope"}, archive.ExtractOptions{}, map[string]string{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dest, err := extractAll(t, ms, c.opt, c.names...)
			if err != nil {
				t.Fatal(err)
			}
			tree := listTree(t, dest)
			if len(tree) != len(c.want) {
				t.Errorf("tree = %v, want %v", tree, c.want)
			}
			for k, v := range c.want {
				if tree[k] != v {
					t.Errorf("%s = %q, want %q", k, tree[k], v)
				}
			}
		})
	}
}

func TestExtractHardlinkWithoutTarget(t *testing.T) {
	// Only the link is selected: its data comes from a second pass.
	dest, err := extractAll(t, []member{
		{name: "data/orig", body: "shared", mode: 0o640},
		{name: "links/l1", typ: tar.TypeLink, link: "data/orig"},
		{name: "links/l2", typ: tar.TypeLink, link: "data/orig"},
	}, archive.ExtractOptions{}, "links")
	if err != nil {
		t.Fatal(err)
	}
	tree := listTree(t, dest)
	if tree["links/l1"] != "shared" || tree["links/l2"] != "shared" || tree["data/orig"] != "" {
		t.Errorf("tree = %v", tree)
	}
	if fi, err := os.Stat(filepath.Join(dest, "links/l1")); err == nil && runtime.GOOS != "windows" && fi.Mode().Perm() != 0o640 {
		t.Errorf("mode %v", fi.Mode().Perm())
	}
}

func TestExtractConflicts(t *testing.T) {
	newer := testTime
	older := testTime.Add(-time.Hour)
	ms := []member{{name: "f", body: "archived", mtime: newer}}
	cases := []struct {
		name     string
		existing time.Time
		opt      archive.ExtractOptions
		want     string
		err      error
	}{
		{"ask without callback skips", older, archive.ExtractOptions{}, "local", nil},
		{"overwrite", older, archive.ExtractOptions{Conflict: archive.ConflictOverwrite}, "archived", nil},
		{"skip", older, archive.ExtractOptions{Conflict: archive.ConflictSkip}, "local", nil},
		{"older replaced", older, archive.ExtractOptions{Conflict: archive.ConflictOverwriteOlder}, "archived", nil},
		{"newer kept", newer.Add(time.Hour), archive.ExtractOptions{Conflict: archive.ConflictOverwriteOlder}, "local", nil},
		{"same age kept", newer, archive.ExtractOptions{Conflict: archive.ConflictOverwriteOlder}, "local", nil},
		{"abort", older, archive.ExtractOptions{Conflict: archive.ConflictAbort}, "local", archive.ErrAborted},
		{"callback overwrite", older, archive.ExtractOptions{OnConflict: func(c archive.Conflict) archive.ConflictPolicy {
			if c.Entry.Path != "f" || c.Existing.Size() != 5 || filepath.Base(c.Path) != "f" {
				return archive.ConflictAbort
			}
			return archive.ConflictOverwrite
		}}, "archived", nil},
		{"callback abort", older, archive.ExtractOptions{OnConflict: func(archive.Conflict) archive.ConflictPolicy { return archive.ConflictAbort }}, "local", archive.ErrAborted},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := writeFile(t, filepath.Join(t.TempDir(), "x.tar"), makeTar(t, ms))
			r := mustOpen(t, p)
			dest := t.TempDir()
			lp := writeFile(t, filepath.Join(dest, "f"), []byte("local"))
			os.Chtimes(lp, c.existing, c.existing)
			err := r.Extract(context.Background(), nil, dest, c.opt)
			if !errors.Is(err, c.err) || (c.err == nil && err != nil) {
				t.Errorf("err = %v, want %v", err, c.err)
			}
			if got := readFile(t, lp); got != c.want {
				t.Errorf("content %q, want %q", got, c.want)
			}
			assertNoTemp(t, dest)
		})
	}
}

func TestExtractOverReadOnlyAndFolderClash(t *testing.T) {
	p := writeFile(t, filepath.Join(t.TempDir(), "x.tar"), makeTar(t, []member{{name: "ro", body: "new"}, {name: "d", body: "file"}}))
	r := mustOpen(t, p)
	dest := t.TempDir()
	ro := writeFile(t, filepath.Join(dest, "ro"), []byte("old"))
	os.Chmod(ro, 0o444)
	os.Mkdir(filepath.Join(dest, "d"), 0o755)
	err := r.Extract(context.Background(), nil, dest, archive.ExtractOptions{Conflict: archive.ConflictOverwrite})
	if countEntryErrors(err) != 1 {
		t.Errorf("err = %v (want one error for the folder clash)", err)
	}
	if readFile(t, ro) != "new" {
		t.Error("read-only file not replaced")
	}
}

func TestExtractFileReplacedByFolder(t *testing.T) {
	p := writeFile(t, filepath.Join(t.TempDir(), "x.tar"), makeTar(t, []member{{name: "d/", typ: tar.TypeDir}, {name: "d/f", body: "x"}}))
	r := mustOpen(t, p)
	dest := t.TempDir()
	writeFile(t, filepath.Join(dest, "d"), []byte("in the way"))
	if err := r.Extract(context.Background(), nil, dest, archive.ExtractOptions{Conflict: archive.ConflictOverwrite}); err != nil {
		t.Fatal(err)
	}
	if readFile(t, filepath.Join(dest, "d", "f")) != "x" {
		t.Error("folder not created")
	}
}

func TestExtractCancelAndProgress(t *testing.T) {
	big := strings.Repeat("0123456789", 300_000)
	plain := makeTar(t, []member{{name: "a", body: big}, {name: "b", body: big}, {name: "c", body: "c"}})
	p := writeFile(t, filepath.Join(t.TempDir(), "x.tar.gz"), compressBytes(t, CompGzip, plain))

	// Full run: progress ends with everything counted.
	r := mustOpen(t, p)
	if _, err := r.Entries(context.Background()); err != nil {
		t.Fatal(err)
	}
	var last archive.Progress
	calls := 0
	dest := t.TempDir()
	err := r.Extract(context.Background(), nil, dest, archive.ExtractOptions{Progress: func(pr archive.Progress) { last = pr; calls++ }})
	if err != nil {
		t.Fatal(err)
	}
	if last.Files != 3 || last.Bytes != int64(2*len(big)+1) || last.Total != last.Bytes || last.ArchiveSize == 0 || calls < 5 {
		t.Errorf("last = %+v, calls %d", last, calls)
	}

	// Cancel during the first big file.
	ctx, cancel := context.WithCancel(context.Background())
	dest2 := t.TempDir()
	err = mustOpen(t, p).Extract(ctx, nil, dest2, archive.ExtractOptions{Progress: func(pr archive.Progress) {
		if pr.Bytes > 100_000 {
			cancel()
		}
	}})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest2, "a")); err == nil {
		t.Error("partially extracted file left under its real name")
	}
	if _, err := os.Stat(filepath.Join(dest2, "c")); err == nil {
		t.Error("extraction continued after cancel")
	}
	assertNoTemp(t, dest2)
}

func TestExtractCorruptArchiveFails(t *testing.T) {
	plain := makeTar(t, []member{{name: "a", body: strings.Repeat("x", 100000)}})
	data := compressBytes(t, CompGzip, plain)
	p := writeFile(t, filepath.Join(t.TempDir(), "x.tgz"), data[:len(data)/2])
	r := mustOpen(t, p)
	dest := t.TempDir()
	err := r.Extract(context.Background(), nil, dest, archive.ExtractOptions{})
	if err == nil || countEntryErrors(err) > 0 {
		t.Errorf("err = %v, want an archive error", err)
	}
	assertNoTemp(t, dest)
	if _, err := os.Stat(filepath.Join(dest, "a")); err == nil {
		t.Error("truncated file written")
	}
}

func TestExtractCreatesDest(t *testing.T) {
	dest, err := extractAll(t, []member{{name: "f", body: "x"}}, archive.ExtractOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if readFile(t, filepath.Join(dest, "f")) != "x" {
		t.Error("missing")
	}
}

func TestReaderSatisfiesInterface(t *testing.T) {
	p := writeFile(t, filepath.Join(t.TempDir(), "x.tar"), makeTar(t, []member{{name: "f", body: "x"}}))
	var ar archive.Reader
	r, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	ar = r
	if es, err := ar.Entries(context.Background()); err != nil || len(es) != 1 {
		t.Fatal(es, err)
	}
	if err := ar.Close(); err != nil {
		t.Fatal(err)
	}
	if err := ar.Close(); err != nil {
		t.Error("second Close failed")
	}
}

func TestExtractFlattenCollisionAsks(t *testing.T) {
	ms := []member{{name: "a/f", body: "first"}, {name: "b/f", body: "second"}}
	asked := 0
	dest, err := extractAll(t, ms, archive.ExtractOptions{Flatten: true, OnConflict: func(c archive.Conflict) archive.ConflictPolicy {
		asked++
		return archive.ConflictSkip
	}})
	if err != nil {
		t.Fatal(err)
	}
	if asked != 1 || readFile(t, filepath.Join(dest, "f")) != "first" {
		t.Errorf("asked %d times, content %q", asked, readFile(t, filepath.Join(dest, "f")))
	}
}
