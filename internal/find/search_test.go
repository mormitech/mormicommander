// Tests for the folder walk and the filter chain, always on a fresh tree under
// t.TempDir(): masks and folder exclusions, depth limits, files/folders only, regex
// names, size, date, age, attributes, permissions, symbolic links and loops, content
// search on real files, archives through a fake handler, refining within results,
// single-file matching, remote-style file systems, errors and cancellation.

package find

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

// mkTree creates files (content given) and folders (names ending in "/").
func mkTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if strings.HasSuffix(name, "/") {
			if err := os.MkdirAll(p, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// rels runs a search and returns the sorted relative paths of the results.
func rels(t *testing.T, o Options) []string {
	t.Helper()
	res, _, err := Search(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(res))
	for _, r := range res {
		out = append(out, r.Rel)
	}
	sort.Strings(out)
	return out
}

func eq(t *testing.T, label string, got, want []string) {
	t.Helper()
	if want == nil {
		want = []string{}
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("%s:\n got  %q\n want %q", label, got, want)
	}
}

var sampleTree = map[string]string{
	"a.txt":                  "alpha",
	"old_a.txt":              "old alpha",
	"b.go":                   "package b",
	"b_test.go":              "package b",
	"docs/":                  "",
	"docs/readme.md":         "# readme",
	"docs/guide.txt":         "guide text",
	"src/main.go":            "package main\nfunc main() {}",
	"src/vendor/lib/lib.go":  "package lib",
	"src/sub/deep/deep.go":   "package deep",
	"src/sub/deep/notes.txt": "deep notes",
	"vendor/x.go":            "package x",
	".git/config":            "[core]",
	"Makefile":               "all:",
}

func TestSearchMasksAndExclusions(t *testing.T) {
	root := mkTree(t, sampleTree)
	markHidden(t, filepath.Join(root, ".git"))
	base := DefaultOptions()
	base.SearchIn = []string{root}
	cases := []struct {
		name string
		mod  func(o *Options)
		want []string
	}{
		{"txt minus old", func(o *Options) { o.SearchFor = "*.txt|old*" },
			[]string{"a.txt", "docs/guide.txt", "src/sub/deep/notes.txt"}},
		{"go minus vendor folders", func(o *Options) { o.SearchFor = "*.go|vendor/"; o.Kind = KindFiles },
			[]string{"b.go", "b_test.go", "src/main.go", "src/sub/deep/deep.go"}},
		{"go minus tests and vendor", func(o *Options) { o.SearchFor = "*.go|*_test.go vendor/" },
			[]string{"b.go", "src/main.go", "src/sub/deep/deep.go"}},
		{"no wildcard is substring", func(o *Options) { o.SearchFor = "read" }, []string{"docs/readme.md"}},
		{"no extension", func(o *Options) { o.SearchFor = "*."; o.Kind = KindFiles }, []string{".git/config", "Makefile"}},
		{"folders only", func(o *Options) { o.Kind = KindDirs; o.SearchFor = "*|.git/" },
			[]string{"docs", "src", "src/sub", "src/sub/deep", "src/vendor", "src/vendor/lib", "vendor"}},
		{"ExcludeDirs option", func(o *Options) { o.SearchFor = "*.go"; o.ExcludeDirs = []string{"vendor", "src/sub/"} },
			[]string{"b.go", "b_test.go", "src/main.go"}},
		{"path mask", func(o *Options) { o.SearchFor = "src/*.go|vendor/" }, []string{"src/main.go", "src/sub/deep/deep.go"}},
		{"regex names", func(o *Options) { o.NameRegex = true; o.SearchFor = `^[ab](_test)?\.GO$` },
			[]string{"b.go", "b_test.go"}},
		{"regex names case-sensitive", func(o *Options) { o.NameRegex = true; o.NameRegexCase = true; o.SearchFor = `\.GO$` }, nil},
		{"skip hidden", func(o *Options) { o.SearchFor = "config"; o.SkipHidden = true }, nil},
		{"hidden found by default", func(o *Options) { o.SearchFor = "config" }, []string{".git/config"}},
	}
	for _, c := range cases {
		o := base
		c.mod(&o)
		eq(t, c.name, rels(t, o), c.want)
	}
}

func TestSearchDepth(t *testing.T) {
	root := mkTree(t, map[string]string{
		"l0.txt": "", "d1/l1.txt": "", "d1/d2/l2.txt": "", "d1/d2/d3/l3.txt": "",
	})
	cases := []struct {
		levels int
		want   []string
	}{
		{0, []string{"l0.txt"}},
		{1, []string{"d1/l1.txt", "l0.txt"}},
		{2, []string{"d1/d2/l2.txt", "d1/l1.txt", "l0.txt"}},
		{Unlimited, []string{"d1/d2/d3/l3.txt", "d1/d2/l2.txt", "d1/l1.txt", "l0.txt"}},
	}
	for _, c := range cases {
		o := Options{SearchFor: "*.txt", SearchIn: []string{root}, Levels: c.levels}
		eq(t, "levels", rels(t, o), c.want)
	}
}

func TestSearchMultipleRootsAndFileRoot(t *testing.T) {
	r1 := mkTree(t, map[string]string{"one.txt": "x"})
	r2 := mkTree(t, map[string]string{"two.txt": "x", "skip.md": ""})
	o := DefaultOptions()
	o.SearchFor = "*.txt"
	o.SearchIn = []string{r1, filepath.Join(r2, "two.txt"), filepath.Join(r2, "missing")}
	var errs []string
	q, err := Compile(o)
	if err != nil {
		t.Fatal(err)
	}
	var got []Result
	st, err := q.Run(context.Background(), Handler{
		Result: func(r Result) error { got = append(got, r); return nil },
		Error:  func(p string, e error) { errs = append(errs, p) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Root != r1 || got[1].Path != filepath.Join(r2, "two.txt") {
		t.Errorf("results: %+v", got)
	}
	if len(errs) != 1 || st.Errors != 1 || st.Matches != 2 {
		t.Errorf("errs %v stats %+v", errs, st)
	}
}

func TestSearchSizeFilter(t *testing.T) {
	root := mkTree(t, map[string]string{
		"zero":  "",
		"small": strings.Repeat("x", 100),
		"k3":    strings.Repeat("x", 3*1024+5),
		"k4":    strings.Repeat("x", 4*1024),
		"dir/":  "",
	})
	cases := []struct {
		f    SizeFilter
		want []string
	}{
		{SizeFilter{Op: SizeEqual, Value: 100}, []string{"small"}},
		{SizeFilter{Op: SizeEqual, Value: 3, Unit: KB}, []string{"k3"}},
		{SizeFilter{Op: SizeGreater, Value: 3, Unit: KB}, []string{"k3", "k4"}},
		{SizeFilter{Op: SizeLess, Value: 100}, []string{"zero"}},
		{SizeFilter{Op: SizeLess, Value: 1, Unit: MB}, []string{"k3", "k4", "small", "zero"}},
		{SizeFilter{}, []string{"dir", "k3", "k4", "small", "zero"}},
	}
	for _, c := range cases {
		o := DefaultOptions()
		o.SearchIn = []string{root}
		o.Size = c.f
		eq(t, "size "+string(c.f.Op), rels(t, o), c.want)
	}
	o := DefaultOptions()
	o.SearchIn = []string{root}
	o.Size = SizeFilter{Op: "~", Value: 1}
	if _, err := Compile(o); err == nil {
		t.Error("bad op accepted")
	}
	o.Size = SizeFilter{Op: SizeLess, Value: 1, Unit: "PB"}
	if _, err := Compile(o); err == nil {
		t.Error("bad unit accepted")
	}
}

func TestSearchDateFilter(t *testing.T) {
	root := mkTree(t, map[string]string{"new": "", "hour": "", "week": "", "year": ""})
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	set := func(name string, t0 time.Time) {
		if err := os.Chtimes(filepath.Join(root, name), t0, t0); err != nil {
			t.Fatal(err)
		}
	}
	set("new", now.Add(-time.Minute))
	set("hour", now.Add(-90*time.Minute))
	set("week", now.AddDate(0, 0, -8))
	set("year", now.AddDate(-1, 0, -1))
	cases := []struct {
		name string
		d    DateFilter
		want []string
	}{
		{"not older than 5 minutes", DateFilter{NotOlderThan: Age{5, Minutes}}, []string{"new"}},
		{"not older than 2 hours", DateFilter{NotOlderThan: Age{2, Hours}}, []string{"hour", "new"}},
		{"not older than 1 week", DateFilter{NotOlderThan: Age{1, Weeks}}, []string{"hour", "new"}},
		{"not older than 9 days", DateFilter{NotOlderThan: Age{9, Days}}, []string{"hour", "new", "week"}},
		{"not older than 1 month", DateFilter{NotOlderThan: Age{1, Months}}, []string{"hour", "new", "week"}},
		{"not older than 2 years", DateFilter{NotOlderThan: Age{2, Years}}, []string{"hour", "new", "week", "year"}},
		{"older than 1 year", DateFilter{OlderThan: Age{1, Years}}, []string{"year"}},
		{"between", DateFilter{From: now.AddDate(0, 0, -10), To: now.Add(-time.Hour)}, []string{"hour", "week"}},
		{"from only", DateFilter{From: now.Add(-2 * time.Hour)}, []string{"hour", "new"}},
	}
	for _, c := range cases {
		o := DefaultOptions()
		o.SearchIn = []string{root}
		o.Date = c.d
		o.Now = func() time.Time { return now }
		eq(t, c.name, rels(t, o), c.want)
	}
	o := DefaultOptions()
	o.SearchIn = []string{root}
	o.Date = DateFilter{NotOlderThan: Age{1, "fortnights"}}
	if _, err := Compile(o); err == nil {
		t.Error("bad age unit accepted")
	}
}

func TestSearchAttributesAndPermissions(t *testing.T) {
	root := mkTree(t, map[string]string{"rw.txt": "", "ro.txt": "", ".hidden": "", "run.sh": "", "dir/": ""})
	if err := os.Chmod(filepath.Join(root, "ro.txt"), 0o444); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(filepath.Join(root, "run.sh"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { os.Chmod(filepath.Join(root, "ro.txt"), 0o644) })
	type tc struct {
		name       string
		set, clear Attr
		want       []string
	}
	cases := []tc{
		{"read-only", AttrReadOnly, 0, []string{"ro.txt"}},
		{"not read-only files", 0, AttrReadOnly | AttrDirectory, []string{".hidden", "run.sh", "rw.txt"}},
		{"directories", AttrDirectory, 0, []string{"dir"}},
	}
	if runtime.GOOS != "windows" {
		cases = append(cases,
			tc{"hidden", AttrHidden, 0, []string{".hidden"}},
			tc{"executable", AttrExecutable, 0, []string{"run.sh"}})
	}
	for _, c := range cases {
		o := DefaultOptions()
		o.SearchIn = []string{root}
		o.AttrSet, o.AttrClear = c.set, c.clear
		eq(t, c.name, rels(t, o), c.want)
	}
	if runtime.GOOS != "windows" {
		o := DefaultOptions()
		o.SearchIn = []string{root}
		o.Kind = KindFiles
		o.PermMask, o.PermValue = 0o100, 0o100
		eq(t, "owner-executable", rels(t, o), []string{"run.sh"})
		o.PermMask, o.PermValue = 0o200, 0
		eq(t, "owner-readonly", rels(t, o), []string{"ro.txt"})
	}
}

func TestAttrString(t *testing.T) {
	a := AttrReadOnly | AttrHidden | AttrDirectory
	if s := a.String(); s != "rhd" {
		t.Errorf("String = %q", s)
	}
	if ParseAttr("RHD?") != a {
		t.Error("ParseAttr round trip failed")
	}
}

func TestSearchSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	root := mkTree(t, map[string]string{"real/file.txt": "1234567890", "real/sub/inner.txt": ""})
	outside := mkTree(t, map[string]string{"far.txt": "far"})
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "link")))
	must(os.Symlink(outside, filepath.Join(root, "real", "outlink")))
	must(os.Symlink(root, filepath.Join(root, "real", "sub", "loop"))) // loop to the root
	must(os.Symlink(filepath.Join(root, "real", "file.txt"), filepath.Join(root, "flink.txt")))
	must(os.Symlink(filepath.Join(root, "nowhere"), filepath.Join(root, "broken.txt")))

	o := DefaultOptions()
	o.SearchIn = []string{root}
	o.SearchFor = "*.txt"
	eq(t, "not following", rels(t, o), []string{"broken.txt", "flink.txt", "real/file.txt", "real/sub/inner.txt"})

	o.FollowLinks = true
	got := rels(t, o)
	want := []string{
		"broken.txt", "flink.txt",
		"link/file.txt", "link/outlink/far.txt", "link/sub/inner.txt",
		"real/file.txt", "real/outlink/far.txt", "real/sub/inner.txt",
	}
	eq(t, "following", got, want)

	// A followed link reports the target's size and keeps the symlink attribute.
	o.SearchFor = "flink.txt"
	res, _, err := Search(context.Background(), o)
	if err != nil || len(res) != 1 || res[0].Size != 10 || res[0].Attr&AttrSymlink == 0 {
		t.Errorf("flink: %+v %v", res, err)
	}
	// Symlink attribute filter without following.
	o.FollowLinks = false
	o.SearchFor = "*"
	o.AttrSet = AttrSymlink
	eq(t, "symlinks", rels(t, o), []string{"broken.txt", "flink.txt", "link", "real/outlink", "real/sub/loop"})
}

func TestSearchContentOnFiles(t *testing.T) {
	root := mkTree(t, map[string]string{
		"has.txt":   "The quick brown fox",
		"hasnt.txt": "lazy dog",
		"sub/":      "",
		"bin.dat":   string(append([]byte{0, 1, 2}, utf16le("brown")...)),
	})
	o := DefaultOptions()
	o.SearchIn = []string{root}
	o.Content = ContentOptions{Text: "BROWN"}
	eq(t, "contains", rels(t, o), []string{"has.txt"})
	o.Content.Encodings = EncUTF8 | EncUTF16LE
	eq(t, "contains utf16", rels(t, o), []string{"bin.dat", "has.txt"})
	o.Content.NotContaining = true
	eq(t, "not containing (folders never match)", rels(t, o), []string{"hasnt.txt"})
	o.Content = ContentOptions{Text: "brown", CaseSensitive: true, WholeWords: true}
	o.SearchFor = "*.txt"
	eq(t, "whole word", rels(t, o), []string{"has.txt"})
}

// fakeArchives treats *.fak files as archives whose lines are "name=content"; a name
// ending in "/" is a folder entry.
type fakeArchives struct{ walked int }

func (f *fakeArchives) IsArchive(p string, info fs.FileInfo) bool {
	return strings.HasSuffix(p, ".fak")
}
func (f *fakeArchives) Walk(ctx context.Context, fsys FS, p string, fn func(ArchiveEntry, func() (io.ReadCloser, error)) error) error {
	f.walked++
	rc, err := fsys.Open(p)
	if err != nil {
		return err
	}
	data, _ := io.ReadAll(rc)
	rc.Close()
	if strings.HasPrefix(string(data), "BROKEN") {
		return errors.New("damaged archive")
	}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		name, content, _ := strings.Cut(line, "=")
		e := ArchiveEntry{Name: name, Size: int64(len(content)), ModTime: time.Unix(1700000000, 0), IsDir: strings.HasSuffix(name, "/")}
		if err := fn(e, func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(content)), nil }); err != nil {
			return err
		}
	}
	return nil
}

