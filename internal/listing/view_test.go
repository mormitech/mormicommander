// Tests for the Build pipeline and Brief view layout arithmetic.
package listing

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestBuild(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("12345"), 0o644)
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("1"), 0o644)
	os.WriteFile(filepath.Join(dir, ".dot"), nil, 0o644)
	os.Mkdir(filepath.Join(dir, "zdir"), 0o755)
	items, err := ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	c := NewComments()
	c.Set("a.go", "source")
	opt := Options{
		Mode:     ViewFull,
		Format:   Format{AttrStyle: AttrStyleWindows, Location: time.UTC},
		Sort:     DefaultSort,
		Filter:   FilterOptions{ShowUp: true, Dir: dir},
		Comments: c,
	}
	v := Build(items, opt)
	var got []string
	for _, r := range v.Rows {
		got = append(got, r.Item.Name)
	}
	if !reflect.DeepEqual(got, []string{"..", "zdir", "a.go", "b.txt"}) {
		t.Fatalf("rows %v", got)
	}
	if !reflect.DeepEqual(v.Titles, []string{"Name", "Ext", "Size", "Date", "Attr"}) {
		t.Errorf("titles %v", v.Titles)
	}
	if r := v.Rows[0].Cells; r[0] != ".." || r[2] != "<UP--DIR>" || r[4] != "" {
		t.Errorf("up row %q", r)
	}
	if r := v.Rows[1].Cells; r[0] != "zdir" || r[2] != "<DIR>" {
		t.Errorf("dir row %q", r)
	}
	if r := v.Rows[3].Cells; r[0] != "b" || r[1] != "txt" || r[2] != "5" {
		t.Errorf("file row %q", r)
	}
	if v.Totals.Files != 2 || v.Totals.Dirs != 1 || v.Totals.Bytes != 6 {
		t.Errorf("totals %+v", v.Totals)
	}
	if v.Find("b.txt") != 3 || v.Find("nope") != -1 {
		t.Error("Find")
	}
	sel := v.Count(func(i int) bool { return i == 3 })
	if sel.SelFiles != 1 || sel.SelBytes != 5 {
		t.Errorf("sel %+v", sel)
	}

	opt.Mode = ViewComments
	v = Build(items, opt)
	if v.Rows[2].Cells[1] != "source" {
		t.Errorf("comments view %q", v.Rows[2].Cells)
	}
	opt.Mode = ViewBrief
	v = Build(items, opt)
	if len(v.Titles) != 1 || v.Rows[3].Cells[0] != "b.txt" {
		t.Errorf("brief %q", v.Rows[3].Cells)
	}
	opt.Mode = ViewFull
	opt.NamesWhole = true
	v = Build(items, opt)
	if v.Rows[3].Cells[0] != "b.txt" || len(v.Titles) != 4 {
		t.Errorf("names whole %q", v.Rows[3].Cells)
	}
	opt.Mode = ViewCustom
	opt.Custom = ColumnSet{Name: "Mini", Columns: []Column{{Title: "N", Template: "[=tc.fullname] ([=tc.size.raw])"}}}.MustCompile()
	opt.Sort = SortSpec{Keys: []SortKey{{By: BySize, Desc: true}}}
	v = Build(items, opt)
	if v.Rows[2].Cells[0] != "b.txt (5)" {
		t.Errorf("custom %q", v.Rows[2].Cells)
	}
}

func TestBriefLayout(t *testing.T) {
	b := BriefLayout{N: 10, Rows: 4}
	if b.Columns() != 3 {
		t.Fatalf("columns %d", b.Columns())
	}
	if c, r := b.Pos(5); c != 1 || r != 1 {
		t.Errorf("Pos(5) = %d,%d", c, r)
	}
	if b.Index(2, 1) != 9 || b.Index(2, 2) != -1 || b.Index(0, 4) != -1 {
		t.Error("Index")
	}
	tests := []struct{ i, dc, dr, want int }{{0, 1, 0, 4}, {5, 1, 0, 9}, {9, 1, 0, 9}, {6, -1, 0, 2}, {1, -1, 0, 0}, {3, 0, 1, 4}}
	for _, tt := range tests {
		if got := b.Move(tt.i, tt.dc, tt.dr); got != tt.want {
			t.Errorf("Move(%d,%d,%d) = %d want %d", tt.i, tt.dc, tt.dr, got, tt.want)
		}
	}
	if (BriefLayout{N: 3}).Columns() != 3 {
		t.Error("zero rows treated as one")
	}
}
