// Tests for file masks, the INI reader and internal associations.
package launch

import (
	"reflect"
	"strings"
	"testing"
)

func TestMask(t *testing.T) {
	tests := []struct {
		mask, name string
		dir, want  bool
	}{
		{"*.txt", "a.TXT", false, true},
		{"*.txt", "a.txt.bak", false, false},
		{"*.txt *.md", "x.md", false, true},
		{"*.txt;*.md", "x.md", false, true},
		{"*.*", "noext", false, true},
		{"*", "anything", false, true},
		{"*.", "noext", false, true},
		{"*.", "a.b", false, false},
		{"?.c", "a.c", false, true},
		{"?.c", "ab.c", false, false},
		{"*.jp*g", "photo.jpeg", false, true},
		{"*.jp*g", "photo.jpg", false, true},
		{"*.txt | read*", "readme.txt", false, false},
		{"*.txt | read*", "notes.txt", false, true},
		{`src\`, "src", true, true},
		{`src\`, "src", false, false},
		{`*/`, "any", true, true},
		{`"my file.txt"`, "My File.txt", false, true},
		{"", "a", false, false},
		{"ä*.txt", "Äpfel.txt", false, true},
		{"?pfel.txt", "äpfel.txt", false, true},
		{"a*b*c", "aXXbYYc", false, true},
		{"a*b*c", "aXXbYY", false, false},
	}
	for _, tt := range tests {
		if got := MatchMask(tt.mask, tt.name, tt.dir); got != tt.want {
			t.Errorf("MatchMask(%q, %q, dir=%v) = %v, want %v", tt.mask, tt.name, tt.dir, got, tt.want)
		}
	}
	if !ParseMask("").Empty() || ParseMask("*.x").Empty() {
		t.Error("Empty is wrong")
	}
}

func TestINIRoundTripAndEncodings(t *testing.T) {
	src := "; top comment is dropped\n[Configuration]\r\nEditor = notepad.exe \"%1\"\r\n; keep me\n\n[left]\npath=C:\\\nbad line\n"
	f := ParseINI([]byte(src))
	s := f.Section("configuration")
	if s == nil {
		t.Fatal("section not found case-insensitively")
	}
	if v, ok := s.Get("EDITOR"); !ok || v != `notepad.exe "%1"` {
		t.Errorf("Editor = %q, %v", v, ok)
	}
	out := string(f.Bytes("\n"))
	want := "[Configuration]\nEditor=notepad.exe \"%1\"\n; keep me\n\n[left]\npath=C:\\\n"
	if out != want {
		t.Errorf("round trip =\n%s\nwant\n%s", out, want)
	}
	// UTF-16 LE with BOM, as Total Commander saves wincmd.ini.
	u16 := []byte{0xFF, 0xFE}
	for _, r := range "[A]\r\nk=ő\r\n" {
		u16 = append(u16, byte(r), byte(r>>8))
	}
	if v, _ := ParseINI(u16).Section("A").Get("k"); v != "ő" {
		t.Errorf("UTF-16 value = %q", v)
	}
	if v, _ := ParseINI(append([]byte{0xEF, 0xBB, 0xBF}, "[B]\nx=1\n"...)).Section("B").Get("x"); v != "1" {
		t.Errorf("UTF-8 BOM value = %q", v)
	}
	f.RemoveSection("LEFT")
	if f.Section("left") != nil {
		t.Error("RemoveSection failed")
	}
	var sb strings.Builder
	if _, err := f.WriteTo(&sb); err != nil || !strings.HasPrefix(sb.String(), "[Configuration]") {
		t.Errorf("WriteTo = %q, %v", sb.String(), err)
	}
}

const assocINI = `[Associations]
2=*.*
2_edit=vim %1
1=*.txt *.md
1_desc=Text files
1_icon=/icons/txt.png
1_open=code --wait "%1"
1_VIEW=cm_List /T
1_Print=lp "%1"
3_open=no mask, skipped
x_open=ignored
`

func TestAssociationsLoadStoreLookup(t *testing.T) {
	a := LoadAssociations(ParseINI([]byte(assocINI)))
	if len(a.List) != 2 {
		t.Fatalf("got %d associations, want 2: %+v", len(a.List), a.List)
	}
	first := a.List[0]
	if first.Mask != "*.txt *.md" || first.Description != "Text files" || first.Icon != "/icons/txt.png" {
		t.Errorf("first = %+v", first)
	}
	wantVerbs := []VerbLine{{"open", `code --wait "%1"`}, {"view", "cm_List /T"}, {"Print", `lp "%1"`}}
	if !reflect.DeepEqual(first.Verbs, wantVerbs) {
		t.Errorf("verbs = %+v", first.Verbs)
	}
	if c, ok := a.Lookup("README.MD", false, VerbOpen); !ok || c != `code --wait "%1"` {
		t.Errorf("open lookup = %q %v", c, ok)
	}
	// *.txt has no editor, so the later *.* entry provides it.
	if c, ok := a.Lookup("a.txt", false, VerbEdit); !ok || c != "vim %1" {
		t.Errorf("edit fallback = %q %v", c, ok)
	}
	if _, ok := a.Lookup("a.jpg", false, VerbOpen); ok {
		t.Error("no open command for jpg expected")
	}
	if a.Find("x.md", false) != &a.List[0] || a.Find("x.bin", false) != &a.List[1] {
		t.Error("Find picked the wrong entry")
	}
	var nilA *Associations
	if nilA.Find("x", false) != nil {
		t.Error("nil list should find nothing")
	}

	f := &INIFile{}
	a.Store(f)
	b := LoadAssociations(f)
	if !reflect.DeepEqual(a, b) {
		t.Errorf("store/load changed the list:\n%+v\n%+v\n%s", a, b, f.Bytes("\n"))
	}

	x := &b.List[0]
	x.SetCommand("view", "")
	x.SetCommand("edit", "nano %1")
	x.SetCommand("open", "gedit %1")
	if c, _ := x.Command("EDIT"); c != "nano %1" {
		t.Errorf("SetCommand edit = %q", c)
	}
	if _, ok := x.Command("view"); ok {
		t.Error("view should be removed")
	}
	if c, _ := x.Command("open"); c != "gedit %1" {
		t.Errorf("open replaced = %q", c)
	}
}

func TestParseAction(t *testing.T) {
	if a := ParseAction("cm_List /T"); a.Internal != "cm_List" || a.Args != "/T" {
		t.Errorf("internal = %+v", a)
	}
	if a := ParseAction(`"C:\Program Files\x.exe" -n "%1"`); a.Tool.Program != `C:\Program Files\x.exe` || a.Tool.Params != `-n "%1"` {
		t.Errorf("program = %+v", a)
	}
}

func TestOpenAssociated(t *testing.T) {
	a := LoadAssociations(ParseINI([]byte(assocINI)))
	l, rec := fakeLauncher(t, "linux", nil)
	ctx := ParamContext{Source: Panel{Dir: "/d", Current: "my notes.txt"}}

	res, err := l.OpenAssociated(a, VerbOpen, ctx)
	if err != nil || !res.Handled || res.Internal != "" {
		t.Fatalf("open = %+v, %v", res, err)
	}
	sameCmd(t, rec.last(t), Command{Path: "code", Args: []string{"--wait", "/d/my notes.txt"}, Dir: "/d"})

	res, _ = l.OpenAssociated(a, VerbView, ctx)
	if !res.Handled || res.Internal != "cm_List" || res.Args != "/T" {
		t.Errorf("view = %+v", res)
	}
	res, _ = l.OpenAssociated(a, VerbOpen, ParamContext{File: "/d/pic.jpg"})
	if res.Handled {
		t.Errorf("jpg open should fall back to the system, got %+v", res)
	}
	n := len(rec.cmds)
	res, _ = l.OpenAssociated(a, VerbEdit, ParamContext{File: "/d/pic.jpg", Source: Panel{Dir: "/d"}})
	if !res.Handled || len(rec.cmds) != n+1 {
		t.Fatalf("edit = %+v", res)
	}
	sameCmd(t, rec.last(t), Command{Path: "vim", Args: []string{"/d/pic.jpg"}, Dir: "/d"})
}
