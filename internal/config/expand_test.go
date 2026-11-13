// Tests for variable expansion, file masks and colours.

package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestExpand(t *testing.T) {
	home, _ := os.UserHomeDir()
	t.Setenv("MC_TEST_VAR", "envval")
	v := Vars{VarCommanderPath: "/opt/mc", "Custom": "cv"}
	tests := []struct{ in, want string }{
		{"%COMMANDER_PATH%/bars/a.bar", "/opt/mc/bars/a.bar"},
		{"%commander_path%", "/opt/mc"},
		{"%CUSTOM%-x", "cv-x"},
		{"%MC_TEST_VAR%", "envval"},
		{"$MC_TEST_VAR/x", "envval/x"},
		{"${MC_TEST_VAR}y", "envvaly"},
		{"$HOME/docs", home + "/docs"},
		{"~/docs", home + "/docs"},
		{"~", home},
		{"~user", "~user"},
		{"100%", "100%"},
		{"50% and 60%", "50% and 60%"},
		{"%UNKNOWN_XYZ_VAR%", "%UNKNOWN_XYZ_VAR%"},
		{`\\server\c$\share`, `\\server\c$\share`},
		{"$Recycle.Bin", "$Recycle.Bin"},
		{"$", "$"},
		{"${}", "${}"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := v.Expand(tt.in); got != tt.want {
			t.Errorf("Expand(%q) = %q want %q", tt.in, got, tt.want)
		}
	}
}

func TestContract(t *testing.T) {
	home, _ := os.UserHomeDir()
	prog := filepath.Join(home, "apps", "mc")
	v := Vars{VarCommanderPath: prog}
	sep := string(filepath.Separator)
	tests := []struct{ in, want string }{
		{filepath.Join(prog, "default.bar"), "%COMMANDER_PATH%" + sep + "default.bar"},
		{prog, "%COMMANDER_PATH%"},
		{filepath.Join(home, "x"), homeVarSpelling + sep + "x"},
		{prog + "x", homeVarSpelling + sep + "apps" + sep + "mcx"},
		{"/elsewhere/file", "/elsewhere/file"},
		{"", ""},
	}
	for _, tt := range tests {
		got := v.Contract(tt.in)
		if got != tt.want {
			t.Errorf("Contract(%q) = %q want %q", tt.in, got, tt.want)
		}
		if tt.in != "" && v.Expand(got) != tt.in {
			t.Errorf("Expand(Contract(%q)) = %q", tt.in, v.Expand(got))
		}
	}
}

func TestMask(t *testing.T) {
	tests := []struct {
		mask, name, full string
		dir, want        bool
	}{
		{"*.txt", "a.TXT", "", false, true},
		{"*.txt *.doc", "b.doc", "", false, true},
		{"*.txt;*.doc", "b.docx", "", false, false},
		{"*.*", "noext", "", false, true},
		{"readme.*", "README", "", false, true},
		{"a?c", "abc", "", false, true},
		{"a?c", "abbc", "", false, false},
		{"*.txt | readme.txt", "readme.txt", "", false, false},
		{"*.txt | readme.txt", "notes.txt", "", false, true},
		{"| *.bak", "x.c", "", false, true},
		{"| *.bak", "x.bak", "", false, false},
		{"node_modules/", "node_modules", "", true, true},
		{"node_modules/", "node_modules", "", false, false},
		{`"my file.txt"`, "my file.txt", "", false, true},
		{"[x].txt", "[x].txt", "", false, true},
		{"[x].txt", "x.txt", "", false, false},
		{"*/build/*", "out.o", "/src/build/out.o", false, true},
		{`src\*.go`, "a.go", "/home/u/src/a.go", false, true},
		{`src\*.go`, "a.go", "", false, false},
		{"ő*", "őz", "", false, true},
		{"?z", "őz", "", false, true},
		{"", "x", "", false, false},
	}
	for _, tt := range tests {
		if got := ParseMask(tt.mask).Match(tt.name, tt.full, tt.dir); got != tt.want {
			t.Errorf("mask %q on %q/%q dir=%v = %v want %v", tt.mask, tt.name, tt.full, tt.dir, got, tt.want)
		}
	}
	if s := ParseMask(`*.c "a b.txt" | *.h`).String(); s != `*.c "a b.txt" | *.h` {
		t.Errorf("String = %q", s)
	}
}

func TestParseColor(t *testing.T) {
	tests := []struct {
		in   string
		want Color
		hex  string
		err  bool
	}{
		{"#ff0000", RGB(255, 0, 0), "#ff0000", false},
		{"#0f0", RGB(0, 255, 0), "#00ff00", false},
		{"rgb(1, 2, 3)", RGB(1, 2, 3), "#010203", false},
		{"255", RGB(255, 0, 0), "#ff0000", false},      // COLORREF red
		{"16711680", RGB(0, 0, 255), "#0000ff", false}, // COLORREF blue
		{"-1", DefaultColor, "", false},
		{"", DefaultColor, "", false},
		{"#12345", DefaultColor, "", true},
		{"rgb(300,0,0)", DefaultColor, "", true},
		{"blue", DefaultColor, "", true},
		{"99999999", DefaultColor, "", true},
	}
	for _, tt := range tests {
		got, err := ParseColor(tt.in)
		if (err != nil) != tt.err || got != tt.want || got.Hex() != tt.hex {
			t.Errorf("ParseColor(%q) = %v,%v (%s) want %v err=%v", tt.in, got, err, got.Hex(), tt.want, tt.err)
		}
	}
	if RGB(1, 2, 3).INI() != "197121" {
		t.Errorf("INI = %s", RGB(1, 2, 3).INI())
	}
}

func TestColorSchemeRoundTrip(t *testing.T) {
	in := "[Colors]\n; mine\nForeColor=0\nBackColor=16777215\nMarkColor=255\nInverseCursor=1\nColorFilter1=*.exe *.com\nColorFilter1Color=32768\nColorFilter2=>Archives\nColorFilter2Color=128\nColorFilter3=src/\nColorFilter3Color=8421376\nSomethingElse=keep\n"
	f := ParseString(in)
	s := LoadColorScheme(f, SectionColors)
	if s.Fore != RGB(0, 0, 0) || s.Back != RGB(255, 255, 255) || s.Mark != RGB(255, 0, 0) || !s.InverseCursor {
		t.Fatalf("scheme = %+v", s)
	}
	if !s.Cursor.IsDefault() || len(s.Filters) != 3 {
		t.Fatalf("cursor %v filters %d", s.Cursor, len(s.Filters))
	}
	tests := []struct {
		name string
		dir  bool
		want Color
	}{
		{"setup.EXE", false, RGB(0, 128, 0)},
		{"x.zip", false, s.Fore}, // ">Archives" refers to a TC search, never matches
		{"src", true, RGB(0, 128, 128)},
		{"src", false, s.Fore},
	}
	for _, tt := range tests {
		if got := s.FileColor(tt.name, "", tt.dir); got != tt.want {
			t.Errorf("FileColor(%s) = %v want %v", tt.name, got.Hex(), tt.want.Hex())
		}
	}
	s.Filters = s.Filters[:1]
	s.Cursor = RGB(1, 1, 1)
	SaveColorScheme(f, SectionColors, s)
	g := LoadColorScheme(f, SectionColors)
	if len(g.Filters) != 1 || g.Cursor != RGB(1, 1, 1) {
		t.Fatalf("after save: %+v", g)
	}
	if v, _ := f.Get(SectionColors, "SomethingElse"); v != "keep" {
		t.Error("unknown key lost")
	}
	if f.Has(SectionColors, "ColorFilter2") {
		t.Error("stale filter left")
	}
}

func TestNamedSchemes(t *testing.T) {
	f := NewFile()
	for _, s := range BuiltinSchemes() {
		SaveNamedScheme(f, s)
	}
	got := NamedSchemes(f)
	if len(got) != 3 || got[0].Name != "Classic blue" || got[2].Name != "Default" {
		t.Fatalf("schemes = %v", got)
	}
	if got[1].Back != RGB(30, 30, 30) {
		t.Errorf("dark back = %s", got[1].Back.Hex())
	}
	if !DeleteNamedScheme(f, "Dark") || len(NamedSchemes(f)) != 2 {
		t.Error("delete failed")
	}
}

func TestPathsEqual(t *testing.T) {
	if !pathsEqual("/a/b/", "/a/b") {
		t.Error("trailing slash")
	}
	if runtime.GOOS == "windows" {
		if !pathsEqual(`C:\Data`, "c:/data") {
			t.Error("windows case/slash")
		}
	} else if pathsEqual("/A", "/a") {
		t.Error("unix should be case-sensitive")
	}
}
