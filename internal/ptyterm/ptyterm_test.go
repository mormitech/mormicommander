package ptyterm

import (
	"runtime"
	"strings"
	"testing"
)

func TestQuote(t *testing.T) {
	cases := []struct{ shell, in, want string }{
		{"/bin/zsh", "/tmp/a b", `'/tmp/a b'`},
		{"/bin/bash", "it's", `'it'\''s'`},
		{"/bin/sh", `$(rm -rf ~); a\b`, `'$(rm -rf ~); a\b'`},
		{"/usr/local/bin/fish", `it's a\b`, `'it\'s a\\b'`},
	}
	for _, c := range cases {
		got, err := Quote(c.shell, c.in)
		if err != nil || got != c.want {
			t.Errorf("Quote(%q, %q) = %q, %v; want %q", c.shell, c.in, got, err, c.want)
		}
	}
	for _, bad := range []string{"a\nb", "a\rb", "a\x03", "a\x1b[A", "a\x7f", "a\u0085b"} {
		if _, err := Quote("/bin/zsh", bad); err != ErrControlChars {
			t.Errorf("Quote(%q) must refuse a control character, got %v", bad, err)
		}
	}
}

func TestCdLine(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix shells and paths; the terminal pane has no Windows ConPTY yet")
	}
	got, err := CdLine("/bin/zsh", "/Users/x/My Folder")
	if err != nil || got != " cd '/Users/x/My Folder'\r" {
		t.Errorf("CdLine = %q, %v", got, err)
	}
	if _, err := CdLine("/bin/zsh", "relative"); err == nil {
		t.Error("a relative folder is refused")
	}
	if _, err := CdLine("/bin/zsh", "/tmp/x\n; rm -rf ~"); err == nil {
		t.Error("a new line in the folder is refused")
	}
}

func TestInsertText(t *testing.T) {
	got, err := InsertText("/bin/bash", []string{"a.txt", "", "b c"})
	if err != nil || got != `'a.txt' 'b c' ` {
		t.Errorf("InsertText = %q, %v", got, err)
	}
	if strings.ContainsAny(got, "\r\n") {
		t.Error("inserting names never ends a line")
	}
	if _, err := InsertText("/bin/bash", []string{"ok", "x\ry"}); err == nil {
		t.Error("a carriage return in a name is refused")
	}
}

func TestEnv(t *testing.T) {
	env := Env([]string{"TERM=dumb", "HOME=/h", "LANG=hu_HU.UTF-8"})
	j := strings.Join(env, "\n")
	if strings.Contains(j, "TERM=dumb") || !strings.Contains(j, "TERM=xterm-256color") || !strings.Contains(j, "HOME=/h") {
		t.Errorf("Env = %v", env)
	}
	if strings.Count(j, "LANG=") != 1 {
		t.Errorf("a locale already set is kept as it is: %v", env)
	}
}

func TestPickShell(t *testing.T) {
	sh, args := PickShell("/bin/sh")
	if sh != "/bin/sh" || len(args) != 1 || args[0] != "-l" {
		t.Errorf("PickShell(/bin/sh) = %q %v", sh, args)
	}
	if sh, _ := PickShell("relative/shell"); sh == "relative/shell" {
		t.Error("a relative shell is not taken")
	}
}
