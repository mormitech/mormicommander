// Tests for numbered menus, the directory hotlist, the Start menu and .mnu
// menu files.

package config

import (
	"reflect"
	"strings"
	"testing"
)

const dirMenu = `[DirMenu]
menu1=&Home
cmd1=cd $HOME
menu2=-&Projects
menu3=mormi&Commander
cmd3=cd ~/Code/mormiCommander
path3=/tmp
menu4=-
menu5=-Nested
menu6=Deep
cmd6=cd /deep/*.txt
menu7=--
menu8=--
menu9=-
menu10=Thumbs
cmd10=cm_SrcThumbs
menu11=Tabs
cmd11=cd "%COMMANDER_PATH%/work tabs.tab"
menu12=Editor
cmd12="/Applications/My Editor.app" --new
menu14=never read (gap)
cmd14=cd /x
`

func TestHotlistParse(t *testing.T) {
	h := LoadHotlist(ParseString(dirMenu))
	if len(h.Items) != 6 {
		t.Fatalf("top items = %d", len(h.Items))
	}
	proj := h.Items[1]
	if proj.Kind != MenuSubmenu || proj.Title != "&Projects" || len(proj.Children) != 3 {
		t.Fatalf("projects = %+v", proj)
	}
	if proj.Children[1].Kind != MenuSeparator || proj.Children[2].Children[0].Title != "Deep" {
		t.Fatalf("nested structure wrong")
	}
	if got := proj.Children[0].Field("path"); got != "/tmp" {
		t.Errorf("target path = %q", got)
	}
	if h.Items[2].Kind != MenuSeparator {
		t.Error("separator expected")
	}
}

func TestHotlistRoundTrip(t *testing.T) {
	f := ParseString(dirMenu)
	h := LoadHotlist(f)
	h.Save(f)
	h2 := LoadHotlist(f)
	if !reflect.DeepEqual(h.Items, h2.Items) {
		t.Fatal("round trip changed the tree")
	}
	// The gap entry (menu14) is dropped on save since TC never read it.
	if f.Has(SectionDirMenu, "menu14") {
		t.Error("stale entry kept")
	}
	kvs := f.Entries(SectionDirMenu)
	if kvs[len(kvs)-2].Key != "menu12" {
		t.Errorf("numbering: last keys %v", kvs[len(kvs)-2:])
	}
}

func TestHotlistActions(t *testing.T) {
	h := LoadHotlist(ParseString(dirMenu))
	v := Vars{VarCommanderPath: "/opt/mc", "HOME": "/Users/me"}
	tests := []struct {
		path MenuPath
		want Action
	}{
		{MenuPath{0}, Action{Kind: ActionChangeDir, Path: "/Users/me"}},
		{MenuPath{1, 0}, Action{Kind: ActionChangeDir, Path: "/Users/me/Code/mormiCommander", Target: "/tmp"}},
		{MenuPath{1, 2, 0}, Action{Kind: ActionChangeDir, Path: "/deep", Filter: "*.txt"}},
		{MenuPath{3}, Action{Kind: ActionInternal, Command: "cm_SrcThumbs"}},
		{MenuPath{4}, Action{Kind: ActionTabSet, Path: "/opt/mc/work tabs.tab"}},
		{MenuPath{5}, Action{Kind: ActionProgram, Command: "/Applications/My Editor.app", Args: "--new"}},
	}
	for _, tt := range tests {
		it, err := h.Get(tt.path)
		if err != nil {
			t.Fatal(err)
		}
		if got := h.Action(it, v); got != tt.want {
			t.Errorf("%v: %+v want %+v", tt.path, got, tt.want)
		}
	}
}

