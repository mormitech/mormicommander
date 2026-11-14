// Regression tests for tar rewrites: adding a file under the name of a
// folder (or below a file) must be refused instead of leaving a tar whose
// folder contents are hidden, AddTo must store each source under its own
// name in one rewrite, and changing or creating an archive through a
// symbolic link must update the real archive and keep the link.
package tarfmt

import (
	"archive/tar"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func arcNames(t *testing.T, arc string) []string {
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

func TestModifyAddConflicts(t *testing.T) {
	src := t.TempDir()
	fileDocs := writeFile(t, filepath.Join(src, "f", "docs"), []byte("file"))
	dirDocs := filepath.Join(src, "d", "docs")
	writeFile(t, filepath.Join(dirDocs, "new.txt"), []byte("n"))
	cases := []struct {
		name string
		base []member
		ch   Changes
		fail bool
	}{
		{"file over folder with header", []member{{name: "docs/", typ: tar.TypeDir}, {name: "docs/inner.txt", body: "i"}},
			Changes{Add: []string{fileDocs}}, true},
		{"file over implied folder", []member{{name: "docs/inner.txt", body: "i"}},
			Changes{Add: []string{fileDocs}}, true},
		{"file below a file", []member{{name: "b.txt", body: "b"}},
			Changes{AddTo: []AddItem{{Source: fileDocs, Dest: "b.txt/docs"}}}, true},
		{"file over folder that is deleted", []member{{name: "docs/inner.txt", body: "i"}},
			Changes{Delete: []string{"docs"}, Add: []string{fileDocs}}, false},
		{"file over folder that is renamed", []member{{name: "docs/inner.txt", body: "i"}},
			Changes{Rename: []Rename{{From: "docs", To: "old"}}, Add: []string{fileDocs}}, false},
		{"folder over file", []member{{name: "docs", body: "f"}},
			Changes{Add: []string{dirDocs}}, false},
		{"folder merges with folder", []member{{name: "docs/inner.txt", body: "i"}},
			Changes{Add: []string{dirDocs}}, false},
		{"file replaces file", []member{{name: "docs", body: "f"}},
			Changes{Add: []string{fileDocs}}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			arc := newArchive(t, FormatTar, c.base)
			before, _ := os.ReadFile(arc)
			err := Modify(context.Background(), arc, c.ch)
			if c.fail {
				if err == nil {
					t.Fatalf("Modify succeeded: %v", arcNames(t, arc))
				}
				if !errors.Is(err, fs.ErrExist) {
					t.Errorf("err = %v, want ErrExist", err)
				}
				after, _ := os.ReadFile(arc)
				if string(before) != string(after) {
					t.Error("refused change modified the archive")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestModifyAddTo(t *testing.T) {
	src := t.TempDir()
	f := writeFile(t, filepath.Join(src, "local.txt"), []byte("L"))
	d := filepath.Join(src, "folder")
	writeFile(t, filepath.Join(d, "in.txt"), []byte("I"))
	arc := newArchive(t, FormatTar, []member{{name: "seed.txt", body: "s"}})
	ch := Changes{
		Delete: []string{"seed.txt"},
		AddTo: []AddItem{
			{Source: f, Dest: "a/renamed.txt"},
			{Source: f, Dest: "b/c/other.txt"},
			{Source: d, Dest: "x/newfolder"},
		},
	}
	if err := Modify(context.Background(), arc, ch); err != nil {
		t.Fatal(err)
	}
	got := arcNames(t, arc)
	want := []string{"a/", "a/renamed.txt", "b/", "b/c/", "b/c/other.txt", "x/", "x/newfolder/", "x/newfolder/in.txt"}
	if len(got) != len(want) {
		t.Fatalf("entries = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("entries = %v, want %v", got, want)
		}
	}
	if c := contentOf(t, arc, "x/newfolder/in.txt"); c != "I" {
		t.Errorf("content = %q", c)
	}
	if err := Modify(context.Background(), arc, Changes{AddTo: []AddItem{{Source: f, Dest: "../evil"}}}); err == nil {
		t.Error("AddTo accepted a name outside the archive")
	}
}

func TestModifyAndCreateThroughSymlink(t *testing.T) {
	needSymlinks(t)
	dir := t.TempDir()
	real := filepath.Join(dir, "real.tar")
	writeFile(t, real, makeTar(t, []member{{name: "a.txt", body: "a"}}))
	link := filepath.Join(dir, "link.tar")
	if err := os.Symlink("real.tar", link); err != nil {
		t.Fatal(err)
	}
	if err := Modify(context.Background(), link, Changes{Mkdir: []string{"newdir"}}); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("link replaced: %v %v", fi, err)
	}
	got := arcNames(t, real)
	if len(got) != 2 || got[0] != "a.txt" || got[1] != "newdir/" {
		t.Errorf("real archive = %v", got)
	}

	// Create through a dangling link makes the file the link names.
	src := writeFile(t, filepath.Join(dir, "src", "f.txt"), []byte("f"))
	dangling := filepath.Join(dir, "new.tar")
	if err := os.Symlink("made.tar", dangling); err != nil {
		t.Fatal(err)
	}
	if err := Create(context.Background(), dangling, []string{src}, CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(dangling); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("dangling link replaced: %v %v", fi, err)
	}
	if got := arcNames(t, filepath.Join(dir, "made.tar")); len(got) != 1 || got[0] != "f.txt" {
		t.Errorf("made.tar = %v", got)
	}
}

// A change keeps the archive's other hard links pointing at the changed archive.
func TestModifyKeepsHardLinks(t *testing.T) {
	dir := t.TempDir()
	arc := filepath.Join(dir, "a.tar")
	writeFile(t, arc, makeTar(t, []member{{name: "a.txt", body: "a"}}))
	other := filepath.Join(dir, "b.tar")
	if err := os.Link(arc, other); err != nil {
		t.Skip("no hard links here:", err)
	}
	if err := Modify(context.Background(), arc, Changes{Mkdir: []string{"newdir"}}); err != nil {
		t.Fatal(err)
	}
	a, _ := os.Stat(arc)
	b, _ := os.Stat(other)
	if !os.SameFile(a, b) {
		t.Fatal("hard link broken by the change")
	}
	if got := arcNames(t, other); len(got) != 2 || got[1] != "newdir/" {
		t.Errorf("linked name = %v", got)
	}
}
