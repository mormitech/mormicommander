package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"mormicommander/internal/clipfiles"
	"mormicommander/internal/launch"
)

// recordingLaunch is a SystemLaunch whose launcher records the commands instead of starting them.
func recordingLaunch(a *App) (*SystemLaunch, *[]launch.Command) {
	var got []launch.Command
	s := newSystemLaunch(a)
	s.newLauncher = func() *launch.Launcher {
		l := launch.New()
		l.Start = func(c launch.Command) error { got = append(got, c); return nil }
		return l
	}
	return s, &got
}

func TestSystemIsProgram(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("execute bits are a Unix thing")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "run.sh")
	os.WriteFile(script, []byte("#!/bin/sh\necho hi\n"), 0o755)
	text := filepath.Join(dir, "notes.txt") // a file on a FAT stick: execute bit, but no program
	os.WriteFile(text, []byte("hello"), 0o755)
	plain := filepath.Join(dir, "plain.sh")
	os.WriteFile(plain, []byte("#!/bin/sh\n"), 0o644)
	if !systemIsProgram(script) {
		t.Error("an executable script is a program")
	}
	if systemIsProgram(text) || systemIsProgram(plain) || systemIsProgram(filepath.Join(dir, "missing")) {
		t.Error("documents are no programs")
	}
}

func TestSystemOpenDocumentAndURLShortcut(t *testing.T) {
	dir := t.TempDir()
	doc := filepath.Join(dir, "a b.txt")
	os.WriteFile(doc, []byte("x"), 0o644)
	s, got := recordingLaunch(NewApp(startOpts{}))
	r, err := s.Open(doc, dir)
	if err != nil || r.Kind != "opened" || len(*got) != 1 {
		t.Fatalf("open document: %+v %v %v", r, err, *got)
	}
	if !strings.Contains((*got)[0].String(), "a b.txt") {
		t.Errorf("command %q does not name the file", (*got)[0].String())
	}

	url := filepath.Join(dir, "site.url")
	os.WriteFile(url, []byte("[InternetShortcut]\r\nURL=https://example.com/\r\n"), 0o644)
	*got = nil
	r, err = s.Open(url, dir)
	if err != nil || r.Kind != "opened" || len(*got) != 1 || !strings.Contains((*got)[0].String(), "https://example.com/") {
		t.Fatalf("url shortcut: %+v %v %v", r, err, *got)
	}
}

func TestSystemOpenFolderShortcut(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip(".desktop files are followed everywhere, but the paths here are Unix paths")
	}
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	os.Mkdir(sub, 0o755)
	f := filepath.Join(dir, "go.desktop")
	os.WriteFile(f, []byte("[Desktop Entry]\nType=Link\nName=Sub\nURL=file://"+sub+"\n"), 0o644)
	s, got := recordingLaunch(NewApp(startOpts{}))
	r, err := s.Open(f, dir)
	if err != nil {
		t.Fatal(err)
	}
	if r.Kind != "dir" || filepath.Clean(r.Dir) != sub && !strings.HasSuffix(r.Dir, "/sub") {
		t.Errorf("folder shortcut: %+v (commands %v)", r, *got)
	}
}

func TestSystemDemo(t *testing.T) {
	a := NewApp(startOpts{Demo: true})
	s, got := recordingLaunch(a)
	if _, err := s.Open("/x/a.txt", "/x"); err == nil {
		t.Error("the demo opens nothing")
	}
	if err := s.Terminal("/"); err == nil {
		t.Error("the demo opens no terminal")
	}
	if err := (&SystemClip{app: a}).Copy([]string{"/x"}, false); err == nil {
		t.Error("the demo puts nothing on the clipboard")
	}
	if len(*got) != 0 {
		t.Errorf("commands started in the demo: %v", *got)
	}
}

func TestSystemRunLineCd(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "sub"), 0o755)
	s, got := recordingLaunch(NewApp(startOpts{}))
	r, err := s.RunLine("cd sub", dir, false)
	if err != nil || r.Kind != "cd" || filepath.Base(r.Target) != "sub" {
		t.Errorf("cd: %+v %v", r, err)
	}
	r, err = s.RunLine("cm_Copy", dir, false)
	if err != nil || r.Kind != "internal" || r.Command != "cm_Copy" {
		t.Errorf("internal: %+v %v", r, err)
	}
	if len(*got) != 0 {
		t.Errorf("cd started something: %v", *got)
	}
}

