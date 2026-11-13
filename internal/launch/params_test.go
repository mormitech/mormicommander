// Tests for Total Commander parameter placeholders.
package launch

import (
	"bytes"
	"os"
	"reflect"
	"testing"
	"unicode/utf16"
)

func TestExpandParamsUnix(t *testing.T) {
	l, _ := fakeLauncher(t, "linux", map[string]string{"EDITOR_DIR": "/opt/ed"})
	ctx := ParamContext{
		Source: Panel{Dir: "/home/u/src", Current: "my file.tar.gz", Selected: []string{"a.txt", "b c.txt"}},
		Target: Panel{Dir: "/tmp/dst", Current: "t.txt", Selected: []string{"x"}},
	}
	tests := []struct{ in, want string }{
		{"%P", `/home/u/src/`},
		{"%T", `/tmp/dst/`},
		{"%N", `my\ file.tar.gz`},
		{`"%P%N"`, `"/home/u/src/my\ file.tar.gz"`},
		{"%O", `my\ file.tar`},
		{"%E", "gz"},
		{"%M", "t.txt"},
		{"%S", `a.txt b\ c.txt`},
		{"%P%S", `/home/u/src/a.txt /home/u/src/b\ c.txt`},
		{"%T%R", `/tmp/dst/x`},
		{"%R", "x"},
		{"100%%", "100%"},
		{"%EDITOR_DIR%/x", "/opt/ed/x"},
		{"%NOPE%", `my\ file.tar.gzOPE%`},
		{"%Q stays", "%Q stays"},
		{"%n", `my\ file.tar.gz`},
		{"%1", `/home/u/src/my\ file.tar.gz`},
		{"trailing %", "trailing %"},
	}
	for _, tt := range tests {
		got, err := l.ExpandParams(tt.in, ctx)
		if err != nil {
			t.Fatalf("ExpandParams(%q): %v", tt.in, err)
		}
		if got.Text != tt.want {
			t.Errorf("ExpandParams(%q) = %q, want %q", tt.in, got.Text, tt.want)
		}
	}
	// What a program would receive: every inserted name stays one argument.
	got, _ := l.ExpandParams(`-x "%P%N" %S`, ctx)
	args, err := SplitArgs(got.Text)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-x", "/home/u/src/my file.tar.gz", "a.txt", "b c.txt"}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("args = %q, want %q", args, want)
	}
}

// Regression: placeholders inside single quotes ('%N', '%P%N') used to get
// backslash escapes, which SplitArgs keeps literally there, and a name with an
// apostrophe left the quote unterminated.
func TestExpandParamsSingleQuotes(t *testing.T) {
	l, _ := fakeLauncher(t, "linux", nil)
	tests := []struct {
		current, params string
		want            []string
	}{
		{"a b.txt", `'%N'`, []string{"a b.txt"}},
		{"it's.txt", `'%N'`, []string{"it's.txt"}},
		{"it's a.txt", `-o '%P%N' x`, []string{"-o", "/d/it's a.txt", "x"}},
		{"plain", `'%N'`, []string{"plain"}},
		{`back\slash "q".txt`, `'%N'`, []string{`back\slash "q".txt`}},
		{"a b.txt", `"%N" '%N' %N`, []string{"a b.txt", "a b.txt", "a b.txt"}},
		{"a b.txt", `'it''s' %N`, []string{"its", "a b.txt"}},
		{"a b.txt", `"it's" %N`, []string{"it's", "a b.txt"}},
		{"a b.txt", `\' %N`, []string{"'", "a b.txt"}},
	}
	for _, tt := range tests {
		ctx := ParamContext{Source: Panel{Dir: "/d", Current: tt.current}}
		got, err := l.ExpandParams(tt.params, ctx)
		if err != nil {
			t.Fatal(err)
		}
		args, err := SplitArgs(got.Text)
		if err != nil {
			t.Fatalf("%q with %q -> %q: %v", tt.params, tt.current, got.Text, err)
		}
		if !reflect.DeepEqual(args, tt.want) {
			t.Errorf("%q with %q -> %q = %q, want %q", tt.params, tt.current, got.Text, args, tt.want)
		}
	}
}

