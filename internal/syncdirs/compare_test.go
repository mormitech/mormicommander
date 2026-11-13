// Tests for the compare half: categories and default actions under every
// option of the Synchronize dialog, name pairing, selection and filters,
// unreadable folders, symbolic links, read-only io/fs sides, progress,
// cancellation and re-comparing a single row.
package syncdirs

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCompareCategories(t *testing.T) {
	baseL := map[string]spec{
		"eq.txt": file("same", 0),
		"ln.txt": file("v2", 10*time.Minute),
		"rn.txt": file("v1", 0),
		"lo.txt": file("left", 0),
		"ue.txt": file("abc", 0),
	}
	baseR := map[string]spec{
		"eq.txt": file("same", time.Second),
		"ln.txt": file("v1", 0),
		"rn.txt": file("v2", 10*time.Minute),
		"ro.txt": file("right", 0),
		"ue.txt": file("abcd", 0),
	}
	strict := DefaultOptions()
	strict.ToleranceSeconds = 0
	asym := DefaultOptions()
	asym.Asymmetric = true
	cases := []struct {
		name string
		opts Options
		want []string
	}{
		{"symmetric defaults", DefaultOptions(), []string{
			"eq.txt equal none",
			"ln.txt leftNewer copyLeftToRight",
			"lo.txt leftOnly copyLeftToRight",
			"rn.txt rightNewer copyRightToLeft",
			"ro.txt rightOnly copyRightToLeft",
			"ue.txt unequal none",
		}},
		{"asymmetric makes right a copy of left", asym, []string{
			"eq.txt equal none",
			"ln.txt leftNewer copyLeftToRight",
			"lo.txt leftOnly copyLeftToRight",
			"rn.txt rightNewer copyLeftToRight",
			"ro.txt rightOnly deleteRight",
			"ue.txt unequal copyLeftToRight",
		}},
		{"no tolerance sees one second", strict, []string{
			"eq.txt rightNewer copyRightToLeft",
			"ln.txt leftNewer copyLeftToRight",
			"lo.txt leftOnly copyLeftToRight",
			"rn.txt rightNewer copyRightToLeft",
			"ro.txt rightOnly copyRightToLeft",
			"ue.txt unequal none",
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l, r := trees(t, baseL, baseR)
			equalLines(t, rows(mustCompare(t, l, r, c.opts)), c.want)
		})
	}
}

