package main

import "testing"

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
	}
	for _, c := range cases {
		if got := parseArgs(c.args); got != c.want {
			t.Errorf("%q:\n got %+v\nwant %+v", c.args, got, c.want)
		}
	}
}
