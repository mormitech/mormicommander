package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"mormicommander/internal/vfs"
)

func newTestFind(t *testing.T) *Find {
	t.Helper()
	t.Setenv("MORMICOMMANDER_CONFIG_DIR", t.TempDir())
	a := &App{fs: vfs.Local{}}
	f := &Find{app: a, runs: map[int]*findRun{}, quick: map[string]*findQuickList{}, arc: vfs.NewArchiveFS()}
	findFeeds.def = a.fs
	return f
}

func findWriteFile(t *testing.T, p, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// runFind starts a search and waits for its end.
func runTestFind(t *testing.T, f *Find, r FindRequest) []findItem {
	t.Helper()
	id, err := f.Start(r)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for f.Running(id) {
		if time.Now().After(deadline) {
			t.Fatal("search did not end")
		}
		time.Sleep(5 * time.Millisecond)
	}
	return f.Results(id)
}

func findNames(items []findItem) []string {
	var out []string
	for _, it := range items {
		out = append(out, it.Name)
	}
	sort.Strings(out)
	return out
}

func TestFindSearchAndText(t *testing.T) {
	f := newTestFind(t)
	dir := t.TempDir()
	findWriteFile(t, filepath.Join(dir, "a.txt"), "hello world")
	findWriteFile(t, filepath.Join(dir, "old.txt"), "hello")
	findWriteFile(t, filepath.Join(dir, "sub", "b.txt"), "goodbye")
	findWriteFile(t, filepath.Join(dir, "vendor", "c.txt"), "hello")

	got := findNames(runTestFind(t, f, FindRequest{SearchFor: "*.txt|old* vendor/", SearchIn: dir, Depth: -1}))
	if want := []string{"a.txt", "b.txt"}; !reflect.DeepEqual(got, want) {
		t.Errorf("mask: got %v, want %v", got, want)
	}
	got = findNames(runTestFind(t, f, FindRequest{SearchFor: "*.txt", SearchIn: dir, Depth: 0}))
	if want := []string{"a.txt", "old.txt"}; !reflect.DeepEqual(got, want) {
		t.Errorf("depth 0: got %v, want %v", got, want)
	}
	got = findNames(runTestFind(t, f, FindRequest{SearchFor: "*", SearchIn: dir, Depth: -1, TextOn: true, Text: "HELLO", Encodings: []string{"utf8"}, Kind: "files"}))
	if want := []string{"a.txt", "c.txt", "old.txt"}; !reflect.DeepEqual(got, want) {
		t.Errorf("text: got %v, want %v", got, want)
	}
	got = findNames(runTestFind(t, f, FindRequest{SearchFor: "*.txt", SearchIn: dir, Depth: -1, TextOn: true, Text: "hello", NotContaining: true, Encodings: []string{"utf8"}}))
	if want := []string{"b.txt"}; !reflect.DeepEqual(got, want) {
		t.Errorf("not containing: got %v, want %v", got, want)
	}
	got = findNames(runTestFind(t, f, FindRequest{SearchFor: "*", SearchIn: dir, Depth: -1, Kind: "dirs"}))
	if want := []string{"sub", "vendor"}; !reflect.DeepEqual(got, want) {
		t.Errorf("folders only: got %v, want %v", got, want)
	}
}

func TestFindRequestChecks(t *testing.T) {
	f := newTestFind(t)
	dir := t.TempDir()
	cases := []struct {
		r    FindRequest
		want string
	}{
		{FindRequest{SearchFor: "*"}, "folder to search in"},
		{FindRequest{SearchIn: dir, TextOn: true, Text: "x"}, "character set"},
		{FindRequest{SearchIn: dir, TextOn: true, Text: "4G", Hex: true}, "hex"},
		{FindRequest{SearchIn: dir, Perms: "rwz"}, "permission letter"},
		{FindRequest{SearchIn: dir, DupOn: true}, "duplicate"},
		{FindRequest{SearchIn: dir, DateOn: true, DateFrom: "31.12.2026"}, "not valid"},
		{FindRequest{SearchIn: dir, SearchFor: "(", NameRegex: true}, "regular expression"},
		{FindRequest{SearchIn: dir, AttrSet: "r", AttrClear: "r"}, "both"},
	}
	for _, c := range cases {
		if _, err := f.Start(c.r); err == nil || !strings.Contains(strings.ToLower(err.Error()), c.want) {
			t.Errorf("%+v: got %v, want an error with %q", c.r, err, c.want)
		}
	}
}

func TestFindPermsAndRoundTrip(t *testing.T) {
	m, v, err := findPerms("rw-?????")
	if err != nil || m != 0o700 || v != 0o600 {
		t.Fatalf("findPerms: %o %o %v", m, v, err)
	}
	f := newTestFind(t)
	r := FindRequest{SearchFor: "*.go", SearchIn: "/tmp", Depth: 2, TextOn: true, Text: "x", WholeWords: true, Encodings: []string{"ansi", "utf16le"},
		DateOn: true, DateFrom: "2026-01-02", DateTo: "2026-03-04", SizeOn: true, SizeOp: ">", SizeValue: 3, SizeUnit: "MB",
		AttrSet: "r", AttrClear: "h", Perms: "rw-??????", Kind: "files", DupOn: true, DupSize: true, ExcludeDirs: "a;b"}
	o, err := f.findOptions(r)
	if err != nil {
		t.Fatal(err)
	}
	back := findRequest(o)
	for _, c := range [][2]any{{back.SearchFor, r.SearchFor}, {back.Depth, r.Depth}, {back.Text, r.Text}, {back.DateFrom, r.DateFrom}, {back.DateTo, r.DateTo},
		{back.SizeValue, r.SizeValue}, {back.SizeUnit, r.SizeUnit}, {back.AttrSet, r.AttrSet}, {back.AttrClear, r.AttrClear}, {back.Perms, r.Perms},
		{back.Kind, r.Kind}, {back.DupSize, true}, {back.ExcludeDirs, r.ExcludeDirs}, {strings.Join(back.Encodings, ","), "ansi,utf16le"}} {
		if c[0] != c[1] {
			t.Errorf("round trip: got %v, want %v", c[0], c[1])
		}
	}
}

func TestFindSavedSearches(t *testing.T) {
	f := newTestFind(t)
	dir := t.TempDir()
	if err := f.Save("Texts", FindRequest{SearchFor: "*.txt", SearchIn: dir, Depth: -1}, false); err != nil {
		t.Fatal(err)
	}
	if n, _ := f.Saved(); !reflect.DeepEqual(n, []string{"Texts"}) {
		t.Fatalf("Saved: %v", n)
	}
	r, err := f.LoadSaved("Texts")
	if err != nil || r.SearchFor != "*.txt" || r.SearchIn != "" {
		t.Fatalf("LoadSaved: %+v %v", r, err)
	}
	runTestFind(t, f, FindRequest{SearchFor: "last*", SearchIn: dir, Depth: -1})
	s := f.Setup()
	if s.Last == nil || s.Last.SearchFor != "last*" || len(s.History["name"]) == 0 || s.History["name"][0] != "last*" || s.History["in"][0] != dir {
		t.Fatalf("Setup: %+v", s)
	}
	if err := f.DeleteSaved("Texts"); err != nil {
		t.Fatal(err)
	}
	if n, _ := f.Saved(); len(n) != 0 {
		t.Fatalf("after delete: %v", n)
	}
}

func TestFindFeed(t *testing.T) {
	f := newTestFind(t)
	dir := t.TempDir()
	findWriteFile(t, filepath.Join(dir, "a.txt"), "1")
	findWriteFile(t, filepath.Join(dir, "sub", "b.txt"), "22")
	id, err := f.Start(FindRequest{SearchFor: "*.txt", SearchIn: dir, Depth: -1})
	if err != nil {
		t.Fatal(err)
	}
	for f.Running(id) {
		time.Sleep(5 * time.Millisecond)
	}
	info, err := f.Feed(id, dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Base != dir || info.Count != 2 {
		t.Fatalf("Feed: %+v", info)
	}
	fsys := vfs.For(info.Path, vfs.Local{})
	l, err := fsys.List(t.Context(), info.Path)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range l.Entries {
		got = append(got, e.Name)
	}
	sort.Strings(got)
	if want := []string{"a.txt", filepath.Join("sub", "b.txt")}; !reflect.DeepEqual(got, want) || l.Parent != dir {
		t.Fatalf("List: %v parent %q", got, l.Parent)
	}
	if fsys.Join(info.Path, filepath.Join("sub", "b.txt")) != filepath.Join(dir, "sub", "b.txt") {
		t.Errorf("Join does not give the real path")
	}
	// search in the fed results (F-544)
	got = findNames(runTestFind(t, f, FindRequest{SearchFor: "b*", SearchIn: info.Path}))
	if !reflect.DeepEqual(got, []string{"b.txt"}) {
		t.Errorf("search in results: %v", got)
	}
	// a deleted file leaves the list
	os.Remove(filepath.Join(dir, "a.txt"))
	l, _ = fsys.List(t.Context(), info.Path)
	if len(l.Entries) != 1 {
		t.Errorf("after delete: %d entries", len(l.Entries))
	}
}

func TestFindInArchives(t *testing.T) {
	f := newTestFind(t)
	dir := t.TempDir()
	zf, err := os.Create(filepath.Join(dir, "pack.zip"))
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(zf)
	w, _ := zw.Create("docs/readme.txt")
	w.Write([]byte("inside the archive"))
	zw.Close()
	zf.Close()
	items := runTestFind(t, f, FindRequest{SearchFor: "*.txt", SearchIn: dir, Depth: -1, Archives: true, TextOn: true, Text: "inside", Encodings: []string{"utf8"}})
	if len(items) != 1 || items[0].Name != "readme.txt" || !items[0].Archive {
		t.Fatalf("archive search: %+v", items)
	}
	if items := runTestFind(t, f, FindRequest{SearchFor: "*.txt", SearchIn: dir, Depth: -1}); len(items) != 0 {
		t.Fatalf("without archives: %+v", items)
	}
}

func TestFindQuickMatch(t *testing.T) {
	f := newTestFind(t)
	f.QuickLoad("left", []string{"..", "Test.txt", "te.txt", "report.pdf", "Árvíz.doc", "docs/", "data.bin"})
	o := FindQuickOptions{Beginning: true, Wildcards: true, IgnoreAccents: true}
	check := func(text string, o FindQuickOptions, want []int) {
		t.Helper()
		if got := f.QuickMatch("left", text, o); !reflect.DeepEqual(got, want) {
			t.Errorf("%q %+v: got %v, want %v", text, o, got, want)
		}
	}
	check("te", o, []int{1, 2})
	check("te.tx", o, []int{1, 2}) // the extension rule: te*.tx*
	mo := o
	mo.MatchEnd = true
	check("te.tx", mo, []int{2})
	check("arv", o, []int{4})
	check("d*c", o, []int{5})
	ko := o
	ko.Kind = "dirs"
	check("d", ko, []int{5})
	check("port", o, []int{})
	ao := o
	ao.Beginning = false
	check("port", ao, []int{3})
	fo := FindQuickOptions{Filter: true, Wildcards: true, IgnoreAccents: true, FilterDirs: false}
	check("txt", fo, []int{1, 2, 5}) // folders stay when folders are not filtered
	no := o
	no.Wildcards = false
	check("d*", no, []int{})
}
