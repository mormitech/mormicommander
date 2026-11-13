// Tests for the comparison rules: which side gets selected for missing, newer, same-time-different-size, type
// mismatch and equal pairs, under each tolerance and folder option. Most cases run against an in-memory Lister so the
// times are exact; the end-to-end cases build real folders in t.TempDir() and compare them through OS.
package dircompare

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// memFile is one entry of the in-memory file system.
type memFile struct {
	size    int64
	mtime   time.Time
	dir     bool
	data    string
	hidden  bool
	openErr bool
}

// memLister keeps a whole tree in a map from slash path to entry; "" is the root.
type memLister map[string]memFile

func (m memLister) ReadDir(_ context.Context, dir string) ([]Entry, error) {
	if dir != "" {
		if f, ok := m[dir]; !ok || !f.dir {
			return nil, errors.New("no such folder " + dir)
		}
	}
	var out []Entry
	for p, f := range m {
		parent := path.Dir(p)
		if parent == "." {
			parent = ""
		}
		if parent == dir {
			size := f.size
			if f.data != "" {
				size = int64(len(f.data))
			}
			out = append(out, Entry{Name: path.Base(p), Size: size, ModTime: f.mtime, IsDir: f.dir, Hidden: f.hidden})
		}
	}
	return out, nil
}

func (m memLister) Open(_ context.Context, p string) (io.ReadCloser, error) {
	f, ok := m[p]
	if !ok || f.openErr {
		return nil, errors.New("cannot open " + p)
	}
	return io.NopCloser(strings.NewReader(f.data)), nil
}

func (memLister) Join(dir, name string) string {
	if dir == "" {
		return name
	}
	return dir + "/" + name
}

var t0 = time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)

func file(size int64, off time.Duration) memFile { return memFile{size: size, mtime: t0.Add(off)} }
func data(s string, off time.Duration) memFile   { return memFile{data: s, mtime: t0.Add(off)} }
func dir() memFile                               { return memFile{dir: true, mtime: t0} }

func statuses(r *Result) map[string]Status {
	out := map[string]Status{}
	for _, it := range r.Flatten() {
		out[it.Rel] = it.Status
	}
	return out
}

