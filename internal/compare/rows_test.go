// Tests for the side-by-side row layout, difference blocks and next/previous
// difference navigation.
package compare

import (
	"reflect"
	"testing"
)

func TestBuildRows(t *testing.T) {
	ops := []Op{
		{OpEqual, 0, 1, 0, 1},
		{OpChange, 1, 3, 1, 2},
		{OpEqual, 3, 4, 2, 3},
		{OpInsert, 4, 4, 3, 5},
		{OpDelete, 4, 5, 5, 5},
	}
	want := []Row{
		{RowEqual, 0, 0, 0},
		{RowChange, 1, 1, 1},
		{RowDelete, 2, -1, 1},
		{RowEqual, 3, 2, 2},
		{RowInsert, -1, 3, 3},
		{RowInsert, -1, 4, 3},
		{RowDelete, 4, -1, 4},
	}
	rows := BuildRows(ops)
	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("got %+v\nwant %+v", rows, want)
	}
	blocks := Blocks(rows)
	wantB := []Block{{1, 2, 1}, {4, 5, 3}, {6, 6, 4}}
	if !reflect.DeepEqual(blocks, wantB) {
		t.Fatalf("blocks %+v want %+v", blocks, wantB)
	}

	nav := []struct {
		row        int
		next, prev int
	}{
		{-1, 0, -1},
		{0, 0, -1},
		{1, 1, -1},
		{2, 1, -1},
		{3, 1, 0},
		{4, 2, 0},
		{5, 2, 0},
		{6, -1, 1},
		{7, -1, 2},
	}
	for _, n := range nav {
		if got := NextDiff(blocks, n.row); got != n.next {
			t.Errorf("NextDiff(%d) = %d, want %d", n.row, got, n.next)
		}
		if got := PrevDiff(blocks, n.row); got != n.prev {
			t.Errorf("PrevDiff(%d) = %d, want %d", n.row, got, n.prev)
		}
	}
	if BlockAt(blocks, 5) != 1 || BlockAt(blocks, 3) != -1 {
		t.Error("BlockAt")
	}
	if RowOfLine(rows, Right, 4) != 5 || RowOfLine(rows, Left, 4) != 6 || RowOfLine(rows, Left, 9) != -1 {
		t.Error("RowOfLine")
	}
}

func TestBuildRowsRepeatedFold(t *testing.T) {
	ops := DiffLines(lines("r\nr\nr\nx"), lines("r\nx"), Options{IgnoreRepeatedLines: true})
	rows := BuildRows(ops)
	want := []Row{{RowEqual, 0, 0, 0}, {RowEqual, 1, -1, 0}, {RowEqual, 2, -1, 0}, {RowEqual, 3, 1, 1}}
	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("got %+v want %+v", rows, want)
	}
	if len(Blocks(rows)) != 0 {
		t.Fatal("folded repeats must not produce blocks")
	}
}
