// Tests for button bar files, parameter placeholders and user commands.

package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
)

const sampleBar = `; my bar
[Buttonbar]
Buttoncount=5
button1=%COMMANDER_PATH%\WCMICONS.DLL,3
cmd1=cm_RereadSource
menu1=Reread
button2=
cmd2=
button3=notepad.exe
cmd3=notepad.exe
param3=%P%N
path3=c:\temp
menu3=Edit
iconic3=1
button4=%COMMANDER_PATH%\WCMICONS.DLL,10
cmd4=%COMMANDER_PATH%\tools.bar
menu4=Tools
`

func TestParseBar(t *testing.T) {
	bar := ParseBar(ParseString(sampleBar))
	if len(bar.Buttons) != 5 {
		t.Fatalf("buttons = %d", len(bar.Buttons))
	}
	tests := []struct {
		i              int
		sep, sub       bool
		icon           string
		idx            int
		cmd, tip, prms string
		iconic         int
	}{
		{0, false, false, `%COMMANDER_PATH%\WCMICONS.DLL`, 3, "cm_RereadSource", "Reread", "", 0},
		{1, true, false, "", 0, "", "", "", 0},
		{2, false, false, "notepad.exe", 0, "notepad.exe", "Edit", "%P%N", 1},
		{3, false, true, `%COMMANDER_PATH%\WCMICONS.DLL`, 10, `%COMMANDER_PATH%\tools.bar`, "Tools", "", 0},
		{4, true, false, "", 0, "", "", "", 0}, // missing number = separator
	}
	for _, tt := range tests {
		b := bar.Buttons[tt.i]
		icon, idx := b.IconFile()
		if b.IsSeparator() != tt.sep || b.IsSubBar() != tt.sub || icon != tt.icon || idx != tt.idx ||
			b.Command != tt.cmd || b.Tooltip != tt.tip || b.Params != tt.prms || b.Iconic != tt.iconic {
			t.Errorf("button %d = %+v", tt.i, b)
		}
	}
}

func TestBarEditAndSave(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "my.bar")
	if err := os.WriteFile(p, []byte(sampleBar), 0o644); err != nil {
		t.Fatal(err)
	}
	bar, _, err := LoadBarFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bar.Remove(4); err != nil {
		t.Fatal(err)
	}
	if err := bar.Move(0, 2); err != nil {
		t.Fatal(err)
	}
	bar.Insert(-1, ButtonForFile("/Applications/Calc.app"))
	if _, err := bar.Remove(99); err == nil {
		t.Error("remove out of range")
	}
	if err := bar.SaveBarFile(p); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(p)
	if !strings.HasPrefix(string(data), "; my bar\n[Buttonbar]\nButtoncount=5\n") {
		t.Fatalf("saved:\n%s", data)
	}
	bar2, _, _ := LoadBarFile(p)
	if !reflect.DeepEqual(bar, bar2) {
		t.Fatalf("round trip:\n%+v\n%+v", bar, bar2)
	}
	last := bar2.Buttons[4]
	if last.Command != "/Applications/Calc.app" || last.Path != "/Applications" || last.Tooltip != "Calc.app" {
		t.Errorf("ButtonForFile = %+v", last)
	}
	// A new file is created when missing.
	np := filepath.Join(dir, "new", "x.bar")
	if err := DefaultBar().SaveBarFile(np); err != nil {
		t.Fatal(err)
	}
	b3, _, err := LoadBarFile(np)
	if err != nil || len(b3.Buttons) != len(DefaultBar().Buttons) {
		t.Fatalf("default bar: %v", err)
	}
}

func TestButtonForFileRoot(t *testing.T) {
	if b := ButtonForFile("/tool"); b.Path != "/" {
		t.Errorf("path = %q", b.Path)
	}
	if b := ButtonForFile("tool"); b.Path != "" || b.Tooltip != "tool" {
		t.Errorf("bare = %+v", b)
	}
	if b := ButtonForFile("/my apps/x"); b.Command != `"/my apps/x"` {
		t.Errorf("quoted = %q", b.Command)
	}
}

func TestExpandParams(t *testing.T) {
	t.Setenv("MC_PARAM_TEST", "env")
	now := time.Date(2026, 11, 13, 9, 5, 7, 0, time.UTC)
	ctx := ParamContext{
		SourceDir: "/src", TargetDir: "/dst/", Current: "photo.jpeg", TargetCurrent: "other.txt",
		Selected: []string{"a.txt", "my file.txt"}, Vars: Vars{VarCommanderPath: "/opt/mc"},
		Now: now, Separator: "/",
	}
	tests := []struct {
		in, want string
		prompt   bool
	}{
		{"%P%N", "/src/photo.jpeg", false},
		{"%T%M", "/dst/other.txt", false},
		{"%O.%E", "photo.jpeg", false},
		{"%S", `a.txt "my file.txt"`, false},
		{"%Q%S", "a.txt my file.txt", false},
		{"%P%S", `/src/a.txt "/src/my file.txt"`, false},
		{"%R", "other.txt", false},
		{"%T%R", "/dst/other.txt", false},
		{"100%%", "100%", false},
		{"?%N", "photo.jpeg", true},
		{"%COMMANDER_PATH%/x", "/opt/mc/x", false},
		{"%MC_PARAM_TEST%", "env", false},
		{"%UNKNOWN_ZZ%", "%UNKNOWN_ZZ%", false},
		{"%$DATE%", "2026-11-13", false},
		{"%$TIME%", "09:05:07", false},
		{"%$DATE:YYYYMMDD%-%$TIME:hh-mm%", "20261113-09-05", false},
		{"%p%n", "/src/photo.jpeg", false},
		{"trailing %", "trailing %", false},
		{"%X", "%X", false},
	}
	for _, tt := range tests {
		got, err := ExpandParams(tt.in, ctx)
		if err != nil {
			t.Fatal(err)
		}
		if got.Text != tt.want || got.Prompt != tt.prompt {
			t.Errorf("ExpandParams(%q) = %q prompt=%v want %q %v", tt.in, got.Text, got.Prompt, tt.want, tt.prompt)
		}
	}
	// No selection: %S uses the file under the cursor; ".." is never used.
	c2 := ctx
	c2.Selected = nil
	if got, _ := ExpandParams("%S", c2); got.Text != "photo.jpeg" {
		t.Errorf("fallback = %q", got.Text)
	}
	c2.Current = ".."
	if got, _ := ExpandParams("[%S]", c2); got.Text != "[]" {
		t.Errorf("parent = %q", got.Text)
	}
}