func TestSearchArchives(t *testing.T) {
	root := mkTree(t, map[string]string{
		"pack.fak":   "docs/=\ndocs/a.txt=hello archive\nvendor/v.txt=hello vendor\nb.md=nothing\n./c.txt=hello dot",
		"bad.fak":    "BROKEN",
		"plain.txt":  "hello disk",
		"sub/in.fak": "deep.txt=hello deep",
	})
	fa := &fakeArchives{}
	o := DefaultOptions()
	o.SearchIn = []string{root}
	o.SearchFor = "*.txt|vendor/"
	o.SearchArchives = true
	o.Archives = fa
	var errs []string
	q, err := Compile(o)
	if err != nil {
		t.Fatal(err)
	}
	var res []Result
	_, err = q.Run(context.Background(), Handler{
		Result: func(r Result) error { res = append(res, r); return nil },
		Error:  func(p string, e error) { errs = append(errs, filepath.Base(p)+": "+e.Error()) },
	})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range res {
		got = append(got, r.Rel)
		if r.Archive != "" {
			if r.Path != filepath.Join(r.Archive, filepath.FromSlash(r.Inner)) {
				t.Errorf("archive path %q vs %q + %q", r.Path, r.Archive, r.Inner)
			}
		}
	}
	sort.Strings(got)
	eq(t, "archive names", got, []string{"pack.fak/c.txt", "pack.fak/docs/a.txt", "plain.txt", "sub/in.fak/deep.txt"})
	if len(errs) != 1 || !strings.Contains(errs[0], "bad.fak") {
		t.Errorf("errors: %v", errs)
	}

	// Content search reaches into archive members.
	o.Content = ContentOptions{Text: "hello a"}
	eq(t, "archive content", rels(t, o), []string{"pack.fak/docs/a.txt"})

	// Without the option nothing inside archives is listed.
	o.Content = ContentOptions{}
	o.SearchArchives = false
	eq(t, "archives off", rels(t, o), []string{"plain.txt"})
}

