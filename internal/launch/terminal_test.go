// Tests for opening terminals and running command lines in them on each OS.
package launch

import (
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestOpenTerminal(t *testing.T) {
	tests := []struct {
		name     string
		os       string
		terminal string
		env      map[string]string
		path     []string
		want     Command
	}{
		{"mac default", "darwin", "", nil, nil, Command{Path: "open", Args: []string{"-a", "Terminal", "/w d"}, Dir: "/w d"}},
		{"mac iTerm", "darwin", "iTerm", nil, nil, Command{Path: "open", Args: []string{"-a", "iTerm", "/w d"}, Dir: "/w d"}},
		{"win wt", "windows", "", nil, []string{"wt.exe"}, Command{Path: "wt.exe", Params: `-d "C:\w d\\"`, Dir: `C:\w d\`}},
		{"win cmd", "windows", "", nil, nil, Command{Path: "cmd.exe", Params: "/k", Dir: `C:\w d\`, NewConsole: true}},
		{"win pwsh", "windows", "pwsh", nil, nil, Command{Path: "pwsh.exe", Params: "-NoExit", Dir: `C:\w d\`, NewConsole: true}},
		{"linux gnome", "linux", "", map[string]string{"XDG_CURRENT_DESKTOP": "ubuntu:GNOME"}, []string{"xterm", "gnome-terminal"},
			Command{Path: "gnome-terminal", Args: []string{"--working-directory=/w d"}, Dir: "/w d"}},
		{"linux $TERMINAL", "linux", "", map[string]string{"TERMINAL": "kitty"}, []string{"kitty", "xterm"},
			Command{Path: "kitty", Args: []string{"--directory", "/w d"}, Dir: "/w d"}},
		{"linux unknown $TERMINAL", "linux", "", map[string]string{"TERMINAL": "myterm"}, []string{"myterm"},
			Command{Path: "myterm", Dir: "/w d"}},
		{"linux probe", "linux", "", nil, []string{"xterm"}, Command{Path: "xterm", Dir: "/w d"}},
		{"linux wezterm", "linux", "wezterm", nil, []string{"wezterm"}, Command{Path: "wezterm", Args: []string{"start", "--cwd", "/w d"}, Dir: "/w d"}},
		{"custom", "linux", "foot -D %D", nil, nil, Command{Path: "foot", Args: []string{"-D", "/w d"}, Dir: "/w d"}},
	}
	for _, tt := range tests {
		l, _ := fakeLauncher(t, tt.os, tt.env, tt.path...)
		l.Terminal = tt.terminal
		dir := "/w d"
		if tt.os == "windows" {
			dir = `C:\w d\`
		}
		got, err := l.OpenTerminalCommand(dir)
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		t.Run(tt.name, func(t *testing.T) { sameCmd(t, got, tt.want) })
	}
	l, _ := fakeLauncher(t, "linux", nil)
	if err := l.OpenTerminal("/x"); !errors.Is(err, ErrNoTerminal) {
		t.Errorf("no terminal installed: %v", err)
	}
}

func TestRunInTerminal(t *testing.T) {
	w, _ := fakeLauncher(t, "windows", nil)
	c, _ := w.RunInTerminalCommand(`C:\w`, `dir /s & pause`, true)
	sameCmd(t, c, Command{Path: "cmd.exe", Params: `/k dir /s & pause`, Dir: `C:\w`, NewConsole: true})
	c, _ = w.RunInTerminalCommand(`C:\w`, `ipconfig`, false)
	sameCmd(t, c, Command{Path: "cmd.exe", Params: `/c ipconfig`, Dir: `C:\w`, NewConsole: true})

	l, _ := fakeLauncher(t, "linux", nil, "konsole")
	c, _ = l.RunInTerminalCommand("/it's", "make test", true)
	script := "cd -- '/it'\\''s' || exit 1\nmake test\nexec \"${SHELL:-/bin/sh}\" -i\n"
	sameCmd(t, c, Command{Path: "konsole", Args: []string{"--workdir", "/it's", "-e", "/bin/sh", "-c", script}, Dir: "/it's"})

	l2, _ := fakeLauncher(t, "linux", nil, "tilix")
	c, _ = l2.RunInTerminalCommand("/w", "ls", false)
	sameCmd(t, c, Command{Path: "tilix", Args: []string{"--working-directory=/w", "-e", "/bin/sh -c 'cd -- /w || exit 1\nls\n'"}, Dir: "/w"})

	m, _ := fakeLauncher(t, "darwin", nil)
	c, err := m.RunInTerminalCommand("/w d", "ls -l", false)
	if err != nil {
		t.Fatal(err)
	}
	if c.Path != "open" || len(c.Args) != 3 || c.Args[0] != "-a" || c.Args[1] != "Terminal" || !strings.HasSuffix(c.Args[2], ".command") {
		t.Fatalf("mac run command = %#v", c)
	}
	data, err := os.ReadFile(c.Args[2])
	if err != nil {
		t.Fatal(err)
	}
	want := "#!/bin/sh\nrm -f \"$0\"\ncd -- '/w d' || exit 1\nls -l\n"
	if string(data) != want {
		t.Errorf("script =\n%s\nwant\n%s", data, want)
	}
	if fi, _ := os.Stat(c.Args[2]); runtime.GOOS != "windows" && fi.Mode().Perm() != 0o700 {
		t.Errorf("script mode = %v", fi.Mode())
	}
}
