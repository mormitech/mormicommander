// Tests for custom column sets, column templates and the list-type settings
// (view modes, tooltip rules, external tools): TC's [CustomFields] layout
// reads and writes back, templates with several fields and units parse and
// render, automatic switching by folder picks the right set, and the
// numbered-record sections survive a round trip.

package config

import (
	"strings"
	"testing"
)

func TestColumnSetsRoundTrip(t *testing.T) {
	sets := []ColumnSet{
		{Name: "Pictures", NameWidth: 200, AutoSwitch: "Pictures Photos*", Columns: []Column{
			{Header: "Size", Content: "[=tc.size.K] kB", Width: 60, Align: AlignRight},
			{Header: "W x H", Content: "[=tc.imagewidth]x[=tc.imageheight]", Width: 80, Align: AlignCenter},
		}},
		{Name: "Plain", Columns: []Column{{Header: "Ext", Content: "[=tc.ext]", Width: 40}}, Options: "-1|0"},
	}
	f := NewFile()
	SaveColumnSets(f, sets)
	got := LoadColumnSets(ParseString(f.String()))
	if len(got) != 2 {
		t.Fatalf("got %d sets:\n%s", len(got), f.String())
	}
	for i := range sets {
		a, b := sets[i], got[i]
		if a.Name != b.Name || a.NameWidth != b.NameWidth || a.AutoSwitch != b.AutoSwitch || a.Options != b.Options || len(a.Columns) != len(b.Columns) {
			t.Errorf("set %d: %+v != %+v", i, b, a)
			continue
		}
		for j := range a.Columns {
			if a.Columns[j] != b.Columns[j] {
				t.Errorf("set %d col %d: %+v != %+v", i, j, b.Columns[j], a.Columns[j])
			}
		}
	}
	if !strings.Contains(f.String(), `Headers1=Size\nW x H`) || !strings.Contains(f.String(), "Widths1=200,-60,80") {
		t.Errorf("TC layout not used:\n%s", f.String())
	}
}

func TestLoadColumnSetsTCForms(t *testing.T) {
	f := ParseString(`[CustomFields]
Title1=First
Widths1=-60,70
Headers1=A\nB
Contents1=[=tc.size]\n[=tc.ext]
Headers2=Only
Contents2=[=tc.path]
`)
	sets := LoadColumnSets(f)
	if len(sets) != 2 {
		t.Fatalf("%d sets", len(sets))
	}
	// Two widths for two columns: no name width given.
	if sets[0].NameWidth != 0 || sets[0].Columns[0].Width != 60 || sets[0].Columns[0].Align != AlignRight {
		t.Errorf("set 1 = %+v", sets[0])
	}
	if sets[1].Name != "Columns 2" || sets[1].Columns[0].Width != 0 {
		t.Errorf("set 2 = %+v", sets[1])
	}
}

func TestTemplates(t *testing.T) {
	cases := []struct {
		in       string
		segments int
		fields   []string
	}{
		{"[=tc.size]", 1, []string{"tc.size"}},
		{"[=tc.size.K] kB", 2, []string{"tc.size.K"}},
		{"[=tc.imagewidth]x[=tc.imageheight] px", 4, []string{"tc.imagewidth", "tc.imageheight"}},
		{"[[literal] [=tc.ext]", 2, []string{"tc.ext"}},
		{"[=broken", 1, nil},
		{"[=.x]", 1, nil},
		{"", 0, nil},
	}
	for _, c := range cases {
		segs := ParseTemplate(c.in)
		if len(segs) != c.segments {
			t.Errorf("%q: %d segments %+v", c.in, len(segs), segs)
		}
		var fields []string
		for _, s := range segs {
			if s.Field != "" {
				n := s.Plugin + "." + s.Field
				if s.Unit != "" {
					n += "." + s.Unit
				}
				fields = append(fields, n)
			}
		}
		if strings.Join(fields, ",") != strings.Join(c.fields, ",") {
			t.Errorf("%q: fields %v", c.in, fields)
		}
		if back := FormatTemplate(segs); back != c.in && c.in != "[=broken" && c.in != "[=.x]" {
			t.Errorf("%q formats back as %q", c.in, back)
		}
	}
	got := RenderTemplate("[=tc.size.K] kB ([=tc.ext])", func(s Segment) string {
		if s.Field == "size" && s.Unit == "K" {
			return "12"
		}
		return strings.ToUpper(s.Field)
	})
	if got != "12 kB (EXT)" {
		t.Errorf("render = %q", got)
	}
}