func TestSearchArchiveStopPropagates(t *testing.T) {
	root := mkTree(t, map[string]string{"p.fak": "a.txt=1\nb.txt=2\nc.txt=3"})
	o := DefaultOptions()
	o.SearchIn = []string{root}
	o.SearchArchives = true
	o.Archives = &fakeArchives{}
	q, _ := Compile(o)
	n := 0
	boom := errors.New("boom")
	_, err := q.Run(context.Background(), Handler{Result: func(r Result) error {
		n++
		if r.Inner == "a.txt" {
			return boom
		}
		return nil
	}})
	if !errors.Is(err, boom) || n != 2 { // p.fak itself, then a.txt
		t.Errorf("err %v n %d", err, n)
	}
}

func TestSearchWithinResults(t *testing.T) {
	root := mkTree(t, map[string]string{"a.txt": "red", "b.txt": "blue", "c.md": "red", "d/": ""})
	o := DefaultOptions()
	o.Within = []string{filepath.Join(root, "a.txt"), filepath.Join(root, "b.txt"), filepath.Join(root, "c.md"), filepath.Join(root, "d"), filepath.Join(root, "gone")}
	o.Content = ContentOptions{Text: "red"}
	var errs int
	q, err := Compile(o)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	_, err = q.Run(context.Background(), Handler{
		Result: func(r Result) error { got = append(got, r.Name); return nil },
		Error:  func(string, error) { errs++ },
	})
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "refine", got, []string{"a.txt", "c.md"})
	if errs != 1 {
		t.Errorf("errs = %d", errs)
	}
	o.Content = ContentOptions{}
	o.SearchFor = "*.txt"
	eq(t, "refine by name", rels(t, o), []string{"a.txt", "b.txt"})
}

