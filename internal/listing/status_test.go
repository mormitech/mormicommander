// Tests for status line totals.
package listing

import "testing"

func TestCountAndStatus(t *testing.T) {
	items := []Item{
		UpItem("/"),
		{Name: "a", Size: 1024},
		{Name: "b", Size: 2048},
		{Name: "c", Size: 4096},
		{Name: "d1", Attr: AttrDir, DirSize: -1},
		{Name: "d2", Attr: AttrDir, DirSize: 10240},
	}
	sel := map[int]bool{1: true, 3: true, 5: true, 0: true}
	tot := Count(items, func(i int) bool { return sel[i] })
	want := Totals{Files: 3, Dirs: 2, Bytes: 1024 + 2048 + 4096 + 10240, SelFiles: 2, SelDirs: 1,
		SelBytes: 1024 + 4096 + 10240, UnsizedDirs: 1}
	if tot != want {
		t.Fatalf("got %+v\nwant %+v", tot, want)
	}
	if got := StatusText(tot, SizeKB, NumberFormat{}); got != "15 k / 17 k in 2 / 3 file(s), 1 / 2 dir(s)" {
		t.Errorf("status: %q", got)
	}
	if got := StatusText(Count(items, nil), SizeBytes, NumberFormat{}); got != "0 / 17,408 in 0 / 3 file(s), 0 / 2 dir(s)" {
		t.Errorf("status bytes: %q", got)
	}
	if sum := tot.Add(tot); sum.Files != 6 || sum.SelBytes != 2*tot.SelBytes {
		t.Errorf("Add: %+v", sum)
	}
	f := &Format{AttrStyle: AttrStyleWindows}
	if got := ItemText(&items[1], f); got != "a  1,024    ----" {
		t.Errorf("item text %q", got)
	}
	if got := ItemText(&items[0], f); got != ".." {
		t.Errorf("up item text %q", got)
	}
}
