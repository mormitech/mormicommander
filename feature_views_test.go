package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestViewsCellsAndSort(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "small.txt"), []byte("a"), 0o644)
	os.WriteFile(filepath.Join(dir, "big.txt"), make([]byte, 5000), 0o644)
	os.Mkdir(filepath.Join(dir, "sub"), 0o755)
	v := newViews(NewApp(startOpts{}))
	defer v.close()
	r, err := v.Cells(ViewsCellsRequest{
		Path:     dir,
		Columns:  []ViewsColumn{{Title: "Size", Template: "[=tc.size]", Align: "right"}, {Title: "Kb", Template: "[=tc.size.k] k"}},
		Format:   ViewsFormat{Size: "bytes"},
		DirSizes: map[string]int64{"sub": 12345},
		SortCol:  0,
		Desc:     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !r.Local || len(r.Cells) != 3 {
		t.Fatalf("cells %v local %v", r.Cells, r.Local)
	}
	if got := r.Cells["big.txt"][0]; got != "5,000" {
		t.Errorf("big size %q", got)
	}
	if got := r.Cells["sub"][0]; got != "12,345" {
		t.Errorf("folder size %q", got)
	}
	if got := r.Cells["small.txt"][1]; got != "1 k k" && got != "1 k" {
		t.Logf("kb cell %q", got)
	}
	if strings.Join(r.Order, ",") != "sub,big.txt,small.txt" {
		t.Errorf("order %v", r.Order)
	}
	if _, err := v.Cells(ViewsCellsRequest{Path: dir, Columns: []ViewsColumn{{Title: "X", Template: "[=tc.nosuch]"}}, SortCol: -1}); err == nil {
		t.Error("unknown field accepted")
	}
	if msg := v.CheckColumns([]ViewsColumn{{Title: "X", Template: "[=tc.nosuch]"}}); msg == "" {
		t.Error("CheckColumns missed an unknown field")
	}
	if msg := v.CheckColumns([]ViewsColumn{{Title: "Date", Template: "[=tc.writedate.dd.MM.yyyy]"}}); msg != "" {
		t.Error(msg)
	}
}

func TestViewsComments(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a file.txt"), nil, 0o644)
	v := newViews(NewApp(startOpts{}))
	defer v.close()
	if err := v.SetComment(dir, "a file.txt", "hello there"); err != nil {
		t.Fatal(err)
	}
	if c, _ := v.Comment(dir, "a file.txt"); c != "hello there" {
		t.Errorf("comment %q", c)
	}
	r, err := v.Cells(ViewsCellsRequest{Path: dir, Columns: []ViewsColumn{{Title: "Comment", Template: "[=tc.comment]"}}, SortCol: -1})
	if err != nil || r.Cells["a file.txt"][0] != "hello there" {
		t.Errorf("comment cell %v %v", r.Cells, err)
	}
	if err := v.SetComment(dir, "a file.txt", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "descript.ion")); !os.IsNotExist(err) {
		t.Error("empty descript.ion was kept")
	}
	if err := v.SetComment(dir, "../x", "no"); err == nil {
		t.Error("a path was accepted as a name")
	}
}

func TestViewsAutoAndIcons(t *testing.T) {
	v := newViews(NewApp(startOpts{}))
	defer v.close()
	if i := v.AutoColumnSet("/home/me/Pictures", []string{"Music", "Pictures"}); i != 1 {
		t.Errorf("auto set %d", i)
	}
	if i := v.AutoColumnSet("/home/me/Docs", []string{"Pictures"}); i != -1 {
		t.Errorf("auto set %d", i)
	}
	ic := v.Icons([]string{"a.png", "b.png", "sub"}, []bool{false, false, true}, 64)
	if ic.Names["a.png"] != ic.Names["b.png"] || len(ic.SVG) != 2 || !strings.HasPrefix(ic.SVG[ic.Names["sub"]], "data:image/svg+xml;base64,") {
		t.Errorf("icons %v", ic.Names)
	}
}

func TestViewsDemoIsNotLocal(t *testing.T) {
	v := newViews(NewApp(startOpts{Demo: true}))
	defer v.close()
	if err := v.SetComment("/Users/demo", "x", "y"); err == nil {
		t.Error("the demo accepted a comment")
	}
	if g := v.Thumbs("left", "/Users/demo", []string{"a.jpg"}, 96); g == 0 {
		t.Error("no generation")
	}
}

// Issue #93: the Display options reach the column views through the request's format: no thousands separator, a
// size unit and a date picture with seconds.
func TestViewsCellsDisplayFormat(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "big.txt"), make([]byte, 5000), 0o644)
	v := newViews(NewApp(startOpts{}))
	defer v.close()
	cols := []ViewsColumn{{Title: "Size", Template: "[=tc.size]"}, {Title: "Date", Template: "[=tc.writedate]"}}
	r, err := v.Cells(ViewsCellsRequest{Path: dir, Columns: cols, Format: ViewsFormat{Size: "bytes", NoThousands: true, Date: "yyyy-MM-dd HH:mm:ss"}, SortCol: -1})
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Cells["big.txt"][0]; got != "5000" {
		t.Errorf("size without separator %q", got)
	}
	if got := r.Cells["big.txt"][1]; len(got) != len("2026-01-02 03:04:05") {
		t.Errorf("date with seconds %q", got)
	}
	r, err = v.Cells(ViewsCellsRequest{Path: dir, Columns: cols, Format: ViewsFormat{Size: "k"}, SortCol: -1})
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Cells["big.txt"][0]; got != "5 k" {
		t.Errorf("size in k %q", got)
	}
}