func TestQueryMatch(t *testing.T) {
	root := mkTree(t, map[string]string{"x.log": "error here", "y.log": "fine"})
	o := DefaultOptions()
	o.SearchIn = []string{root}
	o.SearchFor = "*.log"
	o.Content = ContentOptions{Text: "error"}
	q, err := Compile(o)
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]bool{"x.log": true, "y.log": false} {
		got, err := q.Match(context.Background(), filepath.Join(root, name))
		if err != nil || got != want {
			t.Errorf("Match(%s) = %v, %v", name, got, err)
		}
	}
	if _, err := q.Match(context.Background(), filepath.Join(root, "none")); err == nil {
		t.Error("missing file: want error")
	}
}

func TestCompileErrors(t *testing.T) {
	cases := []Options{
		{SearchFor: "*"}, // no folder
		{SearchFor: "a|b|c", SearchIn: []string{"."}},              // two pipes
		{SearchFor: "(", NameRegex: true, SearchIn: []string{"."}}, // bad regex
		{SearchIn: []string{"."}, Content: ContentOptions{Text: "(", Regex: true}},
		{SearchIn: []string{"."}, Duplicates: DupOptions{Enabled: true, Hash: "sha3"}},
	}
	for i, o := range cases {
		if _, err := Compile(o); err == nil {
			t.Errorf("case %d: want error", i)
		}
	}
}