func TestCompareTimeAndContentOptions(t *testing.T) {
	type pairSpec struct{ l, r spec }
	cases := []struct {
		name string
		opts func(*Options)
		p    pairSpec
		want Category
	}{
		{"2s FAT tolerance", nil, pairSpec{file("x", 0), file("x", 2*time.Second)}, Equal},
		{"3s is newer", nil, pairSpec{file("x", 0), file("x", 3*time.Second)}, RightNewer},
		{"exact hour without option", nil, pairSpec{file("x", time.Hour), file("x", 0)}, LeftNewer},
		{"exact hour ignored", func(o *Options) { o.IgnoreHours = 1 }, pairSpec{file("x", time.Hour), file("x", 0)}, Equal},
		{"hour plus 1s ignored", func(o *Options) { o.IgnoreHours = 1 }, pairSpec{file("x", 0), file("x", time.Hour+time.Second)}, Equal},
		{"hour plus 5s not ignored", func(o *Options) { o.IgnoreHours = 1 }, pairSpec{file("x", 0), file("x", time.Hour+5*time.Second)}, RightNewer},
		{"two hours with IgnoreHours 2", func(o *Options) { o.IgnoreHours = 2 }, pairSpec{file("x", 2*time.Hour), file("x", 0)}, Equal},
		{"two hours with IgnoreHours 1", func(o *Options) { o.IgnoreHours = 1 }, pairSpec{file("x", 2*time.Hour), file("x", 0)}, LeftNewer},
		{"half hour never ignored", func(o *Options) { o.IgnoreHours = 3 }, pairSpec{file("x", 30*time.Minute), file("x", 0)}, LeftNewer},
		{"same date other size", nil, pairSpec{file("ab", 0), file("abc", 0)}, Unequal},
		{"same size same date other content", nil, pairSpec{file("abc", 0), file("xyz", 0)}, Equal},
		{"ignore date same size", func(o *Options) { o.IgnoreDate = true }, pairSpec{file("abc", 0), file("abc", time.Hour)}, Equal},
		{"ignore date other size", func(o *Options) { o.IgnoreDate = true }, pairSpec{file("ab", time.Hour), file("abc", 0)}, Unequal},
		{"content: same date other content", func(o *Options) { o.ByContent = true }, pairSpec{file("abc", 0), file("xyz", 0)}, Unequal},
		{"content: same content other date", func(o *Options) { o.ByContent = true }, pairSpec{file("abc", 0), file("abc", time.Hour)}, Equal},
		{"content: other content newer left", func(o *Options) { o.ByContent = true }, pairSpec{file("abc", time.Hour), file("xyz", 0)}, LeftNewer},
		{"content: other size newer right", func(o *Options) { o.ByContent = true }, pairSpec{file("ab", 0), file("xyz", time.Hour)}, RightNewer},
		{"content + ignore date differs", func(o *Options) { o.ByContent = true; o.IgnoreDate = true }, pairSpec{file("abc", time.Hour), file("xyz", 0)}, Unequal},
		{"content: empty files", func(o *Options) { o.ByContent = true }, pairSpec{file("", 0), file("", time.Hour)}, Equal},
		{"content: large equal", func(o *Options) { o.ByContent = true }, pairSpec{file(strings.Repeat("z", 3*chunkSize+7), 0), file(strings.Repeat("z", 3*chunkSize+7), time.Hour)}, Equal},
		{"content: large differ at end", func(o *Options) { o.ByContent = true }, pairSpec{file(strings.Repeat("z", 3*chunkSize)+"a", 0), file(strings.Repeat("z", 3*chunkSize)+"b", 0)}, Unequal},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l, r := trees(t, map[string]spec{"f": c.p.l}, map[string]spec{"f": c.p.r})
			o := DefaultOptions()
			if c.opts != nil {
				c.opts(&o)
			}
			res := mustCompare(t, l, r, o)
			if len(res.Items) != 1 {
				t.Fatalf("items = %v", rows(res))
			}
			it := res.Items[0]
			if it.Category != c.want {
				t.Errorf("category = %s, want %s", it.Category, c.want)
			}
			if o.ByContent && it.Left.Size == it.Right.Size && !it.ContentCompared {
				t.Errorf("content was not compared")
			}
		})
	}
}