func TestCompareRules(t *testing.T) {
	tests := []struct {
		name        string
		left, right memLister
		opt         Options
		want        map[string]Status
		lsel, rsel  []string
	}{
		{
			name:  "missing newer older equal and same-time-different-size",
			left:  memLister{"only-l": file(1, 0), "newer": file(5, time.Hour*2), "older": file(5, 0), "eq": file(3, 0), "sz": file(3, 0)},
			right: memLister{"only-r": file(1, 0), "newer": file(5, 0), "older": file(9, time.Minute), "eq": file(3, 0), "sz": file(4, 0)},
			want: map[string]Status{"only-l": LeftOnly, "only-r": RightOnly, "newer": LeftNewer, "older": RightNewer,
				"eq": Equal, "sz": Different},
			lsel: []string{"newer", "only-l", "sz"},
			rsel: []string{"older", "only-r", "sz"},
		},
		{
			name:  "folders ignored by default",
			left:  memLister{"d": dir(), "d/x": file(1, 0), "lonely": dir()},
			right: memLister{"d": dir()},
			want:  map[string]Status{},
		},
		{
			name:  "folders missing mode marks lone folders and type mismatches",
			left:  memLister{"d": dir(), "d/x": file(1, 0), "lonely": dir(), "mix": dir()},
			right: memLister{"d": dir(), "mix": file(1, 0)},
			opt:   Options{Subdirs: SubdirsMissing},
			want:  map[string]Status{"d": Equal, "lonely": LeftOnly, "mix": TypeMismatch},
			lsel:  []string{"lonely", "mix"},
			rsel:  []string{"mix"},
		},
		{
			name:  "recursive marks folder on the side with differences",
			left:  memLister{"a": dir(), "a/x": file(1, time.Hour*3), "b": dir(), "b/y": file(1, 0), "c": dir(), "c/z": file(1, 0), "c/w": file(1, 0)},
			right: memLister{"a": dir(), "a/x": file(1, 0), "b": dir(), "b/y": file(1, 0), "c": dir(), "c/z": file(2, 0), "c/v": file(1, 0)},
			opt:   Options{Subdirs: SubdirsRecursive},
			want: map[string]Status{"a": LeftNewer, "a/x": LeftNewer, "b": Equal, "b/y": Equal, "c": Different,
				"c/z": Different, "c/w": LeftOnly, "c/v": RightOnly},
			lsel: []string{"a", "c"},
			rsel: []string{"c"},
		},
		{
			name:  "recursive marks only right when only right has extras",
			left:  memLister{"a": dir()},
			right: memLister{"a": dir(), "a/new": file(1, 0)},
			opt:   Options{Subdirs: SubdirsRecursive},
			want:  map[string]Status{"a": RightNewer, "a/new": RightOnly},
			rsel:  []string{"a"},
		},
		{
			name:  "two second FAT tolerance",
			left:  memLister{"f": file(1, 2*time.Second), "g": file(1, 3*time.Second)},
			right: memLister{"f": file(1, 0), "g": file(1, 0)},
			opt:   Options{TimeTolerance: 2 * time.Second},
			want:  map[string]Status{"f": Equal, "g": LeftNewer},
			lsel:  []string{"g"},
		},
		{
			name:  "sub-millisecond differences are ignored even without tolerance",
			left:  memLister{"f": file(1, 300*time.Microsecond)},
			right: memLister{"f": file(1, 0)},
			want:  map[string]Status{"f": Equal},
		},
		{
			name:  "one hour offset ignored",
			left:  memLister{"f": file(1, time.Hour+time.Second), "g": file(1, -time.Hour), "h": file(1, 2*time.Hour)},
			right: memLister{"f": file(1, 0), "g": file(1, 0), "h": file(1, 0)},
			opt:   Options{IgnoreHourOffset: true, TimeTolerance: 2 * time.Second},
			want:  map[string]Status{"f": Equal, "g": Equal, "h": LeftNewer},
			lsel:  []string{"h"},
		},
		{
			name:  "ignore date compares size only",
			left:  memLister{"same": file(4, time.Hour*5), "diff": file(4, time.Hour*5)},
			right: memLister{"same": file(4, 0), "diff": file(5, 0)},
			opt:   Options{IgnoreDate: true},
			want:  map[string]Status{"same": Equal, "diff": Different},
			lsel:  []string{"diff"},
			rsel:  []string{"diff"},
		},
		{
			name:  "by content: identical bytes are equal despite dates",
			left:  memLister{"same": data("hello", time.Hour), "chg": data("hello", time.Hour), "tie": data("abc", 0), "len": data("ab", time.Hour)},
			right: memLister{"same": data("hello", 0), "chg": data("jello", 0), "tie": data("abd", 0), "len": data("abc", 0)},
			opt:   Options{ByContent: true},
			want:  map[string]Status{"same": Equal, "chg": LeftNewer, "tie": Different, "len": LeftNewer},
			lsel:  []string{"chg", "len", "tie"},
			rsel:  []string{"tie"},
		},
		{
			name:  "by content with ignore date",
			left:  memLister{"a": data("xx", time.Hour), "b": data("xy", time.Hour)},
			right: memLister{"a": data("xx", 0), "b": data("xz", 0)},
			opt:   Options{ByContent: true, IgnoreDate: true},
			want:  map[string]Status{"a": Equal, "b": Different},
			lsel:  []string{"b"},
			rsel:  []string{"b"},
		},
		{
			name:  "content read error is reported and marked",
			left:  memLister{"bad": {data: "q", mtime: t0, openErr: true}},
			right: memLister{"bad": data("q", 0)},
			opt:   Options{ByContent: true},
			want:  map[string]Status{"bad": Error},
			lsel:  []string{"bad"},
			rsel:  []string{"bad"},
		},
		{
			name:  "case insensitive matching keeps each side's spelling",
			left:  memLister{"Read.ME": file(1, time.Hour)},
			right: memLister{"read.me": file(1, 0)},
			opt:   Options{Case: CaseInsensitive},
			want:  map[string]Status{"Read.ME": LeftNewer},
			lsel:  []string{"Read.ME"},
		},
		{
			name:  "case sensitive matching",
			left:  memLister{"Read.ME": file(1, 0)},
			right: memLister{"read.me": file(1, 0)},
			opt:   Options{Case: CaseSensitive},
			want:  map[string]Status{"Read.ME": LeftOnly, "read.me": RightOnly},
			lsel:  []string{"Read.ME"},
			rsel:  []string{"read.me"},
		},
		{
			name:  "case folding collision on one side is not dropped",
			left:  memLister{"A": file(1, 0), "a": file(1, 0)},
			right: memLister{"a": file(1, 0)},
			opt:   Options{Case: CaseInsensitive},
			want:  map[string]Status{"A": Equal, "a": LeftOnly},
			lsel:  []string{"a"},
		},
		{
			name:  "hidden files skipped unless included",
			left:  memLister{".h": {size: 1, mtime: t0, hidden: true}},
			right: memLister{},
			want:  map[string]Status{},
		},
		{
			name:  "hidden files included",
			left:  memLister{".h": {size: 1, mtime: t0, hidden: true}},
			right: memLister{},
			opt:   Options{IncludeHidden: true},
			want:  map[string]Status{".h": LeftOnly},
			lsel:  []string{".h"},
		},
		{
			name:  "filter limits what takes part",
			left:  memLister{"a.txt": file(1, 0), "b.log": file(1, 0), "d": dir(), "d/c.txt": file(1, time.Hour)},
			right: memLister{"d": dir(), "d/c.txt": file(1, 0)},
			opt: Options{Subdirs: SubdirsRecursive, Filter: func(rel string, e Entry) bool {
				return e.IsDir || strings.HasSuffix(rel, ".txt")
			}},
			want: map[string]Status{"a.txt": LeftOnly, "d": LeftNewer, "d/c.txt": LeftNewer},
			lsel: []string{"a.txt", "d"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, err := Compare(context.Background(), tc.left, "", tc.right, "", tc.opt)
			if err != nil {
				t.Fatal(err)
			}
			if got := statuses(r); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("statuses = %v, want %v", got, tc.want)
			}
			if !reflect.DeepEqual(r.LeftSelect, tc.lsel) {
				t.Errorf("LeftSelect = %v, want %v", r.LeftSelect, tc.lsel)
			}
			if !reflect.DeepEqual(r.RightSelect, tc.rsel) {
				t.Errorf("RightSelect = %v, want %v", r.RightSelect, tc.rsel)
			}
			if r.Differ() != (len(tc.lsel)+len(tc.rsel) > 0) {
				t.Errorf("Differ() = %v", r.Differ())
			}
			n := 0
			for _, c := range r.Counts {
				n += c
			}
			if n != len(tc.want) {
				t.Errorf("Counts total %d, want %d", n, len(tc.want))
			}
		})
	}
}

