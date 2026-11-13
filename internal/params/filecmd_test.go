// Tests for editor/viewer/compare-tool templates: %1 and %2 substitution
// with quoting, user quotes and %Q, appending the file when the template
// names none, keeping other placeholders and the "?" prompt working, and
// SplitFileTarget's file-versus-folder decision on real temp files.
package params

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandFileCommand(t *testing.T) {
	cases := []struct {
		name     string
		st       Style
		template string
		files    []string
		want     string
		prompt   bool
	}{
		{"win empty appends", StyleWindows, "", []string{`C:\a b\x.txt`}, `"C:\a b\x.txt"`, false},
		{"win flags then append", StyleWindows, "-n", []string{`C:\x.txt`}, `-n C:\x.txt`, false},
		{"win %1", StyleWindows, "/edit %1", []string{`C:\a b\x.txt`}, `/edit "C:\a b\x.txt"`, false},
		{"win user quoted %1", StyleWindows, `"%1"`, []string{`C:\a b\x.txt`}, `"C:\a b\x.txt"`, false},
		{"win compare %1 %2", StyleWindows, "%1 %2", []string{`C:\l.txt`, `D:\r r.txt`}, `C:\l.txt "D:\r r.txt"`, false},
		{"win missing %2", StyleWindows, "%1 %2", []string{`C:\l.txt`}, `C:\l.txt`, false},
		{"win %Q", StyleWindows, "%Q%1", []string{`C:\a b.txt`}, `C:\a b.txt`, false},
		{"win percent in name", StyleWindows, "%1", []string{`C:\100%.txt`}, `C:\100%.txt`, false},
		{"win %N suppresses append", StyleWindows, "%P%N", []string{`C:\ignored.txt`}, `C:\src\cur.txt`, false},
		{"win %L suppresses append", StyleWindows, "-list", nil, `-list`, false},
		{"prompt kept", StyleWindows, "?%1 -x", []string{`C:\f.txt`}, `C:\f.txt -x`, true},
		{"posix append", StylePOSIX, "-w", []string{"/tmp/a b"}, `-w '/tmp/a b'`, false},
		{"posix %1 apostrophe", StylePOSIX, "%1", []string{"/tmp/it's"}, `'/tmp/it'\''s'`, false},
		{"posix %1 with %O", StylePOSIX, "%1 --title %O", []string{"/tmp/x"}, `/tmp/x --title cur`, false},
		{"literal %%1 appends", StylePOSIX, "%%1", []string{"/f"}, `%1 /f`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx := Context{Style: c.st, TempDir: t.TempDir(), Env: EnvOptions{Lookup: fakeEnv(nil)},
				Source: Panel{Path: `C:\src`, Cursor: Entry{Name: "cur.txt"}}}
			r, err := ExpandFileCommand(c.template, c.files, ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer r.Cleanup()
			if r.Params != c.want {
				t.Errorf("got %q, want %q", r.Params, c.want)
			}
			if r.Prompt != c.prompt {
				t.Errorf("prompt = %v, want %v", r.Prompt, c.prompt)
			}
		})
	}
}

func TestExpandFileCommandListSuppressesAppend(t *testing.T) {
	ctx := Context{Style: StylePOSIX, TempDir: t.TempDir(), Env: EnvOptions{Lookup: fakeEnv(nil)},
		Source: Panel{Path: "/src", Cursor: Entry{Name: "a"}}}
	for _, tpl := range []string{"%L", "%UF", "%WL", "%S"} {
		r, err := ExpandFileCommand(tpl, []string{"/should/not/appear"}, ctx)
		if err != nil {
			t.Fatal(err)
		}
		if r.Params == "" || filepath.Base(r.Params) == "appear" {
			t.Errorf("%s: got %q", tpl, r.Params)
		}
		r.Cleanup()
	}
}

func TestSplitFileTarget(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct{ in, dir, cur string }{
		{file, dir, "f.txt"},
		{dir, dir, ""},
		{filepath.Join(dir, "missing"), filepath.Join(dir, "missing"), ""},
		{"ftp://host/f.txt", "ftp://host/f.txt", ""},
		{"", "", ""},
	}
	for _, c := range cases {
		d, cur := SplitFileTarget(c.in)
		if d != c.dir || cur != c.cur {
			t.Errorf("SplitFileTarget(%q) = %q, %q; want %q, %q", c.in, d, cur, c.dir, c.cur)
		}
	}
}

// TestExpandFileCommandQuotedNamesStayOneArg is a regression test: a file
// name containing quote characters, put inside the user's own quotes, must
// split back into exactly the original name and never add arguments.
func TestExpandFileCommandQuotedNamesStayOneArg(t *testing.T) {
	cases := []struct {
		name     string
		st       Style
		template string
		file     string
		want     []string
	}{
		{"posix dq injection", StylePOSIX, `"%1"`, `/x/a" --flag "b.txt`, []string{`/x/a" --flag "b.txt`}},
		{"posix dq dollar backtick", StylePOSIX, `"%1"`, "/x/$HOME`id`\\.txt", []string{"/x/$HOME`id`\\.txt"}},
		{"posix sq space", StylePOSIX, `'%1'`, `/x/a b.txt`, []string{`/x/a b.txt`}},
		{"posix sq quote", StylePOSIX, `'%1'`, `/x/it's.txt`, []string{`/x/it's.txt`}},
		{"posix dq with prefix", StylePOSIX, `-f "%1" -n`, `/x/a" "b`, []string{"-f", `/x/a" "b`, "-n"}},
		{"posix percent", StylePOSIX, `"%1"`, `/x/100%"y.txt`, []string{`/x/100%"y.txt`}},
		{"win dq trailing slash", StyleWindows, `"%1"`, `C:\a b\`, []string{`C:\a b\`}},
		{"win dq quote", StyleWindows, `"%1" x`, `C:\a" b`, []string{`C:\a" b`, "x"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res, err := ExpandFileCommand(c.template, []string{c.file}, Context{Style: c.st})
			if err != nil {
				t.Fatal(err)
			}
			got, err := SplitArgs(res.Params, c.st)
			if err != nil {
				t.Fatalf("SplitArgs(%q): %v", res.Params, err)
			}
			if len(got) != len(c.want) {
				t.Fatalf("Params %q split into %q, want %q", res.Params, got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("Params %q split into %q, want %q", res.Params, got, c.want)
				}
			}
		})
	}
}