func TestCompareTrees(t *testing.T) {
	noSub := DefaultOptions()
	noSub.Subdirs = false
	empty := DefaultOptions()
	empty.IgnoreEmptyDirs = true
	ci := DefaultOptions()
	ci.Case = CaseInsensitive
	cs := DefaultOptions()
	cs.Case = CaseSensitive
	sel := DefaultOptions()
	sel.Case = CaseInsensitive
	sel.OnlySelected = true
	sel.Selected = []string{"a.txt", "D"}
	filt := DefaultOptions()
	filt.Filter = `*.txt | skip\`

	cases := []struct {
		name        string
		left, right map[string]spec
		opts        Options
		want        []string
	}{
		{
			name:  "one-sided folder lists its contents",
			left:  map[string]spec{"d/x.txt": file("1", 0), "d/sub/y.txt": file("2", 0)},
			right: map[string]spec{},
			opts:  DefaultOptions(),
			want: []string{
				"d/ leftOnly copyLeftToRight",
				"d/sub/ leftOnly copyLeftToRight",
				"d/sub/y.txt leftOnly copyLeftToRight",
				"d/x.txt leftOnly copyLeftToRight",
			},
		},
		{
			name:  "subdirs off ignores folders",
			left:  map[string]spec{"a.txt": file("1", 0), "d/x.txt": file("1", 0)},
			right: map[string]spec{"a.txt": file("1", 0), "e/": {}},
			opts:  noSub,
			want:  []string{"a.txt equal none"},
		},
		{
			name:  "empty folders shown by default",
			left:  map[string]spec{"e/": {}, "f/g/": {}, "h/x": file("1", 0)},
			right: map[string]spec{},
			opts:  DefaultOptions(),
			want: []string{
				"e/ leftOnly copyLeftToRight",
				"f/ leftOnly copyLeftToRight",
				"f/g/ leftOnly copyLeftToRight",
				"h/ leftOnly copyLeftToRight",
				"h/x leftOnly copyLeftToRight",
			},
		},
		{
			name:  "ignore empty folders",
			left:  map[string]spec{"e/": {}, "f/g/": {}, "h/x": file("1", 0)},
			right: map[string]spec{"e/": {}},
			opts:  empty,
			want:  []string{"h/ leftOnly copyLeftToRight", "h/x leftOnly copyLeftToRight"},
		},
		{
			name:  "file against folder is a type mismatch",
			left:  map[string]spec{"t": file("1", 0)},
			right: map[string]spec{"t/inner": file("1", 0)},
			opts:  DefaultOptions(),
			want:  []string{"t unequal none"},
		},
		{
			name:  "case-insensitive pairing",
			left:  map[string]spec{"Docs/A.txt": file("1", 0), "Docs/new.txt": file("n", 0)},
			right: map[string]spec{"docs/a.txt": file("1", 0)},
			opts:  ci,
			want:  []string{"Docs/ equal none", "Docs/A.txt equal none", "Docs/new.txt leftOnly copyLeftToRight"},
		},
		{
			name:  "case-sensitive pairing",
			left:  map[string]spec{"Docs/A.txt": file("1", 0)},
			right: map[string]spec{"docs/a.txt": file("1", 0)},
			opts:  cs,
			want: []string{
				"Docs/ leftOnly copyLeftToRight",
				"Docs/A.txt leftOnly copyLeftToRight",
				"docs/ rightOnly copyRightToLeft",
				"docs/a.txt rightOnly copyRightToLeft",
			},
		},
		{
			name:  "only selected",
			left:  map[string]spec{"a.txt": file("1", 0), "b.txt": file("1", 0), "D/x": file("1", 0)},
			right: map[string]spec{"c.txt": file("1", 0), "d/": {}},
			opts:  sel,
			want:  []string{"a.txt leftOnly copyLeftToRight", "D/ equal none", "D/x leftOnly copyLeftToRight"},
		},
		{
			name:  "filter",
			left:  map[string]spec{"a.txt": file("1", 0), "b.doc": file("1", 0), "skip/c.txt": file("1", 0), "k/d.txt": file("1", 0)},
			right: map[string]spec{},
			opts:  filt,
			want:  []string{"a.txt leftOnly copyLeftToRight", "k/ leftOnly copyLeftToRight", "k/d.txt leftOnly copyLeftToRight"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l, r := trees(t, c.left, c.right)
			equalLines(t, rows(mustCompare(t, l, r, c.opts)), c.want)
		})
	}
}

func TestWouldBePathKeepsOtherSideCase(t *testing.T) {
	o := DefaultOptions()
	o.Case = CaseInsensitive
	l, r := trees(t, map[string]spec{"Docs/new.txt": file("n", 0)}, map[string]spec{"docs/": {}})
	res := mustCompare(t, l, r, o)
	it := res.Find("Docs/new.txt")
	if it == nil {
		t.Fatalf("missing row: %v", rows(res))
	}
	if it.RightPath != "docs/new.txt" {
		t.Errorf("RightPath = %q, want docs/new.txt", it.RightPath)
	}
}

func TestCompareRootErrors(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "file")
	if err := os.WriteFile(f, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct{ l, r string }{
		{filepath.Join(dir, "missing"), dir},
		{dir, filepath.Join(dir, "missing")},
		{f, dir},
	}
	for _, c := range cases {
		if _, err := Compare(context.Background(), DirFS(c.l), DirFS(c.r), DefaultOptions(), nil); err == nil {
			t.Errorf("Compare(%s, %s) succeeded, want error", c.l, c.r)
		}
	}
}

func TestUnreadableSubfolderIsSkipped(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs Unix permissions and a non-root user")
	}
	l, r := trees(t,
		map[string]spec{"u/x": file("1", 0), "ok.txt": file("1", 0)},
		map[string]spec{"u/y": file("1", 0), "ok.txt": file("1", 0)})
	locked := filepath.Join(l, "u")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	o := DefaultOptions()
	o.Asymmetric = true // would delete u/y if the listing error were ignored
	res := mustCompare(t, l, r, o)
	equalLines(t, rows(res), []string{"ok.txt equal none", "u/ equal none"})
	if len(res.Errors) != 1 || res.Errors[0].Side != LeftSide {
		t.Errorf("errors = %v", res.Errors)
	}
	if res.Find("u").Note == "" {
		t.Errorf("folder row has no note")
	}
}

func TestSymlinksNotFollowed(t *testing.T) {
	outside := t.TempDir()
	build(t, outside, map[string]spec{"big.txt": file("outside", 0)})
	l, r := trees(t,
		map[string]spec{"same": {link: "t1"}, "diff": {link: "t1"}, "out": {link: outside}},
		map[string]spec{"same": {link: "t1"}, "diff": {link: "t2"}})
	res := mustCompare(t, l, r, DefaultOptions())
	equalLines(t, rows(res), []string{
		"diff unequal none",
		"out leftOnly copyLeftToRight",
		"same equal none",
	})
	out := res.Find("out")
	if out.Left.Kind != KindLink || out.Left.LinkTarget != outside || out.IsDir {
		t.Errorf("out entry = %+v", out.Left)
	}
}

func TestSymlinkLoopDetected(t *testing.T) {
	l, r := trees(t,
		map[string]spec{"a/f": file("1", 0), "a/loop": {link: ".."}},
		map[string]spec{"a/f": file("1", 0)})
	o := DefaultOptions()
	o.FollowSymlinks = true
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	res, err := Compare(ctx, DirFS(l), DirFS(r), o, nil)
	if err != nil {
		t.Fatal(err)
	}
	loop := res.Find("a/loop")
	if loop == nil || !strings.Contains(loop.Note, "loop") {
		t.Fatalf("loop row = %+v, rows %v", loop, rows(res))
	}
	for _, it := range res.Items {
		if strings.HasPrefix(it.Path, "a/loop/") {
			t.Errorf("entered loop: %s", it.Path)
		}
	}
}

func TestFollowedLinkSidesAreProtected(t *testing.T) {
	ext := t.TempDir()
	build(t, ext, map[string]spec{"old.txt": file("x", 0)})
	l, r := trees(t,
		map[string]spec{"sub/f.txt": file("1", 0)},
		map[string]spec{"sub": {link: ext}})
	o := DefaultOptions()
	o.FollowSymlinks = true
	res := mustCompare(t, l, r, o)
	f := res.Find("sub/f.txt")
	old := res.Find("sub/old.txt")
	if f == nil || old == nil {
		t.Fatalf("rows = %v", rows(res))
	}
	if f.Action != NoAction || f.CanApply(CopyLeftToRight) {
		t.Errorf("copy into followed link allowed: %+v", f)
	}
	if old.CanApply(DeleteRight) || old.CanApply(CopyRightToLeft) != true {
		t.Errorf("delete inside followed link allowed or reading from it refused")
	}
	if !old.Right.ViaLink {
		t.Errorf("entry not marked ViaLink")
	}
	if res.SetAction("sub/old.txt", DeleteRight) == nil {
		t.Errorf("SetAction allowed delete through link")
	}
}

func TestCompareReadOnlyZipSide(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range map[string]string{"a.txt": "alpha", "d/b.txt": "beta"} {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Modified: t0, Method: zip.Deflate})
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(data))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	right := t.TempDir()
	build(t, right, map[string]spec{"a.txt": file("alpha", 0), "extra.txt": file("e", 0)})
	zipSide := FromFS(zr)
	res, err := Compare(context.Background(), zipSide, DirFS(right), DefaultOptions(), nil)
	if err != nil {
		t.Fatal(err)
	}
	equalLines(t, rows(res), []string{
		"a.txt equal none",
		"d/ leftOnly copyLeftToRight",
		"d/b.txt leftOnly copyLeftToRight",
		"extra.txt rightOnly copyRightToLeft",
	})
	// Copying into the archive is impossible: Execute refuses up front.
	if _, err := Execute(context.Background(), res.Plan(AllOps()), zipSide, DirFS(right), ExecOptions{}); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("err = %v, want ErrReadOnly", err)
	}
	if _, err := os.Stat(filepath.Join(right, "d")); err == nil {
		t.Fatalf("something was copied despite ErrReadOnly")
	}
	// One-way from the archive works.
	rep, err := Execute(context.Background(), res.Plan(PlanFilter{LeftToRight: true}), zipSide, DirFS(right), ExecOptions{})
	if err != nil || len(rep.Errors) > 0 {
		t.Fatalf("Execute: %v %v", err, rep.Errors)
	}
	if got := readFile(t, filepath.Join(right, "d", "b.txt")); got != "beta" {
		t.Errorf("b.txt = %q", got)
	}
	res2, err := Compare(context.Background(), zipSide, DirFS(right), DefaultOptions(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if s := res2.Summary(); s.RightOnly != 1 || s.LeftOnly+s.LeftNewer+s.RightNewer+s.Unequal != 0 {
		t.Errorf("after one-way sync: %+v %v", s, rows(res2))
	}
}

func TestCompareProgressAndCancel(t *testing.T) {
	l, r := trees(t, map[string]spec{"a": file("1", 0), "d/b": file("2", 0)}, map[string]spec{"a": file("1", 0)})
	var last CompareProgress
	calls := 0
	_, err := Compare(context.Background(), DirFS(l), DirFS(r), DefaultOptions(), func(p CompareProgress) {
		calls++
		last = p
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls == 0 || last.Dirs != 2 || last.Files != 2 {
		t.Errorf("progress calls=%d last=%+v", calls, last)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Compare(ctx, DirFS(l), DirFS(r), DefaultOptions(), nil); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestRecompare(t *testing.T) {
	l, r := trees(t, map[string]spec{"f": file("abc", 0), "g": file("1", 0)}, map[string]spec{"f": file("abc", time.Hour)})
	res := mustCompare(t, l, r, DefaultOptions())
	if it := res.Find("f"); it.Category != RightNewer {
		t.Fatalf("f = %s", it.Category)
	}
	if err := os.Chtimes(filepath.Join(l, "f"), t0.Add(time.Hour), t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := res.SetAction("f", DeleteBoth); err != nil {
		t.Fatal(err)
	}
	if err := res.Recompare(context.Background(), "f"); err != nil {
		t.Fatal(err)
	}
	if it := res.Find("f"); it.Category != Equal || it.Action != NoAction {
		t.Errorf("after recompare f = %s %s", it.Category, it.Action)
	}
	if err := os.Remove(filepath.Join(l, "g")); err != nil {
		t.Fatal(err)
	}
	if err := res.Recompare(context.Background(), "g"); err != nil {
		t.Fatal(err)
	}
	if res.Find("g") != nil || len(res.Items) != 1 {
		t.Errorf("vanished pair still listed: %v", rows(res))
	}
	if err := res.Recompare(context.Background(), "nope"); err == nil {
		t.Errorf("recompare of unknown path succeeded")
	}
}

func TestActionOverrides(t *testing.T) {
	l, r := trees(t,
		map[string]spec{"d/a": file("1", 0), "d/b": file("2", 0), "both": file("x", time.Hour), "t": file("1", 0)},
		map[string]spec{"ro": file("r", 0), "both": file("x", 0), "t/": {}})
	res := mustCompare(t, l, r, DefaultOptions())
	cases := []struct {
		path string
		a    Action
		ok   bool
	}{
		{"ro", CopyLeftToRight, false},
		{"ro", DeleteRight, true},
		{"ro", DeleteLeft, false},
		{"ro", DeleteBoth, false},
		{"both", DeleteBoth, true},
		{"both", CopyRightToLeft, true},
		{"t", CopyLeftToRight, false},
		{"t", DeleteLeft, true},
		{"missing", NoAction, false},
	}
	for _, c := range cases {
		err := res.SetAction(c.path, c.a)
		if (err == nil) != c.ok {
			t.Errorf("SetAction(%s, %s) err = %v, want ok=%v", c.path, c.a, err, c.ok)
		}
	}
	n, err := res.SetActionTree("d", DeleteLeft)
	if err != nil || n != 3 {
		t.Errorf("SetActionTree changed %d (%v), want 3", n, err)
	}
	for _, p := range []string{"d", "d/a", "d/b"} {
		if a := res.Find(p).Action; a != DeleteLeft {
			t.Errorf("%s action = %s", p, a)
		}
	}
	if err := res.SetAction("d/a", CopyLeftToRight); err != nil {
		t.Fatal(err)
	}
	if err := res.ReverseAction("d/a"); err == nil {
		t.Errorf("reverse to copy right->left of a left-only file succeeded")
	}
	if err := res.SetAction("both", CopyLeftToRight); err != nil {
		t.Fatal(err)
	}
	if err := res.ReverseAction("both"); err != nil || res.Find("both").Action != CopyRightToLeft {
		t.Errorf("reverse failed: %v %s", err, res.Find("both").Action)
	}
	res.ResetActions()
	for _, it := range res.Items {
		if it.Action != it.Default {
			t.Errorf("%s not reset", it.Path)
		}
	}
	if n, _ := res.SetActionTree("", NoAction); n == 0 {
		t.Errorf("clearing all actions changed nothing")
	}
}

func TestShowFilterButtons(t *testing.T) {
	l, r := trees(t,
		map[string]spec{"eq": file("1", 0), "ln": file("1", time.Hour), "lo": file("1", 0), "d/x": file("1", 0), "ue": file("ab", 0), "e/": {}},
		map[string]spec{"eq": file("1", 0), "ln": file("1", 0), "ro": file("1", 0), "d/": {}, "ue": file("abc", 0), "e/": {}})
	res := mustCompare(t, l, r, DefaultOptions())
	paths := func(items []*Item) string {
		var p []string
		for _, it := range items {
			p = append(p, it.Path)
		}
		return strings.Join(p, ",")
	}
	cases := []struct {
		name string
		show Show
		want string
	}{
		{"all", ShowAll(), "d,d/x,e,eq,ln,lo,ro,ue"},
		{"default hides equal", DefaultShow(), "d,d/x,ln,lo,ro,ue"},
		{"only equal", Show{Equal: true, Duplicates: true, Singles: true}, "e,eq"},
		{"left to right", Show{LeftToRight: true, Duplicates: true, Singles: true}, "d,d/x,ln,lo"},
		{"right to left", Show{RightToLeft: true, Duplicates: true, Singles: true}, "ro"},
		{"singles only", Show{true, true, true, true, false, true}, "d,d/x,lo,ro"},
		{"duplicates only", Show{true, true, true, true, true, false}, "e,eq,ln,ue"},
		{"nothing", Show{}, ""},
	}
	for _, c := range cases {
		if got := paths(res.Visible(c.show)); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
	s := res.Summary()
	want := Summary{Equal: 1, LeftNewer: 1, LeftOnly: 2, RightOnly: 1, Unequal: 1, Dirs: 2}
	if s != want {
		t.Errorf("summary = %+v, want %+v", s, want)
	}
	if s.AllEqual() {
		t.Errorf("AllEqual true")
	}
}

func TestJSONShapes(t *testing.T) {
	l, r := trees(t, map[string]spec{"a": file("1", time.Hour)}, map[string]spec{"a": file("1", 0)})
	res := mustCompare(t, l, r, DefaultOptions())
	b, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"category":"leftNewer"`, `"action":"copyLeftToRight"`, `"kind":"file"`, `"case":"auto"`} {
		if !bytes.Contains(b, []byte(want)) {
			t.Errorf("JSON lacks %s: %s", want, b)
		}
	}
	var a Action
	if err := json.Unmarshal([]byte(`"deleteBoth"`), &a); err != nil || a != DeleteBoth {
		t.Errorf("unmarshal action: %v %v", a, err)
	}
	if err := json.Unmarshal([]byte(`"bogus"`), &a); err == nil {
		t.Errorf("bogus action accepted")
	}
	var c Category
	if err := json.Unmarshal([]byte(`"rightOnly"`), &c); err != nil || c != RightOnly {
		t.Errorf("unmarshal category: %v %v", c, err)
	}
	var o Options
	if err := json.Unmarshal([]byte(`{"case":"insensitive","subdirs":true}`), &o); err != nil || o.Case != CaseInsensitive || !o.Subdirs {
		t.Errorf("unmarshal options: %+v %v", o, err)
	}
	for c := Equal; c <= Unequal; c++ {
		if c.Symbol() == "?" {
			t.Errorf("no symbol for %s", c)
		}
	}
}
