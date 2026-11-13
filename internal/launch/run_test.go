// Tests for running programs and command lines, including a few real (harmless, temp-folder) process starts.
package launch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRunCommandCrossOS(t *testing.T) {
	w, _ := fakeLauncher(t, "windows", map[string]string{"TC": `C:\TC`})
	c, err := w.RunCommand(RunSpec{Program: `%TC%\tool.exe`, Params: `/x "a b"`, Dir: `C:\w`, Show: ShowMinimized, Admin: true})
	if err != nil {
		t.Fatal(err)
	}
	sameCmd(t, c, Command{Path: `C:\TC\tool.exe`, Params: `/x "a b"`, Dir: `C:\w`, Show: ShowMinimized, Verb: "runas"})

	l, _ := fakeLauncher(t, "linux", nil)
	if _, err := l.RunCommand(RunSpec{Program: "x", Admin: true}); !errors.Is(err, ErrUnsupported) {
		t.Errorf("admin on linux: %v", err)
	}
	if _, err := l.RunCommand(RunSpec{}); err == nil {
		t.Error("empty program should fail")
	}
	c, _ = l.RunCommand(RunSpec{Program: "/usr/bin/gimp", Params: `"my pic.png"`, Dir: "/p", Show: ShowHidden})
	sameCmd(t, c, Command{Path: "/usr/bin/gimp", Args: []string{"my pic.png"}, Dir: "/p", Show: ShowHidden})

	// In a terminal: the program and its arguments are quoted for the shell.
	l2, _ := fakeLauncher(t, "linux", nil, "xterm")
	c, _ = l2.RunCommand(RunSpec{Program: "/bin/top", Params: `-d "1 2"`, Dir: "/p", Terminal: true})
	sameCmd(t, c, Command{Path: "xterm", Args: []string{"-e", "/bin/sh", "-c", "cd -- /p || exit 1\n/bin/top -d '1 2'\n"}, Dir: "/p"})

	m, _ := fakeLauncher(t, "darwin", nil)
	c, _ = m.RunCommand(RunSpec{Program: "/Applications/Foo.app", Params: "-x 1", Dir: "/p"})
	sameCmd(t, c, Command{Path: "open", Args: []string{"-a", "/Applications/Foo.app", "--args", "-x", "1"}, Dir: "/p"})
	c, _ = m.RunCommand(RunSpec{Program: "BBEdit", Params: "", Dir: "/p"})
	sameCmd(t, c, Command{Path: "open", Args: []string{"-a", "BBEdit"}, Dir: "/p"})
}

func TestRunCommandRealFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix permission bits")
	}
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "doc.txt"), "hello", 0o644)
	writeFile(t, filepath.Join(dir, "tool.sh"), "#!/bin/sh\necho hi\n", 0o755)
	l, _ := fakeLauncher(t, runtime.GOOS, nil)
	// A document typed as a program opens with its app.
	c, err := l.RunCommand(RunSpec{Program: "doc.txt", Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	want := l.OpenDefaultCommand(filepath.Join(dir, "doc.txt"))
	want.Dir = dir
	sameCmd(t, c, want)
	// A program in the start folder is found there, not in PATH.
	c, _ = l.RunCommand(RunSpec{Program: "tool.sh", Params: "a", Dir: dir})
	sameCmd(t, c, Command{Path: filepath.Join(dir, "tool.sh"), Args: []string{"a"}, Dir: dir})
}

func TestRunLineCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix permission bits")
	}
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "doc.txt"), "hello", 0o644)
	writeFile(t, filepath.Join(dir, "tool.sh"), "#!/bin/sh\necho hi\n", 0o755)
	l, _ := fakeLauncher(t, runtime.GOOS, nil, "xterm")

	// Shell syntax always goes to the shell.
	c, err := l.RunLineCommand("ls | wc -l", dir, false, LineDetached)
	if err != nil {
		t.Fatal(err)
	}
	sameCmd(t, c, Command{Path: "/bin/sh", Args: []string{"-c", "ls | wc -l"}, Dir: dir})

	// A document opens with its app.
	c, _ = l.RunLineCommand("doc.txt", dir, false, LineAuto)
	want := l.OpenDefaultCommand(filepath.Join(dir, "doc.txt"))
	want.Dir = dir
	sameCmd(t, c, want)

	// A script is a console program: detached mode runs it directly.
	c, _ = l.RunLineCommand(`./tool.sh "x y"`, dir, false, LineDetached)
	sameCmd(t, c, Command{Path: filepath.Join(dir, "tool.sh"), Args: []string{"x y"}, Dir: dir})

	// Auto mode and Shift+Enter put console programs in a terminal.
	for _, keep := range []bool{false, true} {
		c, _ = l.RunLineCommand("./tool.sh", dir, keep, LineAuto)
		if runtime.GOOS == "darwin" {
			if c.Path != "open" || c.Args[1] != "Terminal" {
				t.Errorf("keep=%v: %#v", keep, c)
			}
			os.Remove(c.Args[2])
		} else if c.Path != "xterm" {
			t.Errorf("keep=%v: %#v", keep, c)
		}
	}
	// An unknown word goes to the shell (it may be a built-in such as export).
	c, _ = l.RunLineCommand("no-such-program-xyz arg", dir, false, LineDetached)
	sameCmd(t, c, Command{Path: "/bin/sh", Args: []string{"-c", "no-such-program-xyz arg"}, Dir: dir})

	if _, err := l.RunLineCommand("  ", dir, false, LineAuto); err == nil {
		t.Error("empty line should fail")
	}
}