func TestSearchCancelAndStop(t *testing.T) {
	files := map[string]string{}
	for i := 0; i < 50; i++ {
		files[filepath.ToSlash(filepath.Join("d", string(rune('a'+i%26)), strings.Repeat("f", i%7+1)+".txt"))] = "x"
	}
	root := mkTree(t, files)
	o := DefaultOptions()
	o.SearchIn = []string{root}

	// ErrStop ends without an error after the first hit.
	q, _ := Compile(o)
	n := 0
	_, err := q.Run(context.Background(), Handler{Result: func(Result) error { n++; return ErrStop }})
	if err != nil || n != 1 {
		t.Errorf("ErrStop: err %v n %d", err, n)
	}
	// A cancelled context returns its error.
	ctx, cancel := context.WithCancel(context.Background())
	n = 0
	_, err = q.Run(ctx, Handler{Result: func(Result) error {
		n++
		if n == 3 {
			cancel()
		}
		return nil
	}})
	if !errors.Is(err, context.Canceled) || n != 3 {
		t.Errorf("cancel: err %v n %d", err, n)
	}
}

func TestSearchUnreadableFolder(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permission bits do not stop this user")
	}
	root := mkTree(t, map[string]string{"open/a.txt": "", "closed/b.txt": ""})
	closed := filepath.Join(root, "closed")
	if err := os.Chmod(closed, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(closed, 0o755) })
	o := DefaultOptions()
	o.SearchIn = []string{root}
	o.SearchFor = "*.txt"
	q, _ := Compile(o)
	var errs []string
	var got []string
	_, err := q.Run(context.Background(), Handler{
		Result: func(r Result) error { got = append(got, r.Rel); return nil },
		Error:  func(p string, e error) { errs = append(errs, p) },
	})
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "rest still found", got, []string{"open/a.txt"})
	if len(errs) != 1 || errs[0] != closed {
		t.Errorf("errors: %v", errs)
	}
}

