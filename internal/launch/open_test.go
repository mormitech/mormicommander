// Tests for the commands that open files, folders, "Open with", reveal and print on each OS.
package launch

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestOpenDefaultFolderReveal(t *testing.T) {
	type cmds struct{ open, folder, reveal Command }
	tests := []struct {
		os   string
		path []string
		file string
		dir  string
		want cmds
	}{
		{"darwin", nil, "/a b/c.txt", "/a b", cmds{
			Command{Path: "open", Args: []string{"/a b/c.txt"}},
			Command{Path: "open", Args: []string{"/a b"}},
			Command{Path: "open", Args: []string{"-R", "/a b/c.txt"}},
		}},
		{"windows", nil, `C:\a b\c&d.txt`, `C:\a b`, cmds{
			Command{Path: "rundll32.exe", Params: `url.dll,FileProtocolHandler C:\a b\c&d.txt`},
			Command{Path: "explorer.exe", Params: `"C:\a b"`},
			Command{Path: "explorer.exe", Params: `/select,"C:\a b\c&d.txt"`},
		}},
		{"linux", []string{"dbus-send"}, "/a b/c.txt", "/a b", cmds{
			Command{Path: "xdg-open", Args: []string{"/a b/c.txt"}},
			Command{Path: "xdg-open", Args: []string{"/a b"}},
			Command{Path: "dbus-send", Args: []string{"--session", "--dest=org.freedesktop.FileManager1", "--type=method_call",
				"/org/freedesktop/FileManager1", "org.freedesktop.FileManager1.ShowItems", "array:string:file:///a%20b/c.txt", "string:"}},
		}},
		{"linux", nil, "/a/c.txt", "/a", cmds{
			Command{Path: "xdg-open", Args: []string{"/a/c.txt"}},
			Command{Path: "xdg-open", Args: []string{"/a"}},
			Command{Path: "xdg-open", Args: []string{"/a"}},
		}},
	}
	for _, tt := range tests {
		l, rec := fakeLauncher(t, tt.os, nil, tt.path...)
		if err := l.OpenDefault(tt.file); err != nil {
			t.Fatal(err)
		}
		sameCmd(t, rec.last(t), tt.want.open)
		_ = l.OpenFolder(tt.dir)
		sameCmd(t, rec.last(t), tt.want.folder)
		_ = l.Reveal(tt.file)
		if tt.os == "linux" && len(tt.path) == 0 {
			// Without D-Bus the parent folder is opened; filepath.Dir runs with this machine's rules.
			tt.want.reveal.Args = []string{filepath.Dir(tt.file)}
		}
		sameCmd(t, rec.last(t), tt.want.reveal)
	}
	l, rec := fakeLauncher(t, "linux", nil)
	_ = l.OpenDefault("-weird.txt")
	sameCmd(t, rec.last(t), Command{Path: "xdg-open", Args: []string{"./-weird.txt"}})
	_ = l.OpenDefault("https://example.com/?a=1&b=2")
	sameCmd(t, rec.last(t), Command{Path: "xdg-open", Args: []string{"https://example.com/?a=1&b=2"}})
}

func TestOpenWith(t *testing.T) {
	tests := []struct {
		os, file, app string
		want          Command
	}{
		{"darwin", "/x.txt", "TextEdit", Command{Path: "open", Args: []string{"-a", "TextEdit", "/x.txt"}}},
		{"darwin", "/x.txt", "/Applications/BBEdit.app", Command{Path: "open", Args: []string{"-a", "/Applications/BBEdit.app", "/x.txt"}}},
		{"darwin", "/x.txt", "com.apple.TextEdit", Command{Path: "open", Args: []string{"-b", "com.apple.TextEdit", "/x.txt"}}},
		{"windows", `C:\a b.txt`, `C:\Windows\notepad.exe`, Command{Path: `C:\Windows\notepad.exe`, Params: `"C:\a b.txt"`}},
		{"windows", `C:\a b.txt`, `"C:\Program Files\App\app.exe" "%1"`, Command{Path: `C:\Program Files\App\app.exe`, Params: `"C:\a b.txt"`}},
		{"windows", `C:\a b.txt`, `"C:\P\app.exe" /open %1 %%`, Command{Path: `C:\P\app.exe`, Params: `/open "C:\a b.txt" %`}},
		{"windows", `C:\a.txt`, `"C:\P\app.exe" -x`, Command{Path: `C:\P\app.exe`, Params: `-x C:\a.txt`}},
		{"linux", "/a b.txt", "gedit", Command{Path: "gedit", Args: []string{"/a b.txt"}}},
		{"linux", "/a b.txt", "code --new-window %F", Command{Path: "code", Args: []string{"--new-window", "/a b.txt"}}},
	}
	for _, tt := range tests {
		l, rec := fakeLauncher(t, tt.os, nil)
		if err := l.OpenWith(tt.file, tt.app); err != nil {
			t.Fatalf("%s OpenWith(%q): %v", tt.os, tt.app, err)
		}
		sameCmd(t, rec.last(t), tt.want)
	}
}

