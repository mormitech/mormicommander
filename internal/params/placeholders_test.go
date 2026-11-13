// Tests for parameter expansion: every placeholder, automatic quoting of
// names with spaces (and its interaction with user quotes and %Q), the
// %P%S repetition, list files and their encodings, the ? prompt, %[...]
// questions, %X/%Y/%Z panel switching and dropped files.
package params

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func winCtx(t *testing.T) Context {
	return Context{
		Style:   StyleWindows,
		TempDir: t.TempDir(),
		Env:     EnvOptions{Lookup: fakeEnv(map[string]string{"EDITOR": `C:\vim\vim.exe`})},
		Source: Panel{Path: `C:\Src Dir`, Cursor: Entry{Name: "my file.txt"},
			Selected: []Entry{{Name: "a.txt"}, {Name: "b c.txt"}, {Name: "sub", Dir: true}}},
		Target: Panel{Path: `D:\tgt`, Cursor: Entry{Name: "other.doc"}},
	}
}

func posixCtx(t *testing.T) Context {
	return Context{
		Style:   StylePOSIX,
		TempDir: t.TempDir(),
		Env:     EnvOptions{Lookup: fakeEnv(map[string]string{"HOME": "/home/p"})},
		Source:  Panel{Path: "/src", Cursor: Entry{Name: "it's here.txt"}},
		Target:  Panel{Path: "/tgt dir", Cursor: Entry{Name: ".."}},
	}
}

