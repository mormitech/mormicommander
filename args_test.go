package main

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseArgs(t *testing.T) {
	cases := []struct {
		args []string
		want startOpts
	}{
		{nil, startOpts{}},
		{[]string{"/L=/tmp/a", "/R=/tmp/b", "/P=R"}, startOpts{Left: "/tmp/a", Right: "/tmp/b", Active: "R"}},
		{[]string{"-L=/x", "-r=/y"}, startOpts{Left: "/x", Right: "/y"}},
		{[]string{"/one", "/two", "/three"}, startOpts{Left: "/one", Right: "/two"}},
		{[]string{"--left", "/a", "--right=/b"}, startOpts{Left: "/a", Right: "/b"}},
		{[]string{"--control"}, startOpts{Control: "0"}},
		{[]string{"--control", "4711", "/x"}, startOpts{Control: "4711", Left: "/x"}},
		{[]string{"--control", "/x"}, startOpts{Control: "0", Left: "/x"}},
		{[]string{"--control=0", "--control-file", "/tmp/c.json"}, startOpts{Control: "0", ControlFile: "/tmp/c.json"}},
		{[]string{"--demo"}, startOpts{Demo: true, Control: "0"}},
		{[]string{"--mock", "--theme", "light"}, startOpts{Demo: true, Control: "0", Theme: "light"}},
		{[]string{"--about", "--theme=dark"}, startOpts{About: true, Theme: "dark"}},
		{[]string{"-psn_0_1234", "--version"}, startOpts{Version: true}},
		{[]string{"/P=X"}, startOpts{}},
		{[]string{"/P=X", "/a"}, startOpts{Left: "/a"}},
		{[]string{"/T", "/a", "/b"}, startOpts{NewTab: true, TCLeft: "/a", TCRight: "/b"}},
		{[]string{"-s", "-P=R", "/a", "/b"}, startOpts{SourceTarget: true, Active: "R", TCLeft: "/b", TCRight: "/a"}},
		{[]string{"/O", "/N", "/I=/tmp/x.ini", "/F=/tmp/f.ini", "/L=/a"}, startOpts{OpenInRunning: true, NewInstance: true, IniPath: "/tmp/x.ini", FtpIniPath: "/tmp/f.ini", Left: "/a"}},
		{[]string{"/S=L:T3", "/tmp/f.txt"}, startOpts{Tool: "lister", ToolOpts: "T3", ToolFile1: "/tmp/f.txt"}},
		{[]string{"/S=C", "/a", "/b"}, startOpts{Tool: "compare", ToolFile1: "/a", ToolFile2: "/b"}},
		{[]string{"/S=S", "/a", "/b"}, startOpts{Tool: "sync", ToolFile1: "/a", ToolFile2: "/b"}},
		{[]string{"/R=/b", "/a"}, startOpts{Left: "/a", Right: "/b"}},
		{[]string{"/P=L", "/O", "/a"}, startOpts{Active: "L", OpenInRunning: true, Left: "/a"}},
		{[]string{"/INI=/tmp/x.ini"}, startOpts{IniPath: "/tmp/x.ini"}},
		{[]string{"--frobnicate", "/a"}, startOpts{Left: "/a"}},
	}
	for _, c := range cases {
		if got := parseArgs(c.args); got != c.want {
			t.Errorf("%q:\n got %+v\nwant %+v", c.args, got, c.want)
		}
	}
}

// F-852: -S=L opens only the Lister when it names an existing file (and the option is on, read by standaloneWanted).
func TestStandaloneCandidate(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		args []string
		want bool
	}{
		{[]string{"/S=L", f}, true},
		{[]string{"/S=L:T3", f}, true},
		{[]string{"/S=L", dir}, false},                        // a folder is not a file to view
		{[]string{"/S=L", filepath.Join(dir, "none")}, false}, // nor a missing file
		{[]string{"/S=L"}, false},
		{[]string{"/S=C", f, f}, false}, // compare and sync need the file manager's window
		{[]string{"--demo", "/S=L", f}, false},
		{[]string{f}, false},
	}
	for _, c := range cases {
		if got := standaloneCandidate(parseArgs(c.args)); got != c.want {
			t.Errorf("%q: got %v, want %v", c.args, got, c.want)
		}
	}
	if (&App{}).isStandalone() {
		t.Error("an app is not standalone unless main sets it")
	}
	if m := (&ConfigStart{app: &App{standalone: true}}).StartParams(); m["standalone"] != true {
		t.Errorf("StartParams does not pass standalone on: %v", m)
	}
}

// F-729 / F-735: a second start hands its command line to the running window; the window answers "ok" only to the
// right token, and the app's own flags are left out of what it applies.
func TestConfigForwardProtocol(t *testing.T) {
	if got := configTCOnly([]string{"--theme", "dark", "--control", "4711", "--left", "/a", "-psn_0_1", "/T", "/b"}); len(got) != 3 || got[0] != "--left=/a" || got[1] != "/T" || got[2] != "/b" {
		t.Errorf("configTCOnly: %q", got)
	}
	send := func(token string) string {
		c1, c2 := net.Pipe()
		defer c1.Close()
		go configServe(&App{}, c2, "secret")
		msg, _ := json.Marshal(configForward{Token: token, Args: []string{"/O", "a"}, Cwd: "/w"})
		c1.SetDeadline(time.Now().Add(2 * time.Second))
		c1.Write(append(msg, '\n'))
		reply, _ := bufio.NewReader(c1).ReadString('\n')
		return reply
	}
	if r := send("secret"); r != "ok\n" {
		t.Errorf("right token: %q", r)
	}
	if r := send("wrong"); r != "" {
		t.Errorf("wrong token answered %q", r)
	}
}