func TestEqualListsAndErrors(t *testing.T) {
	left := memLister{"e1": file(1, 0), "e2": file(2, 0), "x": file(1, time.Hour), "sub": dir()}
	right := memLister{"E1": file(1, 0), "e2": file(2, 0), "x": file(1, 0), "sub": dir()}
	r, err := Compare(context.Background(), left, "", right, "", Options{Case: CaseInsensitive, Subdirs: SubdirsMissing})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"e1", "e2", "sub"}; !reflect.DeepEqual(r.LeftEqual, want) {
		t.Errorf("LeftEqual = %v, want %v", r.LeftEqual, want)
	}
	if want := []string{"E1", "e2", "sub"}; !reflect.DeepEqual(r.RightEqual, want) {
		t.Errorf("RightEqual = %v, want %v", r.RightEqual, want)
	}
	// Folders sort first.
	if r.Items[0].Name != "sub" {
		t.Errorf("first item %q, want folder first", r.Items[0].Name)
	}

	// A subfolder that cannot be listed on one side becomes an Error item, not a failed run.
	broken := memLister{"s": dir()}
	okSide := memLister{"s": dir()}
	r, err = Compare(context.Background(), brokenDir{broken}, "", okSide, "", Options{Subdirs: SubdirsRecursive})
	if err != nil {
		t.Fatal(err)
	}
	if r.Items[0].Status != Error || len(r.Errors) != 1 {
		t.Fatalf("items %+v errors %v", r.Items, r.Errors)
	}
	var ie *ItemError
	if !errors.As(r.Errors[0], &ie) || ie.Rel != "s" {
		t.Errorf("error %v is not an ItemError for s", r.Errors[0])
	}

	if _, err := Compare(context.Background(), left, "nope", right, "", Options{}); err == nil {
		t.Error("missing top folder did not fail")
	}
	if _, err := Compare(context.Background(), nil, "", right, "", Options{}); !errors.Is(err, ErrNilLister) {
		t.Errorf("nil lister: %v", err)
	}
}

