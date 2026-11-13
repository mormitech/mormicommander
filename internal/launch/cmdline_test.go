// Tests for the command line: parsing typed lines, environment variables, history, completion and inserting names.
package launch

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestParseLine(t *testing.T) {
	env := map[string]string{"HOME": "/home/u", "PROJ": "/srv/proj", "USERPROFILE": `C:\Users\u`}
	tests := []struct {
		os, line, cwd string
		want          ParsedLine
	}{
		{"linux", "   ", "/x", ParsedLine{Kind: LineEmpty}},
		{"linux", "cd ..", "/a/b", ParsedLine{Kind: LineCd, Text: "cd ..", Target: "/a"}},
		{"linux", "cd", "/a/b", ParsedLine{Kind: LineCd, Text: "cd", Target: "/home/u"}},
		{"linux", "cd ~/docs", "/a", ParsedLine{Kind: LineCd, Text: "cd ~/docs", Target: "/home/u/docs"}},
		{"linux", "cd -", "/a", ParsedLine{Kind: LineCd, Text: "cd -", Back: true}},
		{"linux", `cd "my dir"`, "/a", ParsedLine{Kind: LineCd, Text: `cd "my dir"`, Target: "/a/my dir"}},
		{"linux", "cd $PROJ/src", "/a", ParsedLine{Kind: LineCd, Text: "cd /srv/proj/src", Target: "/srv/proj/src"}},
		{"linux", "CD %PROJ%", "/a", ParsedLine{Kind: LineCd, Text: "CD /srv/proj", Target: "/srv/proj"}},
		{"linux", "cm_Copy /x", "/a", ParsedLine{Kind: LineInternal, Text: "cm_Copy /x", Command: "cm_Copy", Args: "/x"}},
		{"linux", "em_mine", "/a", ParsedLine{Kind: LineInternal, Text: "em_mine", Command: "em_mine"}},
		{"linux", "ls -l $HOME", "/a", ParsedLine{Kind: LineRun, Text: "ls -l /home/u"}},
		{"windows", `cd ..`, `C:\a\b`, ParsedLine{Kind: LineCd, Text: "cd ..", Target: `C:\a`}},
		{"windows", `cd..`, `C:\a\b`, ParsedLine{Kind: LineCd, Text: "cd..", Target: `C:\a`}},
		{"windows", `cd\`, `C:\a\b`, ParsedLine{Kind: LineCd, Text: `cd\`, Target: `C:\`}},
		{"windows", `cd /d D:\x`, `C:\a`, ParsedLine{Kind: LineCd, Text: `cd /d D:\x`, Target: `D:\x`}},
		{"windows", `cd`, `C:\a`, ParsedLine{Kind: LineCd, Text: `cd`, Target: `C:\a`}},
		{"windows", `cd %USERPROFILE%`, `C:\a`, ParsedLine{Kind: LineCd, Text: `cd C:\Users\u`, Target: `C:\Users\u`}},
		{"windows", `e:`, `C:\a`, ParsedLine{Kind: LineCd, Text: `e:`, Target: `E:\`}},
		{"windows", `notepad $HOME`, `C:\a`, ParsedLine{Kind: LineRun, Text: `notepad $HOME`}},
	}
	for _, tt := range tests {
		l, _ := fakeLauncher(t, tt.os, env)
		if got := l.ParseLine(tt.line, tt.cwd); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s ParseLine(%q) = %+v, want %+v", tt.os, tt.line, got, tt.want)
		}
	}
}

func TestParseLineExistingFolder(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "file.txt"), "x", 0o644)
	l, _ := fakeLauncher(t, runtime.GOOS, nil)
	if got := l.ParseLine("sub", dir); got.Kind != LineCd || got.Target != filepath.Join(dir, "sub") {
		t.Errorf("typing a folder name: %+v", got)
	}
	if got := l.ParseLine("file.txt", dir); got.Kind != LineRun {
		t.Errorf("typing a file name: %+v", got)
	}
}

func TestExpandEnv(t *testing.T) {
	env := map[string]string{"A": "1", "LONG_NAME": "two", "ProgramFiles(x86)": `C:\PF`}
	tests := []struct{ os, in, want string }{
		{"linux", "%A%-$A-${A}-$LONG_NAME", "1-1-1-two"},
		{"linux", "%MISSING% $MISSING ${MISSING}", "%MISSING% $MISSING ${MISSING}"},
		{"linux", "100% sure $", "100% sure $"},
		{"linux", "$A1", "$A1"},
		{"windows", "%A% $A %ProgramFiles(x86)%", `1 $A C:\PF`},
		{"windows", "50%", "50%"},
	}
	for _, tt := range tests {
		l, _ := fakeLauncher(t, tt.os, env)
		if got := l.ExpandEnv(tt.in); got != tt.want {
			t.Errorf("%s ExpandEnv(%q) = %q, want %q", tt.os, tt.in, got, tt.want)
		}
	}
}

func TestAppendName(t *testing.T) {
	l, _ := fakeLauncher(t, "windows", nil)
	line := l.AppendName("", "a.txt")
	line = l.AppendName(line, "b c.txt")
	if line != `a.txt "b c.txt"` {
		t.Errorf("windows Ctrl+Enter twice = %q", line)
	}
	if got := l.AppendName("copy ", "x"); got != "copy x" {
		t.Errorf("no double space: %q", got)
	}
	u, _ := fakeLauncher(t, "linux", nil)
	if got := u.AppendName("vim", "it's.txt"); got != `vim 'it'\''s.txt'` {
		t.Errorf("linux quoting: %q", got)
	}
}

func TestHistory(t *testing.T) {
	h := NewHistory(3)
	for _, s := range []string{"a", "b", " ", "a", "c", "d"} {
		h.Add(s)
	}
	if got := h.Items(); !reflect.DeepEqual(got, []string{"d", "c", "a"}) {
		t.Errorf("history = %q", got)
	}
	f := &INIFile{}
	h.Store(f)
	data := f.Bytes("\n")
	h2 := NewHistory(10)
	h2.Load(ParseINI(data))
	if got := h2.Items(); !reflect.DeepEqual(got, []string{"d", "c", "a"}) {
		t.Errorf("reloaded history = %q from %s", got, data)
	}
	h2.Clear()
	if len(h2.Items()) != 0 {
		t.Error("Clear left items")
	}
	if NewHistory(0).Max != 50 {
		t.Error("default history size should be 50")
	}
}

func TestCompleteLine(t *testing.T) {
	dir := t.TempDir()
	for _, d := range []string{"Docs", "docker", "My Folder"} {
		if err := os.Mkdir(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(dir, "doc.txt"), "", 0o644)
	writeFile(t, filepath.Join(dir, ".dotfile"), "", 0o644)
	writeFile(t, filepath.Join(dir, "Docs", "inner.md"), "", 0o644)

	l, _ := fakeLauncher(t, "linux", nil)
	got := l.CompleteLine("vim doc", dir)
	if want := []string{"vim docker/", "vim doc.txt"}; !reflect.DeepEqual(got, want) {
		t.Errorf("linux completion = %q, want %q", got, want)
	}
	got = l.CompleteLine("cd My", dir)
	if want := []string{`cd My\ Folder/`}; !reflect.DeepEqual(got, want) {
		t.Errorf("escaped completion = %q", got)
	}
	got = l.CompleteLine("cat Docs/in", dir)
	if want := []string{"cat Docs/inner.md"}; !reflect.DeepEqual(got, want) {
		t.Errorf("nested completion = %q", got)
	}
	if got := l.CompleteLine("ls ", dir); len(got) != 4 {
		t.Errorf("empty prefix should list the 4 visible entries, got %q", got)
	}
	m, _ := fakeLauncher(t, "darwin", nil)
	got = m.CompleteLine("open doc", dir)
	if want := []string{"open docker/", "open Docs/", "open doc.txt"}; !reflect.DeepEqual(got, want) {
		t.Errorf("case-insensitive completion = %q, want %q", got, want)
	}
	if got := l.CompleteLine("x nothing-here/", dir); got != nil {
		t.Errorf("missing folder gave %q", got)
	}
}
