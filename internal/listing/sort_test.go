// Tests for TC's sort rules: dirs first, ".." on top, default directions, toggling and multi-key sorts.
package listing

import (
	"reflect"
	"testing"
	"time"
)

func names(items []Item) []string {
	out := make([]string, len(items))
	for i := range items {
		out[i] = items[i].Name
	}
	return out
}

func day(d int) time.Time { return time.Date(2026, 1, d, 12, 0, 0, 0, time.UTC) }

func sample() []Item {
	items := []Item{
		{Name: "b.txt", Size: 300, ModTime: day(3), Attr: AttrArchive},
		{Name: "file10.txt", Size: 10, ModTime: day(1)},
		{Name: "Docs", Attr: AttrDir, DirSize: -1, ModTime: day(5)},
		{Name: "a.zip", Size: 50, ModTime: day(4), Attr: AttrReadOnly},
		{Name: "file2.txt", Size: 20, ModTime: day(2)},
		{Name: "..", Attr: AttrDir, DirSize: -1},
		{Name: "apps", Attr: AttrDir, DirSize: -1, ModTime: day(1)},
		{Name: "noext", Size: 5, ModTime: day(6)},
	}
	for i := range items {
		items[i].Order = i
	}
	return items
}

func TestSort(t *testing.T) {
	tests := []struct {
		name string
		spec SortSpec
		col  Collation
		want []string
	}{
		{"name", DefaultSort, Collation{}, []string{"..", "apps", "Docs", "a.zip", "b.txt", "file10.txt", "file2.txt", "noext"}},
		{"name natural", DefaultSort, Collation{Natural: true}, []string{"..", "apps", "Docs", "a.zip", "b.txt", "file2.txt", "file10.txt", "noext"}},
		{"name desc", SortSpec{Keys: []SortKey{{By: ByName, Desc: true}}}, Collation{}, []string{"..", "Docs", "apps", "noext", "file2.txt", "file10.txt", "b.txt", "a.zip"}},
		{"ext", SortSpec{Keys: []SortKey{{By: ByExt}}}, Collation{}, []string{"..", "apps", "Docs", "noext", "b.txt", "file10.txt", "file2.txt", "a.zip"}},
		{"size desc", SortSpec{Keys: []SortKey{{By: BySize, Desc: true}}}, Collation{}, []string{"..", "apps", "Docs", "b.txt", "a.zip", "file2.txt", "file10.txt", "noext"}},
		{"size asc", SortSpec{Keys: []SortKey{{By: BySize}}}, Collation{}, []string{"..", "apps", "Docs", "noext", "file10.txt", "file2.txt", "a.zip", "b.txt"}},
		{"date desc", SortSpec{Keys: []SortKey{{By: ByDate, Desc: true}}}, Collation{}, []string{"..", "Docs", "apps", "noext", "a.zip", "b.txt", "file2.txt", "file10.txt"}},
		{"date desc dirs by name", SortSpec{Keys: []SortKey{{By: ByDate, Desc: true}}, DirsByName: true}, Collation{}, []string{"..", "apps", "Docs", "noext", "a.zip", "b.txt", "file2.txt", "file10.txt"}},
		{"unsorted", SortSpec{Keys: []SortKey{{By: ByUnsorted}}}, Collation{}, []string{"..", "Docs", "apps", "b.txt", "file10.txt", "a.zip", "file2.txt", "noext"}},
		{"mixed dirs", SortSpec{Keys: []SortKey{{By: ByName}}, MixDirs: true}, Collation{}, []string{"..", "a.zip", "apps", "b.txt", "Docs", "file10.txt", "file2.txt", "noext"}},
		{"attr", SortSpec{Keys: []SortKey{{By: ByAttr}}}, Collation{}, []string{"..", "apps", "Docs", "file10.txt", "file2.txt", "noext", "b.txt", "a.zip"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := sample()
			Sort(items, tt.spec, SortOptions{Collator: NewCollator(tt.col), Format: &Format{AttrStyle: AttrStyleWindows}})
			if got := names(items); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got  %v\nwant %v", got, tt.want)
			}
		})
	}
}