// brokenDir fails to list anything below the root.
type brokenDir struct{ memLister }

func (b brokenDir) ReadDir(ctx context.Context, d string) ([]Entry, error) {
	if d != "" {
		return nil, errors.New("permission denied")
	}
	return b.memLister.ReadDir(ctx, d)
}

func TestCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	left := memLister{"a": file(1, 0), "b": file(1, 0), "c": file(1, 0)}
	_, err := Compare(ctx, left, "", memLister{}, "", Options{Progress: func(Progress) {
		calls++
		cancel()
	}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if calls != 1 {
		t.Errorf("progress called %d times after cancel", calls)
	}
}

func TestCompareEntries(t *testing.T) {
	l := []Entry{{Name: "a", Size: 1, ModTime: t0.Add(time.Hour)}, {Name: "d", IsDir: true}}
	r := []Entry{{Name: "a", Size: 1, ModTime: t0}}
	items := CompareEntries(l, r, Options{Subdirs: SubdirsRecursive, ByContent: true})
	got := map[string]Status{}
	for _, it := range items {
		got[it.Name] = it.Status
	}
	want := map[string]Status{"a": LeftNewer, "d": LeftOnly}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestCompareTimes(t *testing.T) {
	tests := []struct {
		d    time.Duration
		tol  time.Duration
		hour bool
		want int
	}{
		{0, 0, false, 0},
		{time.Millisecond, 0, false, 1},
		{-time.Millisecond, 0, false, -1},
		{2 * time.Second, 2 * time.Second, false, 0},
		{-2 * time.Second, -2 * time.Second, false, 0},
		{time.Hour, 0, false, 1},
		{time.Hour, 0, true, 0},
		{-time.Hour - time.Second, 2 * time.Second, true, 0},
		{time.Hour + 3*time.Second, 2 * time.Second, true, 1},
	}
	for _, tc := range tests {
		if got := CompareTimes(t0.Add(tc.d), t0, tc.tol, tc.hour); got != tc.want {
			t.Errorf("CompareTimes(+%v, tol %v, hour %v) = %d, want %d", tc.d, tc.tol, tc.hour, got, tc.want)
		}
	}
}

func TestStatusHelpers(t *testing.T) {
	tests := []struct {
		s      Status
		l, r   bool
		symbol string
	}{
		{Equal, false, false, "="},
		{LeftOnly, true, false, "->"},
		{RightOnly, false, true, "<-"},
		{LeftNewer, true, false, "->"},
		{RightNewer, false, true, "<-"},
		{Different, true, true, "!="},
		{TypeMismatch, true, true, "x"},
		{Error, true, true, "x"},
	}
	for _, tc := range tests {
		if tc.s.MarksLeft() != tc.l || tc.s.MarksRight() != tc.r || tc.s.Symbol() != tc.symbol || tc.s.String() == "unknown" {
			t.Errorf("%v: left %v right %v symbol %q", tc.s, tc.s.MarksLeft(), tc.s.MarksRight(), tc.s.Symbol())
		}
	}
}

func TestSameContent(t *testing.T) {
	big := bytes.Repeat([]byte("0123456789"), blockSize/5) // two blocks
	alt := append([]byte(nil), big...)
	alt[len(alt)-1] = 'x'
	tests := []struct {
		name string
		a, b []byte
		want bool
	}{
		{"empty", nil, nil, true},
		{"same small", []byte("abc"), []byte("abc"), true},
		{"prefix", []byte("abc"), []byte("abcd"), false},
		{"same big", big, append([]byte(nil), big...), true},
		{"differ at end of big", big, alt, false},
		{"exactly one block vs longer", big[:blockSize], big[:blockSize+1], false},
	}
	for _, tc := range tests {
		got, err := SameContent(context.Background(), bytes.NewReader(tc.a), bytes.NewReader(tc.b))
		if err != nil || got != tc.want {
			t.Errorf("%s: got %v, %v; want %v", tc.name, got, err, tc.want)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := SameContent(ctx, bytes.NewReader(big), bytes.NewReader(big)); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled SameContent: %v", err)
	}
}

// TestOS compares real folders through the OS Lister, the path the app takes for local panels.
func TestOS(t *testing.T) {
	root := t.TempDir()
	l, r := filepath.Join(root, "L"), filepath.Join(root, "R")
	old := time.Now().Add(-48 * time.Hour).Truncate(time.Second)
	mk := func(p, content string, mt time.Time) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, mt, mt); err != nil {
			t.Fatal(err)
		}
	}
	mk(filepath.Join(l, "same.txt"), "same", old)
	mk(filepath.Join(r, "same.txt"), "same", old)
	mk(filepath.Join(l, "touched.txt"), "same", old.Add(time.Hour*3))
	mk(filepath.Join(r, "touched.txt"), "same", old)
	mk(filepath.Join(l, "edited.txt"), "v2!", old.Add(time.Hour*3))
	mk(filepath.Join(r, "edited.txt"), "v1!", old)
	mk(filepath.Join(l, "only-left.txt"), "x", old)
	mk(filepath.Join(r, "sub", "deep.txt"), "x", old)
	mk(filepath.Join(l, "sub", "deep.txt"), "y", old.Add(time.Hour*3))
	mk(filepath.Join(r, "rsub", "a"), "x", old)
	if err := os.Symlink(filepath.Join(l, "same.txt"), filepath.Join(l, "link.txt")); err == nil {
		mk(filepath.Join(r, "link.txt"), "same", old)
	}

	res, err := Compare(context.Background(), OS{}, l, OS{}, r, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"edited.txt", "only-left.txt", "touched.txt"}; !reflect.DeepEqual(res.LeftSelect, want) {
		t.Errorf("plain LeftSelect = %v, want %v", res.LeftSelect, want)
	}
	if len(res.RightSelect) != 0 {
		t.Errorf("plain RightSelect = %v", res.RightSelect)
	}

	res, err = Compare(context.Background(), OS{}, l, OS{}, r, Options{ByContent: true, Subdirs: SubdirsRecursive})
	if err != nil {
		t.Fatal(err)
	}
	st := statuses(res)
	want := map[string]Status{"same.txt": Equal, "touched.txt": Equal, "edited.txt": LeftNewer, "only-left.txt": LeftOnly,
		"sub": LeftNewer, "sub/deep.txt": LeftNewer, "rsub": RightOnly}
	if _, ok := st["link.txt"]; ok {
		want["link.txt"] = Equal // the link is followed and compared as the file it points to
	}
	if !reflect.DeepEqual(st, want) {
		t.Errorf("content+recursive statuses = %v, want %v", st, want)
	}
	if want := []string{"rsub"}; !reflect.DeepEqual(res.RightSelect, want) {
		t.Errorf("RightSelect = %v, want %v", res.RightSelect, want)
	}
}
