// Tests for the custom column template engine, built-in sets and folder rules.
package listing

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestColumnRender(t *testing.T) {
	tm := time.Date(2026, 5, 4, 9, 8, 7, 0, time.UTC)
	it := Item{Name: "photo.final.JPG", Size: 2048, ModTime: tm, CreateTime: tm.Add(-time.Hour), Attr: AttrArchive,
		Mode: 0o640, Owner: "jp", Group: "staff", Dir: "/home/jp", Comment: "line1\nline2", LinkTarget: "x"}
	dir := Item{Name: "src.d", Attr: AttrDir, DirSize: -1, Mode: 0o755, Dir: "/home/jp"}
	f := &Format{Location: time.UTC, AttrStyle: AttrStyleWindows}
	tests := []struct {
		tpl  string
		it   Item
		want string
	}{
		{"[=tc.name]", it, "photo.final"},
		{"[=tc.ext]", it, "JPG"},
		{"[=tc.fullname]", it, "photo.final.JPG"},
		{"[=tc.size]", it, "2,048"},
		{"[=tc.size] bytes", it, "2,048 bytes"},
		{"[=tc.size.k]", it, "2 k"},
		{"[=tc.size.raw]", it, "2048"},
		{"[=tc.size.dynamic]", it, "2.00 k"},
		{"[=tc.size]", dir, "<DIR>"},
		{"[=tc.name]|[=tc.ext]", dir, "src.d|"},
		{"[=tc.writedate]", it, "2026-05-04 09:08"},
		{"[=tc.writedate.dd.MM.yyyy]", it, "04.05.2026"},
		{"[=tc.creationdate.HH:mm]", it, "08:08"},
		{"[=tc.accessdate]", it, ""},
		{"[=tc.attributestr]", it, "-a--"},
		{"[=tc.attributestr.unix]", it, "-rw-r-----"},
		{"[=tc.attributestr.octal]", it, "0640"},
		{"[=tc.permissions]", dir, "drwxr-xr-x"},
		{"[=tc.owner]:[=tc.group]", it, "jp:staff"},
		{"[=tc.path]", it, "/home/jp/"},
		{"[=tc.fullpath]", it, "/home/jp/photo.final.JPG"},
		{"[=tc.comment]", it, "line1 line2"},
		{"[=tc.linktarget]", it, "x"},
		{"[=tc.filetype]", it, "JPEG image"},
		{"[=tc.filetype]", dir, "File folder"},
		{"[=tc.mime]", it, "image/jpeg"},
		{"[x] [=TC.SIZE.K]", it, "[x] 2 k"},
		{"literal only", it, "literal only"},
	}
	for _, tt := range tests {
		c, err := Column{Template: tt.tpl}.Compile()
		if err != nil {
			t.Fatalf("%q: %v", tt.tpl, err)
		}
		if got := c.Render(&tt.it, f); got != tt.want {
			t.Errorf("%q on %s = %q want %q", tt.tpl, tt.it.Name, got, tt.want)
		}
	}
	dirExt := &Format{DirExt: true}
	c, _ := Column{Template: "[=tc.name]|[=tc.ext]"}.Compile()
	if got := c.Render(&dir, dirExt); got != "src|d" {
		t.Errorf("DirExt split: %q", got)
	}
}

func TestTemplateErrors(t *testing.T) {
	for _, tpl := range []string{"[=tc.nosuch]", "[=size]", "[=tc.size", "[=zz.name]"} {
		if _, err := (Column{Template: tpl}).Compile(); err == nil {
			t.Errorf("%q compiled without error", tpl)
		}
	}
	if _, err := (ColumnSet{Columns: []Column{{Title: "bad", Template: "[=tc.x]"}}}).Compile(); err == nil ||
		!strings.Contains(err.Error(), "bad") {
		t.Errorf("set error should name column: %v", err)
	}
}

func TestSortValueKinds(t *testing.T) {
	it := Item{Name: "a", Size: 5, ModTime: time.Unix(100, 0)}
	f := &Format{}
	one, _ := Column{Template: "~ [=tc.size] ~"}.Compile()
	if v := one.SortValue(&it, f); v.Kind != KindNumber || v.Num != 5 {
		t.Errorf("single-field column should sort numerically: %+v", v)
	}
	two, _ := Column{Template: "[=tc.size] [=tc.writedate]"}.Compile()
	if v := two.SortValue(&it, f); v.Kind != KindText {
		t.Errorf("two-field column should sort as text: %+v", v)
	}
	if got := two.Fields(); !reflect.DeepEqual(got, []string{"tc.size", "tc.writedate"}) {
		t.Errorf("Fields = %v", got)
	}
}

func TestRegisterField(t *testing.T) {
	RegisterField("test", ExtraField("rating", "test.rating", KindNumber))
	c, err := Column{Template: "[=test.rating]★"}.Compile()
	if err != nil {
		t.Fatal(err)
	}
	it := Item{Name: "x", Extra: map[string]any{"test.rating": 4}}
	if got := c.Render(&it, &Format{}); got != "4★" {
		t.Errorf("got %q", got)
	}
	empty := Item{Name: "y"}
	if v := c.SortValue(&empty, &Format{}); !v.Empty {
		t.Errorf("missing extra should be empty: %+v", v)
	}
	found := false
	for _, n := range FieldNames() {
		if n == "test.rating" {
			found = true
		}
	}
	if !found {
		t.Error("FieldNames misses registered field")
	}
}

func TestBuiltinSets(t *testing.T) {
	for _, s := range []ColumnSet{BriefSet, FullSet, FullNameSet, CommentsSet} {
		if _, err := s.Compile(); err != nil {
			t.Errorf("%s: %v", s.Name, err)
		}
	}
	want := []string{"Name", "Ext", "Size", "Date", "Attr"}
	if got := FullSet.MustCompile().Titles(); !reflect.DeepEqual(got, want) {
		t.Errorf("Full titles %v", got)
	}
}

func TestChooseSet(t *testing.T) {
	rules := []FolderRule{{Pattern: "Pictures", Set: "Images"}, {Pattern: "/srv/*/logs", Set: "Logs"}, {Pattern: "*.app", Set: "Bundle"}}
	tests := map[string]string{
		"/Users/jp/Pictures":       "Images",
		"/Users/jp/pictures/":      "Images",
		`C:\Users\jp\Pictures`:     "Images",
		"/srv/web/logs":            "Logs",
		"/Applications/Safari.app": "Bundle",
		"/tmp":                     "Full",
	}
	for dir, want := range tests {
		if got := ChooseSet(dir, rules, "Full"); got != want {
			t.Errorf("ChooseSet(%q) = %q want %q", dir, got, want)
		}
	}
}
