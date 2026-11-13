// Tests for hotkey parsing, keymap overrides, histories and frequent
// folders.

package config

import (
	"reflect"
	"strconv"
	"testing"
)

func TestParseHotkey(t *testing.T) {
	tests := []struct {
		in   string
		want Hotkey
		str  string
		err  bool
	}{
		{"F5", Hotkey{Key: "F5"}, "F5", false},
		{"CS+F1", Hotkey{Ctrl: true, Shift: true, Key: "F1"}, "CS+F1", false},
		{"sc+f1", Hotkey{Ctrl: true, Shift: true, Key: "F1"}, "CS+F1", false},
		{"A+F7", Hotkey{Alt: true, Key: "F7"}, "A+F7", false},
		{"C++", Hotkey{Ctrl: true, Key: "+"}, "C++", false},
		{"A+NUM+", Hotkey{Alt: true, Key: "Num+"}, "A+Num+", false},
		{"C+d", Hotkey{Ctrl: true, Key: "D"}, "C+D", false},
		{"W+C", Hotkey{Win: true, Key: "C"}, "W+C", false},
		{"Ctrl+Shift+F1", Hotkey{Ctrl: true, Shift: true, Key: "F1"}, "CS+F1", false},
		{"Cmd+Backspace", Hotkey{Win: true, Key: "Back"}, "W+Back", false},
		{"Option+PageDown", Hotkey{Alt: true, Key: "PgDn"}, "A+PgDn", false},
		{"Ctrl+Num5", Hotkey{Ctrl: true, Key: "Num5"}, "C+Num5", false},
		{"C+Return", Hotkey{Ctrl: true, Key: "Enter"}, "C+Enter", false},
		{"Alt+Space", Hotkey{Alt: true, Key: "Space"}, "A+Space", false},
		{"", Hotkey{}, "", true},
		{"C+F99", Hotkey{}, "", true},
		{"Hyper+X", Hotkey{}, "", true},
		{"C+Bogus", Hotkey{}, "", true},
	}
	for _, tt := range tests {
		got, err := ParseHotkey(tt.in)
		if (err != nil) != tt.err {
			t.Errorf("ParseHotkey(%q) err = %v", tt.in, err)
			continue
		}
		if err == nil && (got != tt.want || got.String() != tt.str) {
			t.Errorf("ParseHotkey(%q) = %+v (%s) want %+v (%s)", tt.in, got, got, tt.want, tt.str)
		}
	}
	if l := (Hotkey{Ctrl: true, Alt: true, Shift: true, Win: true, Key: "X"}).Label(); l != "Ctrl+Alt+Shift+Cmd+X" {
		t.Errorf("Label = %q", l)
	}
}

func TestFromEventCode(t *testing.T) {
	tests := map[string]string{
		"KeyA": "A", "Digit7": "7", "ArrowLeft": "Left", "NumpadAdd": "Num+", "Numpad3": "Num3",
		"F12": "F12", "Backspace": "Back", "Backquote": "`", "Slash": "/", "ShiftLeft": "",
		"MetaRight": "", "Enter": "Enter", "Escape": "Esc", "PageUp": "PgUp", "Delete": "Del",
	}
	for code, want := range tests {
		if got := FromEventCode(code); got != want {
			t.Errorf("FromEventCode(%q) = %q want %q", code, got, want)
		}
	}
}