func TestSearchProgress(t *testing.T) {
	root := mkTree(t, map[string]string{"a/1": "", "b/2": ""})
	o := DefaultOptions()
	o.SearchIn = []string{root}
	q, _ := Compile(o)
	var dirs []string
	st, err := q.Run(context.Background(), Handler{Progress: func(p Progress) { dirs = append(dirs, p.Dir) }})
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 3 || st.Dirs != 3 || st.Files != 4 || st.Matches != 4 {
		t.Errorf("dirs %v stats %+v", dirs, st)
	}
}

// mapFS adapts fstest.MapFS to FS with slash paths, standing in for an SFTP or FTP
// connection: no operating system attributes, forward slashes, no symlinks.
type mapFS struct {
	SlashFS
	m fstest.MapFS
}

func (f mapFS) clean(p string) string {
	p = strings.TrimPrefix(path.Clean(p), "/")
	if p == "" {
		return "."
	}
	return p
}
func (f mapFS) Lstat(p string) (fs.FileInfo, error)     { return fs.Stat(f.m, f.clean(p)) }
func (f mapFS) Stat(p string) (fs.FileInfo, error)      { return fs.Stat(f.m, f.clean(p)) }
func (f mapFS) ReadDir(p string) ([]fs.DirEntry, error) { return fs.ReadDir(f.m, f.clean(p)) }
func (f mapFS) Open(p string) (io.ReadCloser, error)    { return f.m.Open(f.clean(p)) }