func TestSystemTerminalNeedsLocalFolder(t *testing.T) {
	s, _ := recordingLaunch(NewApp(startOpts{}))
	if err := s.Terminal("ftp://host/dir"); err == nil || !strings.Contains(err.Error(), "on this computer") {
		t.Errorf("terminal on a server: %v", err)
	}
	if err := s.Terminal(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("terminal in a missing folder")
	}
}

func TestSystemTransfer(t *testing.T) {
	dir := t.TempDir()
	other := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	os.WriteFile(a, []byte("x"), 0o644)
	b := filepath.Join(other, "b.txt")
	os.WriteFile(b, []byte("y"), 0o644)

	p, err := clipfiles.NewPlan([]string{a, b}, clipfiles.Copy, dir)
	if err != nil {
		t.Fatal(err)
	}
	tr := systemTransfer(p)
	if len(tr.Sources) != 1 || tr.Sources[0] != b || len(tr.Same) != 1 || tr.Same[0].NewName != "a(2).txt" || tr.Move {
		t.Errorf("copy into own folder: %+v", tr)
	}
	p, _ = clipfiles.NewPlan([]string{a, b}, clipfiles.Cut, dir)
	tr = systemTransfer(p)
	if len(tr.Sources) != 1 || len(tr.Same) != 0 || tr.Skipped != 1 || !tr.Move {
		t.Errorf("move into own folder: %+v", tr)
	}

	c := &SystemClip{app: NewApp(startOpts{})}
	os.Mkdir(filepath.Join(dir, "inner"), 0o755)
	if _, err := c.PlanTransfer([]string{dir}, false, filepath.Join(dir, "inner")); err == nil || !strings.Contains(err.Error(), "into itself") {
		t.Errorf("folder into itself: %v", err)
	}
}

func TestSystemEjectCommand(t *testing.T) {
	name, args, env := systemEjectCommand("darwin", "/Volumes/USB stick")
	if name != "diskutil" || len(args) != 2 || args[1] != "/Volumes/USB stick" || env != nil {
		t.Errorf("darwin: %s %v %v", name, args, env)
	}
	name, args, _ = systemEjectCommand("linux", "/media/me/stick")
	if name != "gio" || args[len(args)-1] != "/media/me/stick" {
		t.Errorf("linux: %s %v", name, args)
	}
	name, args, env = systemEjectCommand("windows", `E:\`)
	if name != "powershell" || strings.Contains(strings.Join(args, " "), `E:\`) || len(env) != 1 || env[0] != `MC_EJECT=E:\` {
		t.Errorf("windows: the path must travel in the environment: %s %v %v", name, args, env)
	}
	if name, _, _ := systemEjectCommand("plan9", "/x"); name != "" {
		t.Error("unknown system")
	}
}

func TestSystemLogFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MORMICOMMANDER_CONFIG_DIR", dir)
	a := NewApp(startOpts{})
	a.settings = loadSettings(filepath.Join(dir, "settings.json"))
	l := &SystemLog{app: a}
	if err := l.Write([]string{"nothing"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "mormicommander.log")); err == nil {
		t.Error("the log is off by default")
	}
	a.settings.Set("enableLog", true)
	if err := l.Write([]string{"Error: Can't copy a.txt", "two\nlines"}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "mormicommander.log"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 2 || !strings.HasSuffix(lines[0], ": Error: Can't copy a.txt") || !strings.HasSuffix(lines[1], "two lines") {
		t.Errorf("log file: %q", b)
	}
	if l.Path() != filepath.Join(dir, "mormicommander.log") {
		t.Errorf("path %s", l.Path())
	}
}

func TestSystemWatchable(t *testing.T) {
	if systemWatchable("ftp://host/x") || systemWatchable("") || systemWatchable("rel/dir") {
		t.Error("only local absolute folders are watched")
	}
	if !systemWatchable(t.TempDir()) {
		t.Error("a temp folder is watched")
	}
}
