package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"mormicommander/internal/vfs"
)

func selTestRows() []SelectEntry {
	return []SelectEntry{
		{Name: "..", Dir: true, Size: -1},
		{Name: "docs", Dir: true, Size: -1},
		{Name: "a.txt", Size: 1},
		{Name: "B.TXT", Size: 2},
		{Name: "a.go", Size: 3},
		{Name: "notes", Size: 4},
		{Name: "a.txt.bak", Size: 5},
	}
}

func TestSelectGroup(t *testing.T) {
	s := &Select{app: &App{fs: vfs.Local{}}}
	cases := []struct {
		expr, kind string
		want       []int
	}{
		{"*.txt", "files", []int{2, 3}},
		{"*.*", "files", []int{2, 3, 4, 5, 6}},
		{"*.*", "both", []int{1, 2, 3, 4, 5, 6}},
		{"*.*", "folders", []int{1}},
		{"*.", "files", []int{5}},
		{"*|*.bak *.go", "files", []int{2, 3, 5}},
		{">text", "files", []int{2, 3}},
		{"<^a\\.", "files", []int{2, 4, 6}},
	}
	for _, c := range cases {
		got, err := s.Group(selTestRows(), c.expr, c.kind, map[string]string{"Text": "*.txt"})
		if err != nil {
			t.Fatalf("%s: %v", c.expr, err)
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s %s: got %v, want %v", c.expr, c.kind, got, c.want)
		}
	}
	if _, err := s.Group(selTestRows(), "<(", "files", nil); err == nil || !strings.HasPrefix(err.Error(), "Invalid regular expression") {
		t.Errorf("bad regex: %v", err)
	}
	if msg := s.Check(">nope", nil); msg == "" || strings.HasPrefix(msg, "masks:") {
		t.Errorf("unknown filter message: %q", msg)
	}
}

func TestSelectSameAndNames(t *testing.T) {
	s := &Select{}
	if got := s.Same(selTestRows(), 2, "ext", "files"); !reflect.DeepEqual(got, []int{2, 3}) {
		t.Errorf("same ext: %v", got)
	}
	if got := s.Same(selTestRows(), 2, "name", "files"); !reflect.DeepEqual(got, []int{2, 4}) {
		t.Errorf("same name: %v", got)
	}
	if got := s.Same(selTestRows(), 0, "ext", "files"); len(got) != 0 {
		t.Errorf("on ..: %v", got)
	}
	if got := s.Names(selTestRows(), []string{"notes", "docs", ".."}); !reflect.DeepEqual(got, []int{1, 5}) {
		t.Errorf("names: %v", got)
	}
}

func TestSelectFileAndClipboard(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MORMICOMMANDER_CLIPBOARD_FILE", filepath.Join(dir, "clip.txt"))
	s := &Select{app: &App{fs: vfs.Local{}}}
	file := filepath.Join(dir, "sel.txt")
	if err := s.SaveFile(file, dir, []string{"a.txt", "notes"}, false); err != nil {
		t.Fatal(err)
	}
	if !s.Exists(file) {
		t.Fatal("saved file missing")
	}
	got, err := s.LoadFile(file, dir, selTestRows())
	if err != nil || !reflect.DeepEqual(got, []int{2, 5}) {
		t.Errorf("load file: %v %v", got, err)
	}
	if _, err := s.LoadFile(filepath.Join(dir, "none.txt"), dir, selTestRows()); err == nil {
		t.Error("missing file gave no error")
	}
	// empty clipboard selects nothing
	if got, err := s.LoadClip(dir, selTestRows()); err != nil || len(got) != 0 {
		t.Errorf("empty clip: %v %v", got, err)
	}
	rows := []SelectEntry{{Name: "a.go", Size: 3, MTime: time.Date(2024, 5, 1, 12, 0, 0, 0, time.Local).UnixMilli(), Attr: "rw-r--r--"},
		{Name: "docs", Dir: true, Size: -1}}
	text, err := s.CopyNames("cm_CopyFullNamesToClip", dir, rows)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "a.go") + "\n" + filepath.Join(dir, "docs")
	if os.PathSeparator == '\\' {
		want = strings.ReplaceAll(want, "\n", "\r\n")
	}
	if text != want {
		t.Errorf("full names: %q, want %q", text, want)
	}
	text, _ = s.CopyNames("cm_CopyDetailsToClip", dir, rows)
	if !strings.HasPrefix(text, "a.go\t3\t2024-05-01 12:00\trw-r--r--") || !strings.Contains(text, "docs\t<DIR>") {
		t.Errorf("details: %q", text)
	}
	if s.ReadClipText() != text {
		t.Error("clipboard file not written")
	}
	got, err = s.LoadClip(dir, []SelectEntry{{Name: "x"}, {Name: "a.go"}, {Name: "docs", Dir: true}})
	if err != nil || !reflect.DeepEqual(got, []int{1, 2}) {
		t.Errorf("load clip: %v %v", got, err)
	}
	if _, err := s.CopyNames("cm_Nope", dir, rows); err == nil {
		t.Error("unknown command accepted")
	}
}

func TestSelectCompare(t *testing.T) {
	l, r := t.TempDir(), t.TempDir()
	old := time.Now().Add(-time.Hour)
	write := func(p, text string, mt time.Time) {
		t.Helper()
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		os.Chtimes(p, mt, mt)
	}
	write(filepath.Join(l, "same.txt"), "x", old)
	write(filepath.Join(r, "same.txt"), "x", old)
	write(filepath.Join(l, "newer.txt"), "new", time.Now())
	write(filepath.Join(r, "newer.txt"), "old", old)
	write(filepath.Join(l, "only-left.txt"), "x", old)
	write(filepath.Join(l, "sub", "deep.txt"), "x", time.Now())
	write(filepath.Join(r, "sub", "deep.txt"), "x", old)
	write(filepath.Join(l, "eq", "f"), "x", old)
	write(filepath.Join(r, "eq", "f"), "x", old)

	s := &Select{app: &App{fs: vfs.Local{}}}
	list := func(dir string) []SelectEntry {
		lst, err := vfs.Local{}.List(t.Context(), dir)
		if err != nil {
			t.Fatal(err)
		}
		out := []SelectEntry{{Name: "..", Dir: true, Size: -1}}
		for _, e := range lst.Entries {
			out = append(out, SelectEntry{Name: e.Name, Dir: e.Flags&vfs.FlagDir != 0, Size: e.Size, MTime: e.MTime})
		}
		return out
	}
	res, err := s.Compare(l, r, list(l), list(r), false, false)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(selSorted(res.Left), []string{"newer.txt", "only-left.txt"}) || len(res.Right) != 0 {
		t.Errorf("flat compare: %+v", res)
	}
	res, _ = s.Compare(l, r, list(l), list(r), true, false)
	if !reflect.DeepEqual(selSorted(res.Left), []string{"newer.txt", "only-left.txt", "sub"}) || !reflect.DeepEqual(res.Right, []string{"sub"}) {
		t.Errorf("subdirs compare: %+v", res)
	}
	res, _ = s.Compare(l, l, list(l), list(l), true, false)
	if res.Differs || len(res.Left)+len(res.Right) != 0 {
		t.Errorf("same folder: %+v", res)
	}
}

func selSorted(s []string) []string {
	out := append([]string(nil), s...)
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}
