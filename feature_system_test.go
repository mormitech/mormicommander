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

// TestSystemExpandEnv: "cd $HOME" on the command line (board issue #130) asks Go for the variables' values.
func TestSystemExpandEnv(t *testing.T) {
	s := newSystemLaunch(NewApp(startOpts{}))
	s.newLauncher = func() *launch.Launcher {
		l := launch.New()
		l.LookupEnv = func(n string) (string, bool) {
			if n == "HOME" {
				return "/home/me", true
			}
			return "", false
		}
		return l
	}
	cases := map[string]string{"%HOME%/x": "/home/me/x", "plain": "plain", "%NOPE%": "%NOPE%"}
	if runtime.GOOS != "windows" {
		cases["$HOME"] = "/home/me"
		cases["${HOME}/Documents"] = "/home/me/Documents"
		cases["$NOPE/a"] = "$NOPE/a"
	}
	for in, want := range cases {
		if got := s.ExpandEnv(in); got != want {
			t.Errorf("ExpandEnv(%q) = %q, want %q", in, got, want)
		}
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
	a.settings.put("enableLog", true)
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

// TestSystemClipCutState checks the calls behind the dimmed cut files: CutFiles reports a pending cut only, and
// CancelCut empties the clipboard only while it still holds exactly that cut.
func TestSystemClipCutState(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.txt"), filepath.Join(dir, "b.txt")
	c := &SystemClip{app: NewApp(startOpts{}), clip: clipfiles.NewMemory()}

	if got, err := c.CutFiles(); err != nil || len(got) != 0 {
		t.Errorf("empty clipboard: %v %v", got, err)
	}
	if err := c.Copy([]string{a}, false); err != nil {
		t.Fatal(err)
	}
	if got, _ := c.CutFiles(); len(got) != 0 {
		t.Errorf("copied files are not a cut: %v", got)
	}
	if ok, _ := c.CancelCut([]string{a}); ok {
		t.Error("cancelling must leave copied files alone")
	}
	if ct, _ := c.Content(); !ct.Files {
		t.Error("copied files were taken off the clipboard")
	}

	if err := c.Copy([]string{a, b}, true); err != nil {
		t.Fatal(err)
	}
	if got, _ := c.CutFiles(); len(got) != 2 {
		t.Errorf("cut files: %v", got)
	}
	if ok, _ := c.CancelCut([]string{a}); ok {
		t.Error("a different list must not cancel the cut")
	}
	if ok, _ := c.CancelCut(nil); ok {
		t.Error("an empty list must not cancel the cut")
	}
	if ok, err := c.CancelCut([]string{b, a + string(filepath.Separator)}); !ok || err != nil {
		t.Errorf("the same cut, in another order: %v %v", ok, err)
	}
	if ct, _ := c.Content(); ct.Files || ct.Text || ct.Image {
		t.Errorf("the clipboard is not empty after cancelling: %+v", ct)
	}

	// something copied since the cut (text here) stays
	c.Copy([]string{a}, true)
	c.WriteText("hello")
	if ok, _ := c.CancelCut([]string{a}); ok {
		t.Error("text copied after the cut was cleared")
	}
	if ct, _ := c.Content(); !ct.Text {
		t.Error("the text is gone")
	}
}

func TestSystemSamePaths(t *testing.T) {
	cases := []struct {
		a, b []string
		want bool
	}{
		{[]string{"/a", "/b"}, []string{"/b", "/a"}, true},
		{[]string{"/a", "/a"}, []string{"/a"}, true},
		{[]string{"/a"}, []string{"/a", "/b"}, false},
		{[]string{"/a", "/b"}, []string{"/a"}, false},
		{nil, nil, true},
	}
	for _, tc := range cases {
		if got := systemSamePaths(tc.a, tc.b); got != tc.want {
			t.Errorf("%v vs %v: %v", tc.a, tc.b, got)
		}
	}
}