func TestExpandParamsListFiles(t *testing.T) {
	dir := t.TempDir()
	ctx := ParamContext{SourceDir: "/src", Selected: []string{"ő.txt", "b"}, TempDir: dir, Separator: "/"}
	tests := []struct {
		code  string
		want  []string
		utf16 bool
	}{
		{"%L", []string{"/src/ő.txt", "/src/b"}, false},
		{"%F", []string{"ő.txt", "b"}, false},
		{"%UL", []string{"/src/ő.txt", "/src/b"}, false},
		{"%Z", []string{"/src/ő.txt", "/src/b"}, false},
		{"%UF", []string{"ő.txt", "b"}, false},
		{"%WL", []string{"/src/ő.txt", "/src/b"}, true},
		{"%WF", []string{"ő.txt", "b"}, true},
	}
	for _, tt := range tests {
		got, err := ExpandParams(tt.code, ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(got.ListFiles) != 1 || got.Text != got.ListFiles[0] || filepath.Dir(got.Text) != dir {
			t.Fatalf("%s: %+v", tt.code, got)
		}
		data, _ := os.ReadFile(got.Text)
		text := string(data)
		if tt.utf16 {
			if data[0] != 0xFF || data[1] != 0xFE {
				t.Fatalf("%s: no BOM", tt.code)
			}
			u := make([]uint16, (len(data)-2)/2)
			for i := range u {
				u[i] = uint16(data[2+2*i]) | uint16(data[3+2*i])<<8
			}
			text = string(utf16.Decode(u))
		}
		lines := strings.Fields(strings.ReplaceAll(text, "\r", ""))
		// ANSI lists are UTF-8 off Windows; on Windows ő is not in cp1252,
		// so the list falls back to UTF-8 there too.
		if !reflect.DeepEqual(lines, tt.want) {
			t.Errorf("%s: lines %q want %q", tt.code, lines, tt.want)
		}
	}
	bad := ctx
	bad.TempDir = filepath.Join(dir, "missing")
	if _, err := ExpandParams("%L", bad); err == nil {
		t.Error("expected error for a missing temp folder")
	}
}

func TestUserCommands(t *testing.T) {
	f := ParseString("[em_backup]\ncmd=rsync\nparam=-a %P %T\nmenu=Backup\niconic=1\n[em_ bad]\ncmd=x\n[Other]\nk=v\n[em_alpha]\ncmd=cm_List\n")
	cmds := LoadUserCommands(f)
	if len(cmds) != 2 || cmds[0].Name != "em_alpha" || cmds[1].Params != "-a %P %T" || cmds[1].Iconic != 1 {
		t.Fatalf("cmds = %+v", cmds)
	}
	u, ok := FindUserCommand(cmds, "EM_BACKUP")
	if !ok || u.Button().Command != "em_backup" || u.Button().Tooltip != "Backup" {
		t.Fatalf("find = %+v %v", u, ok)
	}
	if err := SaveUserCommand(f, UserCommand{Name: "em_new", Command: "echo", Icon: "x.png"}); err != nil {
		t.Fatal(err)
	}
	if err := SaveUserCommand(f, UserCommand{Name: "cm_bad"}); err == nil {
		t.Error("invalid name accepted")
	}
	if len(LoadUserCommands(f)) != 3 {
		t.Error("save")
	}
	if !DeleteUserCommand(f, "em_new") || len(LoadUserCommands(f)) != 2 {
		t.Error("delete")
	}
	for name, want := range map[string]bool{"em_x": true, "EM_y": true, "em_": false, "em_a b": false, "cm_x": false} {
		if ValidUserCommandName(name) != want {
			t.Errorf("ValidUserCommandName(%q) != %v", name, want)
		}
	}
}

func TestButtonClipboard(t *testing.T) {
	b := Button{Command: "cm_SrcThumbs", Params: "?%P", Icon: "wcmicons.dll,52", Tooltip: "Thumbnails", Path: "/tmp", Iconic: -1}
	got, err := ParseButtonClipboard(b.ClipboardText())
	if err != nil || got != b {
		t.Fatalf("round trip = %+v %v", got, err)
	}
	// As shared from TC: LF line ends, empty lines, no trailing fields.
	got, err = ParseButtonClipboard("TOTALCMD#BAR#DATA\ncm_List\n\nwcmicons.dll,5\nView\n")
	if err != nil || got.Command != "cm_List" || got.Icon != "wcmicons.dll,5" || got.Tooltip != "View" || got.Path != "" || got.Iconic != 0 {
		t.Errorf("TC text = %+v %v", got, err)
	}
	if _, err := ParseButtonClipboard("hello\nworld"); err == nil {
		t.Error("non-button text accepted")
	}
	if _, err := ParseButtonClipboard(""); err == nil {
		t.Error("empty text accepted")
	}
}
