// Tests for the command-line box: every cd spelling in both styles,
// filters, drive switching, cm_/em_ commands, "cmd", shell detection,
// elevation, Shift+Enter and environment expansion.
package params

import (
	"reflect"
	"testing"
)

func TestParseLineCD(t *testing.T) {
	envW := &EnvOptions{Percent: true, Lookup: fakeEnv(map[string]string{"WINDIR": `C:\Windows`})}
	envP := &EnvOptions{Percent: true, Dollar: true, Lookup: fakeEnv(map[string]string{"HOME": "/home/p", "D": "docs"})}
	w := LineContext{Cwd: `C:\Users\p\Docs`, Home: `C:\Users\p`, Style: StyleWindows, Env: envW}
	p := LineContext{Cwd: "/home/p/src", Home: "/home/p", Style: StylePOSIX, Env: envP}
	cases := []struct {
		line string
		ctx  LineContext
		want Action
	}{
		{"cd ..", w, Action{Kind: ActChangeDir, Path: `C:\Users\p`}},
		{"cd..", w, Action{Kind: ActChangeDir, Path: `C:\Users\p`}},
		{"CD ..", w, Action{Kind: ActChangeDir, Path: `C:\Users\p`}},
		{`cd \`, w, Action{Kind: ActChangeDir, Path: `C:\`}},
		{`cd\`, w, Action{Kind: ActChangeDir, Path: `C:\`}},
		{`cd /`, w, Action{Kind: ActChangeDir, Path: `C:\`}},
		{`cd \Temp`, w, Action{Kind: ActChangeDir, Path: `C:\Temp`}},
		{`cd D:\x`, w, Action{Kind: ActChangeDir, Path: `D:\x`}},
		{`cd /d D:\x`, w, Action{Kind: ActChangeDir, Path: `D:\x`}},
		{`cd d:`, w, Action{Kind: ActChangeDir, Path: `D:\`}},
		{`d:`, w, Action{Kind: ActChangeDir, Path: `D:\`}},
		{`cd Program Files`, w, Action{Kind: ActChangeDir, Path: `C:\Users\p\Docs\Program Files`}},
		{`cd "My Music"`, w, Action{Kind: ActChangeDir, Path: `C:\Users\p\Docs\My Music`}},
		{`cd %WINDIR%\System32`, w, Action{Kind: ActChangeDir, Path: `C:\Windows\System32`}},
		{`cd`, w, Action{Kind: ActNone}},
		{`cd ~`, w, Action{Kind: ActChangeDir, Path: `C:\Users\p`}},
		{`cd -`, w, Action{Kind: ActChangeDir, Back: true}},
		{`cd ...`, w, Action{Kind: ActChangeDir, Path: `C:\Users`}},
		{`cd *.txt`, w, Action{Kind: ActChangeDir, Filter: "*.txt"}},
		{`cd *.*`, w, Action{Kind: ActChangeDir, ClearFilter: true}},
		{`cd C:\x\*.doc`, w, Action{Kind: ActChangeDir, Path: `C:\x`, Filter: "*.doc"}},
		{`cd \\srv\share\x`, w, Action{Kind: ActChangeDir, Path: `\\srv\share\x`}},
		{`cd ftp://host/pub/../x`, w, Action{Kind: ActChangeDir, Path: `ftp://host/x`}},

		{"cd ..", p, Action{Kind: ActChangeDir, Path: "/home/p"}},
		{"cd", p, Action{Kind: ActChangeDir, Path: "/home/p"}},
		{"cd ~/Music", p, Action{Kind: ActChangeDir, Path: "/home/p/Music"}},
		{"cd $HOME/$D", p, Action{Kind: ActChangeDir, Path: "/home/p/docs"}},
		{"cd /", p, Action{Kind: ActChangeDir, Path: "/"}},
		{`cd \`, p, Action{Kind: ActChangeDir, Path: "/"}},
		{`cd my\ dir`, p, Action{Kind: ActChangeDir, Path: "/home/p/src/my dir"}},
		{`cd 'a b'`, p, Action{Kind: ActChangeDir, Path: "/home/p/src/a b"}},
		{`cd *.go`, p, Action{Kind: ActChangeDir, Filter: "*.go"}},
		{`cd /tmp/*`, p, Action{Kind: ActChangeDir, Path: "/tmp", ClearFilter: true}},
		{`cd ....`, p, Action{Kind: ActChangeDir, Path: "/"}},
		{`cd /a/.../b`, p, Action{Kind: ActChangeDir, Path: "/b"}},
	}
	for _, c := range cases {
		got, err := ParseLine(c.line, c.ctx, false)
		if err != nil {
			t.Fatalf("%q: %v", c.line, err)
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("ParseLine(%q) = %+v, want %+v", c.line, got, c.want)
		}
	}
}

func TestParseLineCDVirtualCwd(t *testing.T) {
	ctx := LineContext{Cwd: "sftp://h/home/x", Style: StyleWindows}
	got, _ := ParseLine("cd ..", ctx, false)
	if got.Path != "sftp://h/home" {
		t.Errorf("virtual parent: %q", got.Path)
	}
	got, _ = ParseLine(`cd \`, ctx, false)
	if got.Path != "sftp://h/" {
		t.Errorf("virtual root: %q", got.Path)
	}
}

func TestParseLineRun(t *testing.T) {
	envP := &EnvOptions{Percent: true, Dollar: true, Lookup: fakeEnv(map[string]string{"HOME": "/home/p"})}
	envW := &EnvOptions{Percent: true, Lookup: fakeEnv(map[string]string{"TEMP": `C:\Tmp`})}
	p := LineContext{Cwd: "/src", Home: "/home/p", Style: StylePOSIX, Env: envP}
	w := LineContext{Cwd: `C:\src`, Style: StyleWindows, Env: envW}
	cases := []struct {
		line string
		ctx  LineContext
		keep bool
		want Action
	}{
		{"", p, false, Action{Kind: ActNone}},
		{"   ", w, false, Action{Kind: ActNone}},
		{"cmd", p, false, Action{Kind: ActOpenTerminal, Dir: "/src"}},
		{"CMD", w, false, Action{Kind: ActOpenTerminal, Dir: `C:\src`}},
		{"cm_Copy", p, false, Action{Kind: ActInternal, Command: "cm_Copy", Line: "cm_Copy"}},
		{"em_pack x", w, false, Action{Kind: ActInternal, Command: "em_pack", Args: []string{"x"}, Line: "em_pack x"}},
		{"ls -l 'a b'", p, false, Action{Kind: ActRun, Command: "ls", Args: []string{"-l", "a b"}, Line: "ls -l 'a b'", Dir: "/src"}},
		{"echo $HOME", p, true, Action{Kind: ActRun, Command: "echo", Args: []string{"/home/p"}, Line: "echo /home/p", Dir: "/src", KeepOpen: true}},
		{"ls *.go", p, false, Action{Kind: ActRun, Command: "ls", Line: "ls *.go", Shell: true, Dir: "/src"}},
		{"cd x && make", p, false, Action{Kind: ActRun, Command: "cd", Line: "cd x && make", Shell: true, Dir: "/src"}},
		{"echo $HOME | wc", p, false, Action{Kind: ActRun, Command: "echo", Line: "echo $HOME | wc", Shell: true, Dir: "/src"}},
		{"echo '|'", p, false, Action{Kind: ActRun, Command: "echo", Args: []string{"|"}, Line: "echo '|'", Dir: "/src"}},
		{`echo "$(date)"`, p, false, Action{Kind: ActRun, Command: "echo", Line: `echo "$(date)"`, Shell: true, Dir: "/src"}},
		{`notepad "my file.txt"`, w, false, Action{Kind: ActRun, Command: "notepad", Args: []string{"my file.txt"}, Line: `notepad "my file.txt"`, Dir: `C:\src`}},
		{`dir %TEMP%`, w, false, Action{Kind: ActRun, Command: "dir", Args: []string{`C:\Tmp`}, Line: `dir C:\Tmp`, Dir: `C:\src`}},
		{`dir > out.txt`, w, false, Action{Kind: ActRun, Command: "dir", Line: `dir > out.txt`, Shell: true, Dir: `C:\src`}},
		{`dir *.txt`, w, false, Action{Kind: ActRun, Command: "dir", Args: []string{"*.txt"}, Line: `dir *.txt`, Dir: `C:\src`}},
		{`*regedit`, w, false, Action{Kind: ActRun, Command: "regedit", Line: `regedit`, Elevate: true, Dir: `C:\src`}},
		{`echo a^&b`, w, false, Action{Kind: ActRun, Command: "echo", Line: `echo a^&b`, Shell: true, Dir: `C:\src`}},
		{`cdrom`, w, false, Action{Kind: ActRun, Command: "cdrom", Line: `cdrom`, Dir: `C:\src`}},
	}
	for _, c := range cases {
		got, err := ParseLine(c.line, c.ctx, c.keep)
		if err != nil {
			t.Fatalf("%q: %v", c.line, err)
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("ParseLine(%q) =\n %+v, want\n %+v", c.line, got, c.want)
		}
	}
	if _, err := ParseLine("echo 'open", p, false); err == nil {
		t.Error("unterminated quote should fail")
	}
}

func TestParseLineDefaultEnv(t *testing.T) {
	t.Setenv("MC_TEST_VAR", "val")
	got, err := ParseLine("echo %MC_TEST_VAR%", LineContext{Cwd: "/", Style: StylePOSIX}, false)
	if err != nil || len(got.Args) != 1 || got.Args[0] != "val" {
		t.Errorf("default env: %+v %v", got, err)
	}
}

func TestExpandDotsAndSplitLast(t *testing.T) {
	if got := expandDots(`...\x`); got != `..\..\x` {
		t.Error(got)
	}
	if got := expandDots("a..b"); got != "a..b" {
		t.Error(got)
	}
	cases := []struct{ in, dir, last string }{
		{"a/b", "a", "b"}, {"/b", "/", "b"}, {`C:\b`, `C:\`, "b"}, {"b", "", "b"},
	}
	for _, c := range cases {
		d, l := splitLast(c.in)
		if d != c.dir || l != c.last {
			t.Errorf("splitLast(%q) = %q %q", c.in, d, l)
		}
	}
}