func TestRunLineWindowsShapes(t *testing.T) {
	w, _ := fakeLauncher(t, "windows", nil)
	// Not on Windows, nothing resolves: lines go to cmd.
	c, _ := w.RunLineCommand("dir /w", `C:\x`, false, LineAuto)
	sameCmd(t, c, Command{Path: "cmd.exe", Params: "/c dir /w", Dir: `C:\x`, NewConsole: true})
	c, _ = w.RunLineCommand("dir /w", `C:\x`, true, LineAuto)
	sameCmd(t, c, Command{Path: "cmd.exe", Params: "/k dir /w", Dir: `C:\x`, NewConsole: true})
	c, _ = w.RunLineCommand("dir", `C:\x`, false, LineDetached)
	sameCmd(t, c, Command{Path: "cmd.exe", Params: "/c dir", Dir: `C:\x`, Show: ShowHidden})
}

func TestHasShellSyntax(t *testing.T) {
	for in, want := range map[string]bool{
		"ls -l":           false,
		"ls | wc":         true,
		`echo "a|b"`:      false,
		`echo 'a > b'`:    false,
		"echo $HOME":      true,
		"a && b":          true,
		`echo \|`:         false,
		"rm *.txt":        true,
		`grep "x" file`:   false,
		"cd ~":            true,
		"python3 x.py #c": true,
	} {
		if got := hasShellSyntax(in); got != want {
			t.Errorf("hasShellSyntax(%q) = %v, want %v", in, got, want)
		}
	}
	if got := firstWordRaw(`"my prog" x`); got != `"my prog"` {
		t.Errorf("firstWordRaw = %q", got)
	}
	if got := firstWordRaw(`my\ prog x`); got != `my\ prog` {
		t.Errorf("firstWordRaw escaped = %q", got)
	}
}

func TestCaptureLineReal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "marker.txt"), "", 0o644)
	l := New()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := l.CaptureLine(ctx, "ls; echo err >&2", dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "marker.txt") || !strings.Contains(string(out), "err") {
		t.Errorf("captured output = %q", out)
	}
	if _, err := l.CaptureLine(ctx, "exit 3", dir); err == nil {
		t.Error("a failing command should report its error")
	}
	var empty Launcher
	if _, err := empty.CaptureLine(ctx, "true", dir); err == nil {
		t.Error("a Launcher without Output must fail, not panic")
	}
}

func TestStartProcessRealCleansUp(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	dir := t.TempDir()
	list := filepath.Join(dir, "list.lst")
	writeFile(t, list, "x", 0o644)
	out := filepath.Join(dir, "ran.txt")
	err := startProcess(Command{Path: "/bin/sh", Args: []string{"-c", "pwd > ran.txt"}, Dir: dir, Cleanup: []string{list}})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && (fileExists(list) || !fileExists(out)) {
		time.Sleep(20 * time.Millisecond)
	}
	if fileExists(list) {
		t.Error("list file was not removed after the program quit")
	}
	if !fileExists(out) {
		t.Error("the program did not run in its start folder")
	}
	if err := startProcess(Command{Path: filepath.Join(dir, "missing-program")}); err == nil {
		t.Error("starting a missing program should fail")
	}
}