func TestParseHotlistCommand(t *testing.T) {
	tests := []struct {
		in   string
		want Action
	}{
		{"", Action{}},
		{`cd c:\data\*.doc`, Action{Kind: ActionChangeDir, Path: `c:\data`, Filter: "*.doc"}},
		{`cd c:\*.doc`, Action{Kind: ActionChangeDir, Path: `c:\`, Filter: "*.doc"}},
		{`CD /*.go`, Action{Kind: ActionChangeDir, Path: "/", Filter: "*.go"}},
		{`cd ftp://host/pub`, Action{Kind: ActionChangeDir, Path: "ftp://host/pub"}},
		{`em_backup now`, Action{Kind: ActionUser, Command: "em_backup", Args: "now"}},
		{`CM_List`, Action{Kind: ActionInternal, Command: "CM_List"}},
	}
	for _, tt := range tests {
		if got := ParseHotlistCommand(tt.in); got != tt.want {
			t.Errorf("%q: %+v want %+v", tt.in, got, tt.want)
		}
	}
}

func TestHotlistEditing(t *testing.T) {
	h := &Hotlist{}
	p := h.AddFolder("", "/Users/me/Music/", "")
	if p.String() != "0" || h.Items[0].Title != "Music" || h.Items[0].Field("cmd") != "cd /Users/me/Music/" {
		t.Fatalf("AddFolder: %v %+v", p, h.Items[0])
	}
	h.AddFolder("Both", "/a b", "/c")
	if h.Items[1].Field("cmd") != `cd "/a b"` || h.Items[1].Field("path") != "/c" {
		t.Fatalf("quoted: %+v", h.Items[1])
	}
	if !h.Contains("/a b") || h.Contains("/zzz") {
		t.Error("Contains")
	}
	sub, _ := h.Insert(nil, 0, NewHotlistSubmenu("Sub"))
	if _, err := h.Move(MenuPath{2}, sub, -1); err != nil { // "Both" into Sub
		t.Fatal(err)
	}
	if len(h.Items) != 2 || h.Items[0].Children[0].Title != "Both" {
		t.Fatalf("after move: %d items", len(h.Items))
	}
	if _, err := h.Move(MenuPath{0}, MenuPath{0}, 0); err == nil {
		t.Error("moving a submenu into itself must fail")
	}
	if _, err := h.Move(MenuPath{1}, MenuPath{1}, 0); err == nil {
		t.Error("moving into a non-submenu must fail")
	}
	if len(h.Items) != 2 {
		t.Fatal("failed move changed the tree")
	}
	// Move the submenu after Music: later siblings shift.
	if p, err := h.Move(MenuPath{0}, nil, 1); err != nil || p.String() != "1" {
		t.Fatalf("move = %v %v", p, err)
	}
	if _, err := h.Remove(MenuPath{5}); err == nil {
		t.Error("out of range remove must fail")
	}
	got := h.Filter("both")
	if len(got) != 1 || got[0].Breadcrumb != "Sub" || got[0].Path.String() != "1/0" {
		t.Fatalf("Filter = %+v", got)
	}
	if n := len(h.Filter("")); n != 2 {
		t.Errorf("empty filter = %d", n)
	}
	if n := len(h.Filter("music zzz")); n != 0 {
		t.Errorf("AND filter = %d", n)
	}
}

func TestMenuPathMoveDeep(t *testing.T) {
	// [A[x y] B[z]]: move x (0/0) into B (1) and z stays.
	items := []*MenuItem{
		{Kind: MenuSubmenu, Title: "A", Children: []*MenuItem{{Title: "x"}, {Title: "y"}}},
		{Kind: MenuSubmenu, Title: "B", Children: []*MenuItem{{Title: "z"}}},
	}
	p, err := MenuMove(&items, MenuPath{0, 0}, MenuPath{1}, 0)
	if err != nil || p.String() != "1/0" {
		t.Fatalf("%v %v", p, err)
	}
	if items[1].Children[0].Title != "x" || items[0].Children[0].Title != "y" {
		t.Fatal("wrong result")
	}
	// Moving A (index 0) into B (index 1) shifts B to index 0.
	p, err = MenuMove(&items, MenuPath{0}, MenuPath{1}, -1)
	if err != nil || p.String() != "0/2" || items[0].Title != "B" {
		t.Fatalf("%v %v", p, err)
	}
	if mp, err := ParseMenuPath("0/2"); err != nil || mp.String() != "0/2" {
		t.Errorf("ParseMenuPath = %v %v", mp, err)
	}
	if _, err := ParseMenuPath("a/1"); err == nil {
		t.Error("bad path accepted")
	}
}

func TestAccelerators(t *testing.T) {
	tests := []struct {
		title string
		acc   rune
		plain string
	}{
		{"&Home", 'h', "Home"},
		{"mormi&Commander", 'c', "mormiCommander"},
		{"A && B", 0, "A & B"},
		{"A && &B", 'b', "A & B"},
		{"&Ő", 'ő', "Ő"},
		{"none", 0, "none"},
		{"trailing&", 0, "trailing&"},
	}
	for _, tt := range tests {
		if got := Accelerator(tt.title); got != tt.acc {
			t.Errorf("Accelerator(%q) = %q", tt.title, got)
		}
		if got := StripAccelerator(tt.title); got != tt.plain {
			t.Errorf("Strip(%q) = %q", tt.title, got)
		}
	}
	list := []*MenuItem{{Kind: MenuSeparator}, {Title: "Docs"}, {Title: "&Downloads"}, {Title: "Music"}}
	if i := FindAccelerator(list, 'D'); i != 2 {
		t.Errorf("explicit & wins: %d", i)
	}
	if i := FindAccelerator(list, 'm'); i != 3 {
		t.Errorf("first letter fallback: %d", i)
	}
	if i := FindAccelerator(list, 'x'); i != -1 {
		t.Errorf("no match: %d", i)
	}
}

func TestNumberedMenuEdgeCases(t *testing.T) {
	// Unbalanced: extra "--" at top, missing close at the end.
	m := ParseString("[M]\nmenu1=--\nmenu2=A\nmenu3=-Sub\nmenu4=B\n").Map("M")
	items := ParseNumberedMenu(m, []string{"cmd"})
	if len(items) != 2 || len(items[1].Children) != 1 {
		t.Fatalf("repair failed: %d", len(items))
	}
	// A title starting with "-" can't be stored literally.
	kvs := NumberedMenuKVs([]*MenuItem{{Kind: MenuCommand, Title: "-dash"}}, nil)
	if kvs[0].Value == "-dash" {
		t.Error("title would read back as submenu")
	}
	back := ParseNumberedMenu(map[string]string{"menu1": kvs[0].Value}, nil)
	if back[0].Kind != MenuCommand {
		t.Error("escaped title is not a command")
	}
	it := &MenuItem{}
	it.SetField("CMD", "x")
	if it.Field("cmd") != "x" {
		t.Error("field case")
	}
	it.SetField("cmd", "")
	if _, ok := it.Fields["cmd"]; ok {
		t.Error("empty value should remove field")
	}
	c := (&MenuItem{Kind: MenuSubmenu, Children: []*MenuItem{{Title: "a"}}}).Clone()
	if c.Children[0].Title != "a" {
		t.Error("clone")
	}
}

func TestStartMenu(t *testing.T) {
	f := ParseString("[user]\nmenu1=&Notes\ncmd1=open\nparam1=-a TextEdit %P%N\npath1=%COMMANDER_PATH%\nkey1=CS+N\nmenu2=-Tools\nmenu3=Reread\ncmd3=cm_RereadSource\nkey3=bad+key+\nmenu4=--\n")
	sm := LoadStartMenu(f)
	if len(sm.Items) != 2 || sm.Items[0].Field("param") != "-a TextEdit %P%N" {
		t.Fatalf("items = %+v", sm.Items)
	}
	keys, bad := sm.Hotkeys()
	if len(keys) != 1 || keys[0].Key != (Hotkey{Ctrl: true, Shift: true, Key: "N"}) || len(bad) != 1 {
		t.Fatalf("hotkeys = %+v bad %v", keys, bad)
	}
	a := sm.Action(sm.Items[0], Vars{VarCommanderPath: "/opt/mc"})
	if a.Kind != ActionProgram || a.Command != "open" || a.Args != "-a TextEdit %P%N" || a.Target != "/opt/mc" {
		t.Fatalf("action = %+v", a)
	}
	sm.Items = append(sm.Items, NewStartItem("New", "cm_Edit", "", "", "A+E"))
	sm.Items[2].SetField("icon", "%COMMANDER_PATH%\\new.png")
	sm.Save(f)
	sm2 := LoadStartMenu(f)
	if len(sm2.Items) != 3 || sm2.Items[2].Field("key") != "A+E" || sm2.Items[2].Field("icon") != `%COMMANDER_PATH%\new.png` {
		t.Fatalf("after save: %+v", sm2.Items)
	}
}

func TestMenuFile(t *testing.T) {
	src := `; my menu
POPUP "&Files"
  MENUITEM "&Copy", cm_Copy
  MENUITEM SEPARATOR
  MENUITEM "Say ""hi""", em_hello arg1 arg2
  POPUP "More"
    MENUITEM "Pack", cm_PackFiles
  END_POPUP
END_POPUP
POPUP "&Help"
END_POPUP
`
	items, err := ParseMenuFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || len(items[0].Children) != 4 {
		t.Fatalf("structure: %d", len(items))
	}
	say := items[0].Children[2]
	if say.Title != `Say "hi"` || say.Field("cmd") != "em_hello" || say.Field("param") != "arg1 arg2" {
		t.Fatalf("item = %+v", say)
	}
	out := FormatMenuFile(items)
	again, err := ParseMenuFile(out)
	if err != nil || !reflect.DeepEqual(items, again) {
		t.Fatalf("round trip: %v\n%s", err, out)
	}
	bad := []string{
		`POPUP "x"`,
		`END_POPUP`,
		`MENUITEM noquote, cm_x`,
		`MENUITEM "unterminated, cm_x`,
		`FOO "x"`,
	}
	for _, b := range bad {
		if _, err := ParseMenuFile(b); err == nil {
			t.Errorf("accepted %q", b)
		} else if !strings.Contains(err.Error(), "config:") {
			t.Errorf("error without prefix: %v", err)
		}
	}
}
