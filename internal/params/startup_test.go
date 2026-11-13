// Tests for start-up parameter parsing: TC switches in "/", "-" and "--"
// form, the POSIX path ambiguity, /S=L lister options, panel resolution
// with /S and /P, absolutising for /O and forwarding.
package params

import (
	"reflect"
	"testing"
)

func TestParseStartup(t *testing.T) {
	cases := []struct {
		args []string
		want Startup
	}{
		{nil, Startup{}},
		{[]string{`C:\a`, `C:\b`}, Startup{Paths: []string{`C:\a`, `C:\b`}}},
		{[]string{"/O", "/t", "-N", "--S"}, Startup{OpenInRunning: true, NewTab: true, NewInstance: true, SourceTarget: true}},
		{[]string{"/L=/tmp/a", "-R=/tmp/b"}, Startup{Left: "/tmp/a", Right: "/tmp/b"}},
		{[]string{"--left=x", "--right=y"}, Startup{Left: "x", Right: "y"}},
		{[]string{"/P=r"}, Startup{ActivePanel: 'R'}},
		{[]string{"/P=L"}, Startup{ActivePanel: 'L'}},
		{[]string{"/I=c:\\tc.ini", "/INI=d.ini", "/F=ftp.ini"}, Startup{IniPath: "d.ini", FtpIniPath: "ftp.ini"}},
		{[]string{`/L=C:\My dir"`}, Startup{Left: `C:\My dir`}},
		{[]string{`C:\My dir"`}, Startup{Paths: []string{`C:\My dir`}}},
		{[]string{"/Users/p", "/O"}, Startup{Paths: []string{"/Users/p"}, OpenInRunning: true}},
		{[]string{"/Ox", "/X=1"}, Startup{Paths: []string{"/Ox", "/X=1"}}},
		{[]string{"--control", "0", "--theme=dark"}, Startup{Paths: []string{"0"}, Unknown: []string{"--control", "--theme=dark"}}},
		{[]string{"--", "/O", "-x"}, Startup{Paths: []string{"/O", "-x"}}},
		{[]string{"-"}, Startup{Paths: []string{"-"}}},
		{[]string{"/S=L", "/tmp/f.txt"}, Startup{Tool: ToolLister, Paths: []string{"/tmp/f.txt"}}},
		{[]string{"/S=L:AT3C1250", "f"}, Startup{Tool: ToolLister, Paths: []string{"f"},
			Lister: ListerOptions{Raw: "AT3C1250", Charset: 'A', Mode: 3, Codepage: 1250}}},
		{[]string{"/S=l:nPImageViewer"}, Startup{Tool: ToolLister,
			Lister: ListerOptions{Raw: "nPImageViewer", AutoDetect: true, Plugin: "ImageViewer"}}},
		{[]string{"-S=C", "a", "b"}, Startup{Tool: ToolCompare, Paths: []string{"a", "b"}}},
		{[]string{"-S=S", "a", "b"}, Startup{Tool: ToolSync, Paths: []string{"a", "b"}}},
	}
	for _, c := range cases {
		got, err := ParseStartup(c.args)
		if err != nil {
			t.Fatalf("%q: %v", c.args, err)
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("ParseStartup(%q) =\n %+v, want\n %+v", c.args, got, c.want)
		}
	}
}

func TestParseStartupErrors(t *testing.T) {
	bad := [][]string{{"/P=X"}, {"/L="}, {"-INI="}, {"/S=Q"}, {"/S=L:T9"}, {"/S=L:T"}, {"/S=L:C"}, {"/S=L:Z"}}
	for _, a := range bad {
		if _, err := ParseStartup(a); err == nil {
			t.Errorf("ParseStartup(%q) should fail", a)
		}
	}
}

func TestPanels(t *testing.T) {
	cases := []struct {
		args          []string
		activeIsRight bool
		l, r          string
	}{
		{[]string{"a", "b"}, false, "a", "b"},
		{[]string{"a"}, true, "a", ""},
		{[]string{"/R=b"}, false, "", "b"},
		{[]string{"/R=b", "a"}, false, "a", "b"},
		{[]string{"/L=x", "a", "b"}, false, "x", "a"},
		{[]string{"/S", "src", "tgt"}, false, "src", "tgt"},
		{[]string{"/S", "src", "tgt"}, true, "tgt", "src"},
		{[]string{"/S", "/P=R", "src", "tgt"}, false, "tgt", "src"},
		{[]string{"/S", "/P=L", "src"}, true, "src", ""},
		{[]string{"/S=L", "file"}, false, "", ""},
	}
	for _, c := range cases {
		s, err := ParseStartup(c.args)
		if err != nil {
			t.Fatal(err)
		}
		l, r := s.Panels(c.activeIsRight)
		if l != c.l || r != c.r {
			t.Errorf("%q right=%v: got %q,%q want %q,%q", c.args, c.activeIsRight, l, r, c.l, c.r)
		}
	}
}

func TestAbsoluteAndArgs(t *testing.T) {
	s, _ := ParseStartup([]string{"/O", "/T", "/S", "/P=R", "/L=a", "/R=/abs", "/I=x.ini", "/F=ftp://h/f", "rel", "--theme"})
	a := s.Absolute("/home/p", StylePOSIX)
	if a.Left != "/home/p/a" || a.Right != "/abs" || a.IniPath != "/home/p/x.ini" ||
		a.FtpIniPath != "ftp://h/f" || a.Paths[0] != "/home/p/rel" || s.Paths[0] != "rel" {
		t.Errorf("absolute: %+v", a)
	}
	args := a.Args()
	want := []string{"-T", "-S", "-P=R", "-L=/home/p/a", "-R=/abs", "-I=/home/p/x.ini", "-F=ftp://h/f", "--", "/home/p/rel"}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("args = %q", args)
	}
	back, err := ParseStartup(args)
	a.OpenInRunning, a.Unknown = false, nil
	if err != nil || !reflect.DeepEqual(back, a) {
		t.Errorf("round trip = %+v, %v; want %+v", back, err, a)
	}
	for _, in := range [][]string{{"/S=L:A", "f"}, {"/S=C"}, {"/S=S"}} {
		s, _ := ParseStartup(in)
		b, _ := ParseStartup(s.Args())
		if !reflect.DeepEqual(b, s) {
			t.Errorf("tool round trip %q: %+v vs %+v", in, b, s)
		}
	}
	w, _ := ParseStartup([]string{`sub`, `D:\x`})
	w = w.Absolute(`C:\cur`, StyleWindows)
	if w.Paths[0] != `C:\cur\sub` || w.Paths[1] != `D:\x` {
		t.Errorf("windows absolute: %q", w.Paths)
	}
}