func TestExpandWindows(t *testing.T) {
	cases := []struct{ in, want string }{
		{"%P", `"C:\Src Dir\\"`},
		{"%T", `D:\tgt\`},
		{"%N", `"my file.txt"`},
		{"%M", `other.doc`},
		{"%O", `"my file"`},
		{"%E", `txt`},
		{"%O.zip", `"my file.zip"`},
		{"%P%N", `"C:\Src Dir\my file.txt"`},
		{`"%P%N"`, `"C:\Src Dir\my file.txt"`},
		{`"%P"`, `"C:\Src Dir\\"`},
		{"%T%M", `D:\tgt\other.doc`},
		{"%S", `a.txt "b c.txt" sub`},
		{"%P%S", `"C:\Src Dir\a.txt" "C:\Src Dir\b c.txt" "C:\Src Dir\sub"`},
		{"-x %S -y", `-x a.txt "b c.txt" sub -y`},
		{"%Q%P%N", `C:\Src Dir\my file.txt`},
		{"%N %Q", `my file.txt`},
		{"%R", `other.doc`},
		{"%T%R", `D:\tgt\other.doc`},
		{"-o %R", `-o other.doc`},
		{"100%%", `100%`},
		{"%EDITOR% %N", `C:\vim\vim.exe "my file.txt"`},
		{"%K stays", `%K stays`},
		{"trailing %", `trailing %`},
		{"a   b", `a b`},
		{"%p%n", `"C:\Src Dir\my file.txt"`},
		{"%S %S", `a.txt "b c.txt" sub a.txt "b c.txt" sub`},
		{"%S,%S", `"a.txt,a.txt b c.txt sub" "b c.txt,a.txt b c.txt sub" "sub,a.txt b c.txt sub"`},
	}
	for _, c := range cases {
		r, err := Expand(c.in, winCtx(t))
		if err != nil {
			t.Fatal(err)
		}
		if r.Params != c.want {
			t.Errorf("Expand(%q) = %s, want %s", c.in, r.Params, c.want)
		}
		if len(r.ListFiles) != 0 || r.Prompt {
			t.Errorf("Expand(%q): unexpected list/prompt", c.in)
		}
	}
}

func TestExpandPOSIX(t *testing.T) {
	cases := []struct{ in, want string }{
		{"%N", `'it'\''s here.txt'`},
		{"%P%N", `'/src/it'\''s here.txt'`},
		{`"%P%N"`, `"/src/it's here.txt"`},
		{`'%N'`, `'it'\''s here.txt'`},
		{"%T", `'/tgt dir/'`},
		{"%M", ``},
		{"%S", `'it'\''s here.txt'`},
		{"$HOME %N", `$HOME 'it'\''s here.txt'`},
		{`a\ b`, `a\ b`},
		{`x\ %N`, `x\ 'it'\''s here.txt'`},
	}
	for _, c := range cases {
		r, err := Expand(c.in, posixCtx(t))
		if err != nil {
			t.Fatal(err)
		}
		if r.Params != c.want {
			t.Errorf("Expand(%q) = %s, want %s", c.in, r.Params, c.want)
		}
	}
	// The expanded text must split back into the intended arguments.
	r, _ := Expand(`-a %P%N "%T" x\ y`, posixCtx(t))
	args, err := SplitArgs(r.Params, StylePOSIX)
	want := []string{"-a", "/src/it's here.txt", "/tgt dir/", "x y"}
	if err != nil || strings.Join(args, "|") != strings.Join(want, "|") {
		t.Errorf("split %s = %q %v", r.Params, args, err)
	}
	// Double quotes with $ and backslash in a name are escaped.
	ctx := posixCtx(t)
	ctx.Source.Cursor.Name = `a"$b\c`
	r, _ = Expand(`"%N"`, ctx)
	if args, _ := SplitArgs(r.Params, StylePOSIX); len(args) != 1 || args[0] != `a"$b\c` {
		t.Errorf("escaped double = %s -> %q", r.Params, args)
	}
}

func TestExpandWindowsSplit(t *testing.T) {
	r, _ := Expand(`"%P" %P%S`, winCtx(t))
	args, _ := SplitArgs(r.Params, StyleWindows)
	want := []string{`C:\Src Dir\`, `C:\Src Dir\a.txt`, `C:\Src Dir\b c.txt`, `C:\Src Dir\sub`}
	if strings.Join(args, "|") != strings.Join(want, "|") {
		t.Errorf("split %s = %q", r.Params, args)
	}
}

func TestExpandCursorFallbackAndDotDot(t *testing.T) {
	ctx := winCtx(t)
	ctx.Source.Selected = nil
	r, _ := Expand("%S", ctx)
	if r.Params != `"my file.txt"` {
		t.Errorf("fallback to cursor: %s", r.Params)
	}
	ctx.Source.Cursor = Entry{Name: ".."}
	r, _ = Expand("x %S %N %O %E y", ctx)
	if r.Params != "x y" {
		t.Errorf("nothing under cursor: %q", r.Params)
	}
	ctx.Source.Cursor = Entry{Name: "dir.v2", Dir: true}
	r, _ = Expand("%O|%E", ctx)
	if r.Params != "dir.v2|" {
		t.Errorf("folder not split: %q", r.Params)
	}
}

func readList(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestListFiles(t *testing.T) {
	ctx := winCtx(t)
	cases := []struct {
		ph   string
		want []byte
	}{
		{"%L", []byte("C:\\Src Dir\\a.txt\r\nC:\\Src Dir\\b c.txt\r\nC:\\Src Dir\\sub\\\r\n")},
		{"%l", []byte("C:\\Src Dir\\a.txt\r\nC:\\Src Dir\\b c.txt\r\nC:\\Src Dir\\sub\\\r\n")},
		{"%D", []byte("C:\\Src Dir\\a.txt\r\nC:\\Src Dir\\b c.txt\r\nC:\\Src Dir\\sub\\\r\n")},
		{"%F", []byte("a.txt\r\nb c.txt\r\nsub\\\r\n")},
		{"%f", []byte("a.txt\r\nb c.txt\r\nsub\\\r\n")},
		{"%d", []byte("a.txt\r\nb c.txt\r\nsub\\\r\n")},
		{"%UL", []byte("C:\\Src Dir\\a.txt\r\nC:\\Src Dir\\b c.txt\r\nC:\\Src Dir\\sub\\\r\n")},
		{"%UF", []byte("a.txt\r\nb c.txt\r\nsub\\\r\n")},
		{"%WF", EncodeList([]string{"a.txt", "b c.txt", `sub\`}, ListUTF16, true)},
		{"%WL", EncodeList([]string{`C:\Src Dir\a.txt`, `C:\Src Dir\b c.txt`, `C:\Src Dir\sub\`}, ListUTF16, true)},
	}
	for _, c := range cases {
		r, err := Expand("-@"+c.ph, ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(r.ListFiles) != 1 {
			t.Fatalf("%s: %d list files", c.ph, len(r.ListFiles))
		}
		f := r.ListFiles[0]
		if r.Params != Quote("-@"+f, StyleWindows) {
			t.Errorf("%s: params %q", c.ph, r.Params)
		}
		if got := readList(t, f); !bytes.Equal(got, c.want) {
			t.Errorf("%s: list = %q, want %q", c.ph, got, c.want)
		}
		if err := r.Cleanup(); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Errorf("%s: list file not deleted", c.ph)
		}
	}
}

func TestListFilePOSIXAndVirtual(t *testing.T) {
	ctx := posixCtx(t)
	ctx.Source = Panel{Path: "ftp://host/pub", Selected: []Entry{{Name: "x"}, {Name: "d", Dir: true}}}
	r, err := Expand("%L %F", ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Cleanup()
	if got := string(readList(t, r.ListFiles[0])); got != "ftp://host/pub/x\nftp://host/pub/d/\n" {
		t.Errorf("virtual list = %q", got)
	}
	if got := string(readList(t, r.ListFiles[1])); got != "x\nd/\n" {
		t.Errorf("names list = %q", got)
	}
	// Empty selection gives an empty list file, not an error.
	ctx.Source = Panel{Path: "/x", Cursor: Entry{Name: ".."}}
	r2, err := Expand("%L", ctx)
	if err != nil || len(readList(t, r2.ListFiles[0])) != 0 {
		t.Errorf("empty list: %v", err)
	}
	r2.Cleanup()
}

func TestListFileError(t *testing.T) {
	ctx := winCtx(t)
	ctx.TempDir = ctx.TempDir + "/does/not/exist"
	r, err := Expand("%F %L", ctx)
	if err == nil || r.Params != "" || len(r.ListFiles) != 0 {
		t.Errorf("want error and no files, got %v %+v", err, r)
	}
}

func TestPromptAndAsk(t *testing.T) {
	ctx := winCtx(t)
	r, _ := Expand("?%N", ctx)
	if !r.Prompt || r.Params != `"my file.txt"` {
		t.Errorf("prompt: %+v", r)
	}
	r, _ = Expand("-q %[Quality;80] %N", ctx)
	if r.Params != `-q 80 "my file.txt"` {
		t.Errorf("ask default: %q", r.Params)
	}
	var gotCap, gotDef string
	ctx.Ask = func(c, d string) (string, bool) { gotCap, gotDef = c, d; return "95", true }
	r, _ = Expand("-q %[Quality;80]", ctx)
	if r.Params != "-q 95" || gotCap != "Quality" || gotDef != "80" {
		t.Errorf("ask: %q %q %q", r.Params, gotCap, gotDef)
	}
	ctx.Ask = func(c, d string) (string, bool) { return "", false }
	r, err := Expand("%L %[Really?]", ctx)
	if err != nil || !r.Canceled || r.Params != "" || len(r.ListFiles) != 0 {
		t.Errorf("cancel: %+v %v", r, err)
	}
	r, _ = Expand("%[unterminated", winCtx(t))
	if r.Params != "%[unterminated" {
		t.Errorf("unterminated: %q", r.Params)
	}
}

func TestPanelSwitch(t *testing.T) {
	ctx := winCtx(t)
	ctx.SourceIsRight = true // source (C:) is right, target (D:) is left
	cases := []struct{ in, want string }{
		{"%X%P %T", `D:\tgt\ "C:\Src Dir\\"`},
		{"%Y%P %T", `"C:\Src Dir\\" D:\tgt\`},
		{"%X%N %Z%N", `other.doc "my file.txt"`},
		{"%Y%M", `other.doc`},
	}
	for _, c := range cases {
		r, _ := Expand(c.in, ctx)
		if r.Params != c.want {
			t.Errorf("Expand(%q) = %s, want %s", c.in, r.Params, c.want)
		}
	}
	ctx.SourceIsRight = false
	r, _ := Expand("%X%P", ctx)
	if r.Params != `"C:\Src Dir\\"` {
		t.Errorf("left is source: %s", r.Params)
	}
}

func TestDroppedFiles(t *testing.T) {
	ctx := winCtx(t)
	ctx.Dropped = []string{`C:\a b.txt`, `C:\c.txt`}
	r, _ := Expand("/x", ctx)
	if r.Params != `/x "C:\a b.txt" C:\c.txt` {
		t.Errorf("dropped: %s", r.Params)
	}
}

func TestExpandPath(t *testing.T) {
	ctx := winCtx(t)
	cases := []struct{ in, want string }{
		{"", ""},
		{"%P", `C:\Src Dir\`},
		{"%T sub dir", `D:\tgt\ sub dir`},
		{"%EDITOR%", `C:\vim\vim.exe`},
		{"%L", "%L"},
		{"%WL", "%WL"},
		{"%S", `a.txt b c.txt sub`},
	}
	for _, c := range cases {
		got, err := ExpandPath(c.in, ctx)
		if err != nil || got != c.want {
			t.Errorf("ExpandPath(%q) = %q, %v; want %q", c.in, got, err, c.want)
		}
	}
}

func TestHasQ(t *testing.T) {
	cases := map[string]bool{"%Q": true, "%q%N": true, "%%Q": false, "Q": false, "%%%Q": true, "%": false}
	for in, want := range cases {
		if hasQ(in) != want {
			t.Errorf("hasQ(%q) != %v", in, want)
		}
	}
}