func TestOpenWithDesktopFile(t *testing.T) {
	data := t.TempDir()
	writeFile(t, filepath.Join(data, "applications", "org.ex.Edit.desktop"),
		"[Desktop Entry]\nType=Application\nName=Ex Edit\nExec=exedit --file=%f\n", 0o644)
	writeFile(t, filepath.Join(data, "applications", "vi.desktop"),
		"[Desktop Entry]\nType=Application\nName=Vi\nExec=vi %F\nTerminal=true\n", 0o644)
	l, rec := fakeLauncher(t, "linux", map[string]string{"XDG_DATA_HOME": data, "XDG_DATA_DIRS": "/nonexistent"}, "xterm")
	if err := l.OpenWith("/a b.txt", "org.ex.Edit.desktop"); err != nil {
		t.Fatal(err)
	}
	sameCmd(t, rec.last(t), Command{Path: "exedit", Args: []string{"--file=/a b.txt"}})
	if err := l.OpenWith("/w/f.txt", "vi.desktop"); err != nil {
		t.Fatal(err)
	}
	sameCmd(t, rec.last(t), Command{Path: "xterm", Args: []string{"-e", "vi", "/w/f.txt"}, Dir: "/w"})
	if err := l.OpenWith("/x", "missing.desktop"); err == nil {
		t.Error("unknown desktop id should fail")
	}
	// The same entry through the AppsFor shape.
	_ = l.OpenWithApp("/q.txt", App{Name: "Ex", Path: "/p/ex.desktop", Exec: "ex %U"})
	sameCmd(t, rec.last(t), Command{Path: "ex", Args: []string{"/q.txt"}})
}

func TestOpenWithAppAndChooserAndPrint(t *testing.T) {
	m, rec := fakeLauncher(t, "darwin", nil)
	_ = m.OpenWithApp("/f.pdf", App{Name: "Preview", Path: "/System/Applications/Preview.app"})
	sameCmd(t, rec.last(t), Command{Path: "open", Args: []string{"-a", "/System/Applications/Preview.app", "/f.pdf"}})
	if err := m.OpenWithChooser("/f.pdf"); !errors.Is(err, ErrUnsupported) {
		t.Errorf("chooser on macOS: %v", err)
	}
	_ = m.Print("/f.pdf")
	sameCmd(t, rec.last(t), Command{Path: "lp", Args: []string{"--", "/f.pdf"}})

	w, wrec := fakeLauncher(t, "windows", nil)
	_ = w.OpenWithApp(`C:\f.txt`, App{Path: `C:\npp.exe`, Exec: `"C:\npp.exe" "%1"`})
	sameCmd(t, wrec.last(t), Command{Path: `C:\npp.exe`, Params: `"C:\f.txt"`})
	_ = w.OpenWithChooser(`C:\f.txt`)
	sameCmd(t, wrec.last(t), Command{Path: "rundll32.exe", Params: `shell32.dll,OpenAs_RunDLL C:\f.txt`})
	_ = w.Print(`C:\a&b.txt`)
	c := wrec.last(t)
	if c.Show != ShowHidden || len(c.Env) != 1 || c.Env[0] != `MORMI_PRINT_FILE=C:\a&b.txt` {
		t.Errorf("print command = %#v", c)
	}
	if containsString(c.Params, `a&b`) {
		t.Error("the file name must not appear in the PowerShell script text")
	}
}

func containsString(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

func TestStartErrorRemovesCleanup(t *testing.T) {
	l, _ := fakeLauncher(t, "linux", nil)
	f := filepath.Join(t.TempDir(), "list.lst")
	writeFile(t, f, "x", 0o644)
	l.Start = func(Command) error { return errors.New("boom") }
	if err := l.start(Command{Path: "x", Cleanup: []string{f}}); err == nil {
		t.Fatal("expected the start error")
	}
	if fileExists(f) {
		t.Error("cleanup file left behind after a failed start")
	}
	var empty Launcher
	if err := empty.start(Command{Path: "x"}); err == nil {
		t.Error("a Launcher without Start must fail, not panic")
	}
}
