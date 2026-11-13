// Tests for the external viewer and editor, Shift+F4 file creation and watching an edited file.
package launch

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseTool(t *testing.T) {
	tests := []struct {
		in   string
		want Tool
	}{
		{"", Tool{}},
		{"notepad.exe", Tool{Program: "notepad.exe"}},
		{`"C:\Program Files\Notepad++\notepad++.exe" -n1 "%1"`, Tool{Program: `C:\Program Files\Notepad++\notepad++.exe`, Params: `-n1 "%1"`}},
		{"code --wait %1", Tool{Program: "code", Params: "--wait %1"}},
	}
	for _, tt := range tests {
		got := ParseTool(tt.in)
		if got != tt.want {
			t.Errorf("ParseTool(%q) = %+v, want %+v", tt.in, got, tt.want)
		}
		if back := ParseTool(got.String()); back != got {
			t.Errorf("String/ParseTool round trip of %+v gave %+v", got, back)
		}
	}
}

func TestToolCommand(t *testing.T) {
	l, _ := fakeLauncher(t, "linux", nil)
	ctx := ParamContext{Source: Panel{Dir: "/src", Current: "a b.txt"}}
	c, err := l.ToolCommand(Tool{Program: "/usr/bin/code", Params: "--wait"}, "/src/a b.txt", ctx)
	if err != nil {
		t.Fatal(err)
	}
	sameCmd(t, c, Command{Path: "/usr/bin/code", Args: []string{"--wait", "/src/a b.txt"}, Dir: "/src"})
	c, _ = l.ToolCommand(Tool{Program: "/usr/bin/x", Params: `-g "%1":1`}, "/src/a b.txt", ctx)
	sameCmd(t, c, Command{Path: "/usr/bin/x", Args: []string{"-g", "/src/a b.txt:1"}, Dir: "/src"})
	c, _ = l.ToolCommand(Tool{Program: "/usr/bin/x"}, "/other/f.txt", ParamContext{})
	sameCmd(t, c, Command{Path: "/usr/bin/x", Args: []string{"/other/f.txt"}, Dir: filepath.Dir("/other/f.txt")})

	w, _ := fakeLauncher(t, "windows", nil)
	c, _ = w.ToolCommand(Tool{Program: `C:\npp.exe`}, `C:\a b.txt`, ParamContext{Source: Panel{Dir: `C:\`}})
	sameCmd(t, c, Command{Path: `C:\npp.exe`, Params: `"C:\a b.txt"`, Dir: `C:\`})
}

func TestDefaultEditor(t *testing.T) {
	m, rec := fakeLauncher(t, "darwin", nil)
	if err := m.OpenInTool(Tool{}, "/x.txt", ParamContext{}); err != nil {
		t.Fatal(err)
	}
	sameCmd(t, rec.last(t), Command{Path: "open", Args: []string{"-t", "/x.txt"}})
	w, _ := fakeLauncher(t, "windows", nil)
	c, _ := w.DefaultEditorCommand(`C:\a b.txt`)
	sameCmd(t, c, Command{Path: "notepad.exe", Params: `"C:\a b.txt"`})
	// Linux pretended on another OS: no desktop database to ask, so xdg-open.
	if l, _ := fakeLauncher(t, "linux", nil); true {
		c, _ := l.DefaultEditorCommand("/a.txt")
		if l.OS != "linux" || c.Path == "" {
			t.Errorf("linux default editor = %#v", c)
		}
	}
}

func TestCreateForEdit(t *testing.T) {
	dir := t.TempDir()
	p, created, err := CreateForEdit(dir, "new.txt")
	if err != nil || !created || p != filepath.Join(dir, "new.txt") || !fileExists(p) {
		t.Fatalf("create: %q %v %v", p, created, err)
	}
	if err := os.WriteFile(p, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, created, err = CreateForEdit(dir, "new.txt")
	if err != nil || created {
		t.Fatalf("existing: %v %v", created, err)
	}
	if data, _ := os.ReadFile(p); string(data) != "keep" {
		t.Error("existing file was changed")
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := CreateForEdit(dir, "sub"); err == nil {
		t.Error("a folder must not be accepted")
	}
	if _, _, err := CreateForEdit(dir, ""); err == nil {
		t.Error("an empty name must not be accepted")
	}
	if _, _, err := CreateForEdit(dir, filepath.Join("missing", "x.txt")); err == nil {
		t.Error("a missing folder should fail")
	}
}

func TestWatchFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "edit.txt")
	writeFile(t, p, "one", 0o644)
	ctx, cancel := context.WithCancel(context.Background())
	ch := WatchFile(ctx, p, 10*time.Millisecond)
	time.Sleep(30 * time.Millisecond)
	writeFile(t, p, "two two", 0o644)
	select {
	case ev := <-ch:
		if ev.Removed || ev.Size != 7 {
			t.Errorf("change event = %+v", ev)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no change noticed")
	}
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-ch:
		if !ev.Removed {
			t.Errorf("remove event = %+v", ev)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("removal not noticed")
	}
	cancel()
	for range ch {
	}
}
