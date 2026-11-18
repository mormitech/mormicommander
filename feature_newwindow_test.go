package main

import (
	"errors"
	"reflect"
	"testing"

	"mormicommander/internal/launch"
)

// F-376: the new window's argument list, and that "Allow only one copy" stops it with an explanation.
func TestNewWindowArgs(t *testing.T) {
	got := newWindowArgs(startOpts{}, "/home/me/My files")
	if want := []string{"/N", "/L=/home/me/My files", "/P=L"}; !reflect.DeepEqual(got, want) {
		t.Errorf("args = %q, want %q", got, want)
	}
	got = newWindowArgs(startOpts{IniPath: "/x/my.ini", FtpIniPath: "/x/ftp.ini"}, "/w")
	if want := []string{"/N", "/L=/w", "/P=L", "/I=/x/my.ini", "/F=/x/ftp.ini"}; !reflect.DeepEqual(got, want) {
		t.Errorf("args = %q, want %q", got, want)
	}
	// the new copy reads them as a new window with the folder on the left, active
	o := parseArgs(newWindowArgs(startOpts{IniPath: "/x/my.ini"}, "/home/me/My files"))
	if !o.NewInstance || o.Left != "/home/me/My files" || o.Active != "L" || o.IniPath != "/x/my.ini" || configForwardWanted(newWindowArgs(startOpts{}, "/w")) {
		t.Errorf("parsed back: %+v", o)
	}
}

func TestNewWindowOpen(t *testing.T) {
	dir := t.TempDir()
	n := newConfigNewWindow(NewApp(startOpts{}))
	var got []launch.Command
	n.start = func(c launch.Command) error { got = append(got, c); return nil }
	n.exe = func() (string, error) { return "/apps/mc", nil }
	only := false
	n.onlyOnce = func() bool { return only }

	if err := n.Open(dir); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Path != "/apps/mc" || got[0].Args[0] != "/N" || got[0].Args[1] != "/L="+dir {
		t.Errorf("started %+v", got)
	}
	only = true
	if err := n.Open(dir); !errors.Is(err, errOnlyOneCopy) {
		t.Errorf("with Allow only one copy: %v", err)
	}
	only = false
	if err := n.Open("sftp://host/x"); err == nil {
		t.Error("a server folder can't open in a new window")
	}
	if err := n.Open(dir + "/missing"); err == nil {
		t.Error("a missing folder must fail")
	}
	if len(got) != 1 {
		t.Errorf("nothing more may start: %+v", got)
	}
}