func TestBuiltinFields(t *testing.T) {
	for _, n := range []string{"name", "ext", "size", "writedate", "creationdate", "accessdate",
		"attributestr", "path", "fullname", "comment", "filetype", "permissions", "owner",
		"group", "linktarget", "mime", "imagewidth", "imageheight", "exifdate", "artist"} {
		if _, ok := LookupField("TC", n); !ok {
			t.Errorf("missing built-in field %s", n)
		}
	}
	if _, ok := LookupField("tc", "nope"); ok {
		t.Error("unknown field found")
	}
}

func TestAutoSwitch(t *testing.T) {
	sets := []ColumnSet{{Name: "none"}, {Name: "pics", AutoSwitch: "Pictures Photos*"}, {Name: "src", AutoSwitch: "*/src"}}
	cases := map[string]int{
		"/home/p/Pictures":     1,
		"/home/p/Photos 2024/": 1,
		"/home/p/code/src":     2,
		"/home/p/Documents":    -1,
		`C:\Users\p\Pictures`:  1,
	}
	for in, want := range cases {
		if got := AutoColumnSet(sets, in); got != want {
			t.Errorf("AutoColumnSet(%q) = %d, want %d", in, got, want)
		}
	}
	modes := []ViewMode{{Name: "Thumbs", View: "thumbnails", AutoSwitch: "Pictures"}}
	if AutoViewMode(modes, "/x/Pictures") != 0 || AutoViewMode(modes, "/x/y") != -1 {
		t.Error("AutoViewMode")
	}
}

func TestListRecordsRoundTrip(t *testing.T) {
	f := NewFile()
	modes := []ViewMode{
		{Name: "Thumbs", View: "thumbnails", SortColumn: 3, SortDesc: true, AutoSwitch: "Pictures"},
		{Name: "Code", View: "custom", ColumnSet: "Dev"},
	}
	tips := []TooltipRule{{Mask: "*.jpg *.png", Fields: `[=tc.imagewidth]x[=tc.imageheight]\n[=tc.size.K] kB`}}
	tools := []ExternalTool{{Name: "7-Zip", Kind: "packer", Mask: "*.7z", Command: "7z", Args: "a %A %L"}}
	SaveViewModes(f, modes)
	SaveTooltipRules(f, tips)
	SaveExternalTools(f, tools)
	g := ParseString(f.String())
	gm := LoadViewModes(g)
	if len(gm) != 2 || gm[0] != modes[0] || gm[1] != modes[1] {
		t.Errorf("view modes = %+v", gm)
	}
	gt := LoadTooltipRules(g)
	if len(gt) != 1 || gt[0] != tips[0] {
		t.Errorf("tooltips = %+v", gt)
	}
	lines, ok := TooltipFor(gt, "a.PNG", "", false)
	if !ok || len(lines) != 2 {
		t.Errorf("TooltipFor = %v %v", lines, ok)
	}
	if _, ok := TooltipFor(gt, "a.txt", "", false); ok {
		t.Error("tooltip for txt")
	}
	gx := LoadExternalTools(g)
	if len(gx) != 1 || gx[0] != tools[0] {
		t.Errorf("tools = %+v", gx)
	}
	if tl, ok := ToolFor(gx, "PACKER", "x.7z"); !ok || tl.Command != "7z" {
		t.Error("ToolFor")
	}
	if _, ok := ToolFor(gx, "viewer", "x.7z"); ok {
		t.Error("ToolFor wrong kind")
	}
}

func TestIgnoreListParse(t *testing.T) {
	l := ParseIgnoreList("; comment\r\n# also\r\n\r\n*.bak\r\n/Users/p/My Folder\r\nThumbs.db\r\n")
	cases := []struct {
		name, full string
		dir        bool
		want       bool
	}{
		{"x.bak", "/a/x.bak", false, true},
		{"My Folder", "/Users/p/My Folder", true, true},
		{"My Folder", "/Users/q/My Folder", true, false},
		{"Thumbs.db", "/a/Thumbs.db", false, true},
		{"x.txt", "/a/x.txt", false, false},
	}
	for _, c := range cases {
		if got := l.Hidden(c.name, c.full, c.dir); got != c.want {
			t.Errorf("Hidden(%s) = %v", c.full, got)
		}
	}
}