func TestSearchRemoteFS(t *testing.T) {
	m := mapFS{m: fstest.MapFS{
		"home/u/a.txt":      {Data: []byte("remote hello")},
		"home/u/.profile":   {Data: []byte("x")},
		"home/u/sub/b.txt":  {Data: []byte("bye"), Mode: 0o444},
		"home/u/sub/c.conf": {Data: []byte("hello")},
	}}
	o := DefaultOptions()
	o.FS = m
	o.SearchIn = []string{"/home/u"}
	o.SearchFor = "*.txt"
	res, _, err := Search(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 || res[1].Path != "/home/u/sub/b.txt" || res[1].Attr&AttrReadOnly == 0 {
		t.Errorf("remote: %+v", res)
	}
	o.SearchFor = "*"
	o.Content = ContentOptions{Text: "hello"}
	eq(t, "remote content", rels(t, o), []string{"a.txt", "sub/c.conf"})
	o.Content = ContentOptions{}
	o.AttrSet = AttrHidden
	eq(t, "remote hidden", rels(t, o), []string{".profile"})
}

func TestArchiveHelpers(t *testing.T) {
	is := ArchiveExtensions(".zip", ".TAR.GZ")
	for p, want := range map[string]bool{"a.ZIP": true, "x.tar.gz": true, "x.gz": false, "zip": false} {
		if is(p, nil) != want {
			t.Errorf("ArchiveExtensions(%q) != %v", p, want)
		}
	}
	for in, want := range map[string]string{"./a/b": "a/b", "/a/": "a", `a\b`: "a/b", "../x": "x", "": ""} {
		if got := cleanInner(in); got != want {
			t.Errorf("cleanInner(%q) = %q want %q", in, got, want)
		}
	}
	af := ArchiveFuncs{Is: is, WalkFunc: func(context.Context, FS, string, func(ArchiveEntry, func() (io.ReadCloser, error)) error) error {
		return nil
	}}
	if !af.IsArchive("a.zip", nil) || af.Walk(context.Background(), OSFS{}, "a.zip", nil) != nil {
		t.Error("ArchiveFuncs")
	}
}

// TestContentSearchSkipsLinkedFolder searches text without following links in
// a tree that holds a symbolic link to a folder. The link is not a file and
// has no text, like the folder itself, so it must neither be opened (reading a
// folder fails with "is a directory", which was reported as an error for every
// such link) nor be hashed as a duplicate candidate.
func TestContentSearchSkipsLinkedFolder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	// Targets of equal length give both links the same size, so the duplicate
	// search groups them and would try to read them.
	root := mkTree(t, map[string]string{"dir1/a.txt": "a needle here", "dir2/b.txt": "a needle here"})
	if err := os.Symlink(filepath.Join(root, "dir1"), filepath.Join(root, "link1")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "dir2"), filepath.Join(root, "link2")); err != nil {
		t.Fatal(err)
	}
	for _, notContaining := range []bool{false, true} {
		o := DefaultOptions()
		o.SearchIn = []string{root}
		o.Content.Text = "needle"
		o.Content.NotContaining = notContaining
		var errs []string
		q, err := Compile(o)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := q.Run(context.Background(), Handler{Error: func(p string, e error) { errs = append(errs, p+": "+e.Error()) }}); err != nil {
			t.Fatal(err)
		}
		if len(errs) > 0 {
			t.Errorf("notContaining=%v: errors %v", notContaining, errs)
		}
	}
	o := DefaultOptions()
	o.SearchIn = []string{root}
	o.Duplicates = DupOptions{Enabled: true, Size: true, Content: true}
	var errs []string
	q, err := Compile(o)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	if _, err := q.Run(context.Background(), Handler{
		Result: func(r Result) error { got = append(got, r.Rel); return nil },
		Error:  func(p string, e error) { errs = append(errs, p+": "+e.Error()) },
	}); err != nil {
		t.Fatal(err)
	}
	if len(errs) > 0 {
		t.Errorf("duplicates: errors %v", errs)
	}
	for _, g := range got {
		if strings.HasPrefix(g, "link") {
			t.Errorf("duplicates: link to a folder listed: %v", got)
		}
	}
}
