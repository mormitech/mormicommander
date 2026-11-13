// Tests for saved searches and the remembered last search: save, replace, rename,
// delete, reload from disk, JSON round trip of every option, and damaged files.

package find

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestStore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg", "searches.json")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Names()) != 0 {
		t.Fatal("new store not empty")
	}
	full := Options{
		SearchFor: "*.go|vendor/", SearchIn: []string{"/src"}, Levels: 3, FollowLinks: true,
		Kind: KindFiles, SkipHidden: true, ExcludeDirs: []string{"node_modules"}, SearchArchives: true,
		NameRegex: false,
		Content: ContentOptions{Text: "TODO", CaseSensitive: true, WholeWords: true, Encodings: EncUTF8 | EncUTF16LE,
			ANSICodepage: "windows-1250", NotContaining: true},
		Size:    SizeFilter{Op: SizeGreater, Value: 10, Unit: KB},
		Date:    DateFilter{From: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), NotOlderThan: Age{3, Weeks}},
		AttrSet: AttrReadOnly, AttrClear: AttrHidden,
		PermMask: 0o002, PermValue: 0o002,
		Duplicates: DupOptions{Enabled: true, Name: true, Hash: "md5"},
		Within:     []string{"/runtime/only"},
		FS:         OSFS{},
	}
	if err := s.Save("Go sources", full, false); err != nil {
		t.Fatal(err)
	}
	if err := s.Save("with folders", full, true); err != nil {
		t.Fatal(err)
	}
	if err := s.Save("  ", full, true); err == nil {
		t.Error("empty name accepted")
	}
	if err := s.SetLast(full); err != nil {
		t.Fatal(err)
	}

	// Reload from disk.
	s2, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "names", s2.Names(), []string{"Go sources", "with folders"})
	got, ok := s2.Get("go SOURCES")
	if !ok {
		t.Fatal("Get failed")
	}
	want := full
	want.SearchIn, want.Within, want.FS = nil, nil, nil
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round trip:\n got  %+v\n want %+v", got, want)
	}
	wf, _ := s2.Get("with folders")
	if len(wf.SearchIn) != 1 {
		t.Errorf("keepSearchIn lost folders: %+v", wf.SearchIn)
	}
	last, ok := s2.Last()
	if !ok || last.SearchFor != full.SearchFor || len(last.SearchIn) != 1 || last.Within != nil {
		t.Errorf("last: %+v", last)
	}

	// Replace, rename, delete.
	full.SearchFor = "*.md"
	if err := s2.Save("GO SOURCES", full, false); err != nil {
		t.Fatal(err)
	}
	if n := s2.Names(); len(n) != 2 {
		t.Errorf("replace added a template: %v", n)
	}
	if err := s2.Rename("go sources", "with folders"); err == nil {
		t.Error("rename onto existing name accepted")
	}
	if err := s2.Rename("missing", "x"); err == nil {
		t.Error("rename of missing accepted")
	}
	if err := s2.Rename("go sources", "Markdown"); err != nil {
		t.Fatal(err)
	}
	if err := s2.Delete("with folders"); err != nil {
		t.Fatal(err)
	}
	if err := s2.Delete("never existed"); err != nil {
		t.Fatal(err)
	}
	s3, _ := OpenStore(path)
	eq(t, "after edits", s3.Names(), []string{"Markdown"})
	md, _ := s3.Get("markdown")
	if md.SearchFor != "*.md" {
		t.Errorf("replaced template: %+v", md)
	}
	// No temporary files left behind.
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("leftover files: %v", entries)
	}
}

func TestStoreDamaged(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.json")
	os.WriteFile(p, []byte("{not json"), 0o644)
	if _, err := OpenStore(p); err == nil || !strings.Contains(err.Error(), "damaged") {
		t.Errorf("want damaged error, got %v", err)
	}
}

// A saved template used as a selection filter on single files.
func TestTemplateAsFilter(t *testing.T) {
	root := mkTree(t, map[string]string{"keep.txt": "TODO", "drop.txt": "done"})
	s, _ := OpenStore(filepath.Join(t.TempDir(), "s.json"))
	o := DefaultOptions()
	o.SearchFor = "*.txt"
	o.Content = ContentOptions{Text: "todo"}
	if err := s.Save("todos", o, false); err != nil {
		t.Fatal(err)
	}
	tpl, _ := s.Get("todos")
	tpl.SearchIn = []string{root}
	q, err := Compile(tpl)
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]bool{"keep.txt": true, "drop.txt": false} {
		if got, _ := q.Match(context.Background(), filepath.Join(root, name)); got != want {
			t.Errorf("%s: %v", name, got)
		}
	}
}