func TestKeymapLoadSave(t *testing.T) {
	f := ParseString("[Shortcuts]\n; my keys\nC+D=cm_DirectoryHotlist\nCS+F1=cm_SrcThumbs\nA+F1=\nnonsense+=x\n[ShortcutsWin]\nC=cm_Copy\n")
	km, bad := LoadKeymap(f)
	if len(bad) != 1 {
		t.Errorf("bad = %v", bad)
	}
	if b, ok := km.Lookup(Hotkey{Win: true, Key: "c"}); !ok || b.Command != "cm_Copy" {
		t.Errorf("win binding = %+v %v", b, ok)
	}
	defaults := map[string]string{"A+F1": "cm_LeftOpenDrives", "F5": "cm_Copy", "Ctrl+D": "cm_Old"}
	eff := km.Effective(defaults)
	want := map[string]string{"F5": "cm_Copy", "C+D": "cm_DirectoryHotlist", "CS+F1": "cm_SrcThumbs", "W+C": "cm_Copy"}
	if !reflect.DeepEqual(eff, want) {
		t.Errorf("Effective = %v", eff)
	}
	if keys := KeysFor(eff, "CM_COPY"); !reflect.DeepEqual(keys, []string{"F5", "W+C"}) {
		t.Errorf("KeysFor = %v", keys)
	}
	km.Reset(Hotkey{Alt: true, Key: "F1"})
	km.Bind(Hotkey{Win: true, Key: "v"}, "cm_Paste")
	km.Unbind(Hotkey{Key: "F5"})
	km.Save(f)
	if got := f.String(); got != "[Shortcuts]\n; my keys\nC+D=cm_DirectoryHotlist\nCS+F1=cm_SrcThumbs\nF5=\n[ShortcutsWin]\nC=cm_Copy\nV=cm_Paste\n" {
		t.Errorf("saved:\n%s", got)
	}
	km2, _ := LoadKeymap(f)
	if !reflect.DeepEqual(km.Bindings(), km2.Bindings()) {
		t.Error("round trip differs")
	}
	km.RestoreDefaults()
	if len(km.Bindings()) != 0 {
		t.Error("RestoreDefaults")
	}
}

func TestHistory(t *testing.T) {
	h := NewHistory(3, false)
	for _, s := range []string{"a", "b", "", "c", "a", "d"} {
		h.Add(s)
	}
	if !reflect.DeepEqual(h.Items(), []string{"d", "a", "c"}) {
		t.Fatalf("items = %v", h.Items())
	}
	h.FoldCase = true
	h.Add("A")
	if !reflect.DeepEqual(h.Items(), []string{"A", "d", "c"}) {
		t.Fatalf("fold = %v", h.Items())
	}
	if !h.Remove("D") || h.Len() != 2 {
		t.Fatal("remove")
	}
	h.SetMax(1)
	if h.Len() != 1 {
		t.Fatal("SetMax")
	}
	h.Clear()
	if h.Len() != 0 {
		t.Fatal("Clear")
	}
	var zero History
	for i := 0; i < 40; i++ {
		zero.Add(strconv.Itoa(i))
	}
	if zero.Len() != DefaultHistorySize {
		t.Errorf("default size = %d", zero.Len())
	}
}

func TestHistoryLoadSave(t *testing.T) {
	f := ParseString("[LeftHistory]\n0=/a\n1=/b/\n2=/b\n3=\n4=/c\n6=/never\n")
	h := NewHistory(10, true)
	h.Load(f, SectionLeftHistory)
	if !reflect.DeepEqual(h.Items(), []string{"/a", "/b/", "/c"}) {
		t.Fatalf("loaded %v", h.Items())
	}
	h.Add("/z")
	h.Save(f, SectionLeftHistory)
	if got := f.String(); got != "[LeftHistory]\n0=/z\n1=/a\n2=/b/\n3=/c\n" {
		t.Fatalf("saved %q", got)
	}
}

func TestFrequent(t *testing.T) {
	q := NewFrequent()
	q.Max = 3
	for _, p := range []string{"/a", "/b", "/a", "/c", "/a", "/b", "/d"} {
		q.Visit(p)
	}
	top := q.Top(10)
	if len(top) != 3 || top[0] != "/a" || top[1] != "/b" {
		t.Fatalf("top = %v", top)
	}
	f := NewFile()
	q.Save(f, SectionFrequentDirs)
	q2 := NewFrequent()
	q2.Load(f, SectionFrequentDirs)
	if !reflect.DeepEqual(q2.Top(10), top) {
		t.Fatalf("reloaded %v", q2.Top(10))
	}
	q2.Forget("/a")
	if q2.Top(1)[0] != "/b" {
		t.Error("Forget")
	}
	q2.Clear()
	if len(q2.Top(5)) != 0 {
		t.Error("Clear")
	}
	bad := ParseString("[F]\n0=x|/a\n1=-1|/b\n2=nopipe\n3=2|/c\n")
	q3 := NewFrequent()
	q3.Load(bad, "F")
	if !reflect.DeepEqual(q3.Top(5), []string{"/c"}) {
		t.Errorf("bad entries: %v", q3.Top(5))
	}
}