func TestExpandParamsWindows(t *testing.T) {
	l, _ := fakeLauncher(t, "windows", map[string]string{"COMMANDER_PATH": `C:\TC`})
	ctx := ParamContext{
		Source: Panel{Dir: `C:\src`, Current: "my file.txt", Selected: []string{"a.txt", "b c.txt"}},
		Target: Panel{Dir: `D:\`},
	}
	tests := []struct{ in, want string }{
		{`"%P%N"`, `"C:\src\my file.txt"`},
		{"%T", `D:\`},
		{"%S", `a.txt "b c.txt"`},
		{"%P%S", `C:\src\a.txt "C:\src\b c.txt"`},
		{`%COMMANDER_PATH%\x.ini`, `C:\TC\x.ini`},
		{`"%1"`, `"C:\src\my file.txt"`},
	}
	for _, tt := range tests {
		got, err := l.ExpandParams(tt.in, ctx)
		if err != nil {
			t.Fatal(err)
		}
		if got.Text != tt.want {
			t.Errorf("ExpandParams(%q) = %q, want %q", tt.in, got.Text, tt.want)
		}
	}
}

func TestExpandParamsSelectionFallbackAndPrompt(t *testing.T) {
	l, _ := fakeLauncher(t, "linux", nil)
	ctx := ParamContext{Source: Panel{Dir: "/d", Current: "cur.txt"}}
	got, _ := l.ExpandParams("?%S", ctx)
	if !got.Prompt || got.Text != "cur.txt" {
		t.Errorf("got %+v, want prompt and the file under the cursor", got)
	}
	got, _ = l.ExpandParams("%S", ParamContext{Source: Panel{Dir: "/d", Current: ".."}})
	if got.Text != "" {
		t.Errorf("cursor on .. should give no names, got %q", got.Text)
	}
	if got, _ := l.ExpandParams("-v", ctx); got.HasFile {
		t.Error("plain parameters must not claim a file")
	}
}

func TestExpandParamsListFiles(t *testing.T) {
	for _, osName := range []string{"linux", "windows"} {
		l, _ := fakeLauncher(t, osName, nil)
		dir := "/src"
		eol := "\n"
		if osName == "windows" {
			dir = `C:\src`
			eol = "\r\n"
		}
		sep := sepFor(osName)
		ctx := ParamContext{Source: Panel{Dir: dir, Current: "cur", Selected: []string{"a", "b b"}}}
		tests := []struct {
			in   string
			want string
			wide bool
		}{
			{"%L", dir + sep + "a" + eol + dir + sep + "b b" + eol, false},
			{"%F", "a" + eol + "b b" + eol, false},
			{"%UL", dir + sep + "a" + eol + dir + sep + "b b" + eol, false},
			{"%UF", "a" + eol + "b b" + eol, false},
			{"%Z", dir + sep + "a" + eol + dir + sep + "b b" + eol, false},
			{"%WF", "a" + eol + "b b" + eol, true},
		}
		for _, tt := range tests {
			got, err := l.ExpandParams(tt.in, ctx)
			if err != nil {
				t.Fatal(err)
			}
			if len(got.ListFiles) != 1 || !got.HasFile {
				t.Fatalf("%s %s: list files = %v", osName, tt.in, got.ListFiles)
			}
			path := got.ListFiles[0]
			if osName == "linux" && got.Text != EscapeArg(path) || osName == "windows" && got.Text != path {
				t.Errorf("%s: inserted %q, want the list file path %q", tt.in, got.Text, path)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			text := string(data)
			if tt.wide {
				if !bytes.HasPrefix(data, []byte{0xFF, 0xFE}) {
					t.Fatalf("%s: no UTF-16 byte order mark", tt.in)
				}
				u := make([]uint16, (len(data)-2)/2)
				for i := range u {
					u[i] = uint16(data[2+2*i]) | uint16(data[3+2*i])<<8
				}
				text = string(utf16.Decode(u))
			}
			if text != tt.want {
				t.Errorf("%s %s: list = %q, want %q", osName, tt.in, text, tt.want)
			}
		}
	}
	// %Y: nothing selected gives an empty list instead of the file under the cursor.
	l, _ := fakeLauncher(t, "linux", nil)
	got, _ := l.ExpandParams("%Y%L", ParamContext{Source: Panel{Dir: "/d", Current: "cur"}})
	data, _ := os.ReadFile(got.ListFiles[0])
	if len(data) != 0 {
		t.Errorf("%%Y list = %q, want empty", data)
	}
	got, _ = l.ExpandParams("%L", ParamContext{Source: Panel{Dir: "/d", Current: "cur"}})
	data, _ = os.ReadFile(got.ListFiles[0])
	if string(data) != "/d/cur\n" {
		t.Errorf("list without selection = %q, want the file under the cursor", data)
	}
}

func TestExpandParamsListFileError(t *testing.T) {
	l, _ := fakeLauncher(t, "linux", nil)
	l.TempDir = "/nonexistent/folder/for/sure"
	if _, err := l.ExpandParams("%L %F", ParamContext{Source: Panel{Dir: "/d", Current: "x"}}); err == nil {
		t.Error("expected an error when the list file cannot be written")
	}
}

func TestCommandFor(t *testing.T) {
	l, _ := fakeLauncher(t, "linux", map[string]string{"TOOLS": "/opt/tools"})
	c, err := l.CommandFor("%TOOLS%/run", `-a "b c" d\ e`, "/w")
	if err != nil {
		t.Fatal(err)
	}
	sameCmd(t, c, Command{Path: "/opt/tools/run", Args: []string{"-a", "b c", "d e"}, Dir: "/w"})
	if _, err := l.CommandFor("x", `"open`, ""); err == nil {
		t.Error("unterminated quote should fail")
	}
	w, _ := fakeLauncher(t, "windows", nil)
	c, _ = w.CommandFor(`C:\x.exe`, `/a "b c"`, `C:\w`)
	sameCmd(t, c, Command{Path: `C:\x.exe`, Params: `/a "b c"`, Dir: `C:\w`})
}
