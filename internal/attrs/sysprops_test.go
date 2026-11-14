// Tests for the native properties window command builders. Nothing is
// started; only the argument lists are checked.
package attrs

import (
	"errors"
	"strings"
	"testing"
)

func TestFinderInfoCommand(t *testing.T) {
	path := `/tmp/it's "quoted" \ a,b`
	cmd := finderInfoCommand(path)
	if cmd.Args[0] != "osascript" || cmd.Args[len(cmd.Args)-1] != path {
		t.Errorf("args = %q", cmd.Args)
	}
	for _, a := range cmd.Args[1 : len(cmd.Args)-1] {
		if strings.Contains(a, "quoted") {
			t.Error("path must not be embedded in the script")
		}
	}
}

func TestFileManagerPropertiesCommand(t *testing.T) {
	cmd := fileManagerPropertiesCommand("/home/ann/a b,c.txt")
	var arr string
	for _, a := range cmd.Args {
		if strings.HasPrefix(a, "array:string:") {
			arr = a
		}
	}
	if arr != "array:string:file:///home/ann/a%20b%2Cc.txt" {
		t.Errorf("array arg = %q (all %q)", arr, cmd.Args)
	}
	if cmd.Args[0] != "dbus-send" {
		t.Errorf("program = %q", cmd.Args[0])
	}
}

func TestOnUIThreadDirect(t *testing.T) {
	old := UIThread
	defer func() { UIThread = old }()
	UIThread = nil
	ran := false
	if err := onUIThread(func() error { ran = true; return nil }); err != nil || !ran {
		t.Errorf("ran=%v err=%v", ran, err)
	}
}

func TestOnUIThreadDispatches(t *testing.T) {
	old := UIThread
	defer func() { UIThread = old }()
	dispatched := 0
	UIThread = func(fn func()) { dispatched++; fn() }
	want := errors.New("boom")
	if err := onUIThread(func() error { return want }); err != want {
		t.Errorf("err = %v, want %v", err, want)
	}
	if dispatched != 1 {
		t.Errorf("dispatched %d times, want 1", dispatched)
	}
}