func TestSortSizedDirs(t *testing.T) {
	items := []Item{
		{Name: "small", Attr: AttrDir, DirSize: 10},
		{Name: "big", Attr: AttrDir, DirSize: 1000},
		{Name: "unknown", Attr: AttrDir, DirSize: -1},
	}
	Sort(items, SortSpec{Keys: []SortKey{{By: BySize, Desc: true}}}, SortOptions{})
	if got := names(items); !reflect.DeepEqual(got, []string{"big", "small", "unknown"}) {
		t.Fatalf("got %v", got)
	}
}

func TestSortMultiKey(t *testing.T) {
	items := []Item{
		{Name: "c.txt", Size: 1, ModTime: day(1)},
		{Name: "a.txt", Size: 2, ModTime: day(1)},
		{Name: "b.txt", Size: 3, ModTime: day(2)},
	}
	spec := SortSpec{Keys: []SortKey{{By: ByDate, Desc: true}, {By: BySize, Desc: true}}}
	Sort(items, spec, SortOptions{})
	if got := names(items); !reflect.DeepEqual(got, []string{"b.txt", "a.txt", "c.txt"}) {
		t.Fatalf("got %v", got)
	}
}

func TestSortByColumn(t *testing.T) {
	set := ColumnSet{Columns: []Column{
		{Title: "Name", Template: "[=tc.fullname]"},
		{Title: "Size", Template: "[=tc.size] bytes"},
		{Title: "Comment", Template: "[=tc.comment]"},
	}}.MustCompile()
	items := []Item{
		{Name: "a", Size: 900, Comment: "zeta"},
		{Name: "b", Size: 80, Comment: ""},
		{Name: "c", Size: 1000, Comment: "alpha"},
	}
	opt := SortOptions{Set: set}
	Sort(items, SortSpec{Keys: []SortKey{{By: ByColumn, Column: 1}}}, opt)
	if got := names(items); !reflect.DeepEqual(got, []string{"b", "a", "c"}) {
		t.Fatalf("numeric column sort: %v", got)
	}
	Sort(items, SortSpec{Keys: []SortKey{{By: ByColumn, Column: 2}}}, opt)
	if got := names(items); !reflect.DeepEqual(got, []string{"c", "a", "b"}) {
		t.Fatalf("text column asc (empty last): %v", got)
	}
	Sort(items, SortSpec{Keys: []SortKey{{By: ByColumn, Column: 2, Desc: true}}}, opt)
	if got := names(items); !reflect.DeepEqual(got, []string{"a", "c", "b"}) {
		t.Fatalf("text column desc (empty still last): %v", got)
	}
}

func TestToggle(t *testing.T) {
	s := DefaultSort
	s = s.Toggle(ByName, 0, false)
	if p := s.Primary(); p.By != ByName || !p.Desc {
		t.Fatalf("same key should reverse: %+v", p)
	}
	s = s.Toggle(BySize, 0, false)
	if p := s.Primary(); p.By != BySize || !p.Desc || len(s.Keys) != 1 {
		t.Fatalf("size starts descending: %+v", s.Keys)
	}
	s = s.Toggle(ByName, 0, true)
	if len(s.Keys) != 2 || s.Keys[1].By != ByName || s.Keys[1].Desc {
		t.Fatalf("ctrl+click adds key: %+v", s.Keys)
	}
	s = s.Toggle(ByName, 0, true)
	if !s.Keys[1].Desc {
		t.Fatalf("ctrl+click again reverses secondary: %+v", s.Keys)
	}
	s = s.Toggle(BySize, 0, false)
	if len(s.Keys) != 1 || s.Keys[0].Desc {
		t.Fatalf("plain click on primary reverses and drops secondaries: %+v", s.Keys)
	}
	c := DefaultSort.Toggle(ByColumn, 3, false)
	c2 := c.Toggle(ByColumn, 4, false)
	if c2.Keys[0].Column != 4 || c2.Keys[0].Desc {
		t.Fatalf("other column is a new key: %+v", c2.Keys)
	}
	r := DefaultSort.Reverse()
	if !r.Keys[0].Desc || DefaultSort.Keys[0].Desc {
		t.Fatal("Reverse must not alias DefaultSort")
	}
}

func TestOrderLeavesInput(t *testing.T) {
	items := sample()
	before := names(items)
	idx := Order(items, DefaultSort, SortOptions{})
	if !reflect.DeepEqual(names(items), before) || len(idx) != len(items) {
		t.Fatal("Order modified input")
	}
	if items[idx[0]].Name != ".." {
		t.Fatalf("first is %q", items[idx[0]].Name)
	}
}
