// Tests for directory compare: date rules, size and content differences, filters and the recursive tree walk on
// real temporary folders.

package masks

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

var t0 = time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)

func f(name string, size int64, d time.Duration) Entry {
	return Entry{Name: name, Size: size, ModTime: t0.Add(d)}
}

func TestCompareLists(t *testing.T) {
	left := []Entry{{Name: "..", IsDir: true}, f("same", 1, 0), f("newer", 1, time.Hour*2), f("older", 1, 0),
		f("onlyL", 1, 0), f("fat", 1, time.Second), f("size", 1, 0), f("dst", 1, time.Hour),
		{Name: "dirL", IsDir: true}, f("Case", 1, 0)}
	right := []Entry{f("same", 1, 0), f("newer", 1, 0), f("older", 1, time.Minute), f("onlyR", 1, 0),
		f("fat", 1, 0), f("size", 2, 0), f("dst", 1, 0), {Name: "dirR", IsDir: true}, f("case", 1, 0)}

	opt := DefaultCompareOptions()
	opt.Case = CaseSensitive
	res := CompareLists(left, right, opt)
	wantL := []CompareStatus{StatusSame, StatusSame, StatusNewer, StatusOlder, StatusOnlyHere, StatusSame,
		StatusSizeDiffers, StatusNewer, StatusSame, StatusOnlyHere}
	wantR := []CompareStatus{StatusSame, StatusOlder, StatusNewer, StatusOnlyHere, StatusSame, StatusSizeDiffers,
		StatusOlder, StatusSame, StatusOnlyHere}
	if !reflect.DeepEqual(res.Left, wantL) || !reflect.DeepEqual(res.Right, wantR) {
		t.Fatalf("\nleft  %v\nright %v", res.Left, res.Right)
	}
	if got := res.LeftMarks(); !reflect.DeepEqual(got, []bool{false, false, true, false, true, false, true, true, false, true}) {
		t.Errorf("left marks %v", got)
	}

	opt.Case = CaseInsensitive
	opt.IgnoreHourShift = true
	opt.Subdirs = true
	res = CompareLists(left, right, opt)
	if res.Left[7] != StatusSame || res.Left[9] != StatusSame || res.Left[8] != StatusOnlyHere || res.Right[7] != StatusOnlyHere {
		t.Errorf("hour shift / case / subdirs: %v | %v", res.Left, res.Right)
	}

	opt = CompareOptions{IgnoreDate: true, Case: CaseSensitive}
	res = CompareLists(left, right, opt)
	if res.Left[2] != StatusSame || res.Left[6] != StatusSizeDiffers {
		t.Errorf("ignore date: %v", res.Left)
	}

	opt = DefaultCompareOptions()
	opt.Case = CaseSensitive
	opt.Filter = MustParse("*|newer onlyL", Options{Case: CaseSensitive})
	res = CompareLists(left, right, opt)
	if res.Left[2] != StatusSame || res.Left[4] != StatusSame || res.Left[3] != StatusOlder {
		t.Errorf("filter: %v", res.Left)
	}
}

func TestCompareContentHook(t *testing.T) {
	left := []Entry{f("eqLater", 3, time.Hour), f("diffSameDate", 3, 0)}
	right := []Entry{f("eqLater", 3, 0), f("diffSameDate", 3, 0)}
	opt := DefaultCompareOptions()
	opt.Case = CaseSensitive
	opt.Content = func(l, r Entry) (bool, error) { return l.Name == "eqLater", nil }
	res := CompareLists(left, right, opt)
	if res.Left[0] != StatusSame || res.Left[1] != StatusContentDiffers || res.Right[1] != StatusContentDiffers {
		t.Errorf("content: %v %v", res.Left, res.Right)
	}
}

func write(t *testing.T, p, data string, mt time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, mt, mt); err != nil {
		t.Fatal(err)
	}
}

func TestCompareTrees(t *testing.T) {
	root := t.TempDir()
	L, R := filepath.Join(root, "L"), filepath.Join(root, "R")
	write(t, filepath.Join(L, "same.txt"), "abc", t0)
	write(t, filepath.Join(R, "same.txt"), "abc", t0)
	write(t, filepath.Join(L, "eq", "x"), "1", t0)
	write(t, filepath.Join(R, "eq", "x"), "1", t0)
	write(t, filepath.Join(L, "deep", "a", "b"), "old", t0)
	write(t, filepath.Join(R, "deep", "a", "b"), "new", t0.Add(time.Hour))
	write(t, filepath.Join(L, ".git", "HEAD"), "1", t0)
	write(t, filepath.Join(R, ".git", "HEAD"), "2", t0.Add(time.Hour))
	write(t, filepath.Join(L, "onlyL", "f"), "", t0)
	write(t, filepath.Join(L, "content.bin"), "aaaa", t0)
	write(t, filepath.Join(R, "content.bin"), "aaab", t0)

	lfs, rfs := os.DirFS(L), os.DirFS(R)
	opt := DefaultCompareOptions()
	opt.Case = CaseSensitive
	opt.Subdirs = true
	opt.Content = ContentEqualFS(lfs, rfs)
	opt.Filter = MustParse("*|.git\\", Options{Case: CaseSensitive})
	left, right, res, err := CompareTrees(lfs, ".", rfs, ".", opt)
	if err != nil || res.Err != nil {
		t.Fatal(err, res.Err)
	}
	got := map[string]CompareStatus{}
	for i, e := range left {
		got["L/"+e.Name] = res.Left[i]
	}
	for i, e := range right {
		got["R/"+e.Name] = res.Right[i]
	}
	want := map[string]CompareStatus{
		"L/same.txt": StatusSame, "R/same.txt": StatusSame,
		"L/eq": StatusSame, "R/eq": StatusSame,
		"L/deep": StatusDirDiffers, "R/deep": StatusDirDiffers,
		"L/.git": StatusSame, "R/.git": StatusSame,
		"L/onlyL":       StatusOnlyHere,
		"L/content.bin": StatusContentDiffers, "R/content.bin": StatusContentDiffers,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %v\nwant %v", got, want)
	}

	opt.Subdirs = false
	_, _, res, _ = CompareTrees(lfs, ".", rfs, ".", opt)
	for i, e := range left {
		if e.IsDir && res.Left[i] != StatusSame {
			t.Errorf("without subdirs %s = %v", e.Name, res.Left[i])
		}
	}
	if _, _, _, err := CompareTrees(lfs, "nope", rfs, ".", opt); err == nil {
		t.Error("missing folder must fail")
	}
}

func TestContentEqualFSErrors(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "big"), string(make([]byte, 200*1024)), t0)
	write(t, filepath.Join(dir, "big2"), string(make([]byte, 200*1024+1)), t0)
	fsys := os.DirFS(dir)
	eq := ContentEqualFS(fsys, fsys)
	if ok, err := eq(Entry{Path: "big"}, Entry{Path: "big"}); !ok || err != nil {
		t.Errorf("same file: %v %v", ok, err)
	}
	if ok, _ := eq(Entry{Path: "big"}, Entry{Path: "big2"}); ok {
		t.Error("different length must differ")
	}
	if _, err := eq(Entry{}, Entry{Path: "big"}); err == nil {
		t.Error("missing path must fail")
	}
	if _, err := eq(Entry{Path: "nope"}, Entry{Path: "big"}); err == nil {
		t.Error("missing file must fail")
	}
}
