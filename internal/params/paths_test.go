// Tests for the style-aware path helpers, including Windows drives, UNC
// shares and virtual ftp:// paths, all run on any host.
package params

import "testing"

func TestCleanAndJoin(t *testing.T) {
	cases := []struct {
		base, rel string
		st        Style
		want      string
	}{
		{"/a/b", "..", StylePOSIX, "/a"},
		{"/a/b", "../../..", StylePOSIX, "/"},
		{"/a/b", "c/./d", StylePOSIX, "/a/b/c/d"},
		{"/a/b", "/x", StylePOSIX, "/x"},
		{`C:\a\b`, "..", StyleWindows, `C:\a`},
		{`C:\a\b`, `..\..\..`, StyleWindows, `C:\`},
		{`C:\a`, `\x`, StyleWindows, `C:\x`},
		{`C:\a`, `D:\x`, StyleWindows, `D:\x`},
		{`C:\a`, `d:x`, StyleWindows, `D:\x`},
		{`C:\a`, `sub/dir`, StyleWindows, `C:\a\sub\dir`},
		{`\\srv\share\a`, "..", StyleWindows, `\\srv\share\`},
		{`\\srv\share\a`, `..\..`, StyleWindows, `\\srv\share\`},
		{`\\srv\share`, `x`, StyleWindows, `\\srv\share\x`},
		{"ftp://host/a/b", "..", StylePOSIX, "ftp://host/a"},
		{"ftp://host/a", "../..", StyleWindows, "ftp://host/"},
		{"ftp://host", "x", StyleWindows, "ftp://host/x"},
	}
	for _, c := range cases {
		if got := JoinPath(c.base, c.rel, c.st); got != c.want {
			t.Errorf("JoinPath(%q,%q,%d) = %q, want %q", c.base, c.rel, c.st, got, c.want)
		}
	}
}

func TestIsAbsVolumeRoot(t *testing.T) {
	cases := []struct {
		p         string
		st        Style
		abs       bool
		vol, root string
	}{
		{`C:\x`, StyleWindows, true, "C:", `C:\`},
		{`c:x`, StyleWindows, false, "C:", `C:\`},
		{`\x`, StyleWindows, false, "", `\`},
		{`\\srv\sh\x`, StyleWindows, true, `\\srv\sh`, `\\srv\sh\`},
		{`//srv/sh/x`, StyleWindows, true, `\\srv\sh`, `\\srv\sh\`},
		{"/x", StylePOSIX, true, "", "/"},
		{"x", StylePOSIX, false, "", "/"},
		{"sftp://h:22/x/y", StylePOSIX, true, "", "sftp://h:22/"},
	}
	for _, c := range cases {
		if got := IsAbs(c.p, c.st); got != c.abs {
			t.Errorf("IsAbs(%q) = %v", c.p, got)
		}
		if got := VolumeName(c.p, c.st); got != c.vol {
			t.Errorf("VolumeName(%q) = %q, want %q", c.p, got, c.vol)
		}
		if got := RootOf(c.p, c.st); got != c.root {
			t.Errorf("RootOf(%q) = %q, want %q", c.p, got, c.root)
		}
	}
}

func TestCleanPathEdge(t *testing.T) {
	cases := []struct {
		p    string
		st   Style
		want string
	}{
		{"", StylePOSIX, "."},
		{"", StyleWindows, "."},
		{"C:", StyleWindows, "C:."},
		{`C:\\a\\\b\`, StyleWindows, `C:\a\b`},
		{"a//b/", StylePOSIX, "a/b"},
	}
	for _, c := range cases {
		if got := CleanPath(c.p, c.st); got != c.want {
			t.Errorf("CleanPath(%q) = %q, want %q", c.p, got, c.want)
		}
	}
}

func TestTrailingSepAndExt(t *testing.T) {
	if got := WithTrailingSep(`C:\x`, StyleWindows); got != `C:\x\` {
		t.Error(got)
	}
	if got := WithTrailingSep(`C:\`, StyleWindows); got != `C:\` {
		t.Error(got)
	}
	if got := WithTrailingSep("ftp://h/x", StyleWindows); got != "ftp://h/x/" {
		t.Error(got)
	}
	if got := WithTrailingSep("", StylePOSIX); got != "" {
		t.Error(got)
	}
	ext := []struct {
		name, base, ext string
		dir             bool
	}{
		{"a.tar.gz", "a.tar", "gz", false},
		{"noext", "noext", "", false},
		{".bashrc", ".bashrc", "", false},
		{"dir.d", "dir.d", "", true},
		{"trail.", "trail", "", false},
	}
	for _, c := range ext {
		b, e := splitExt(c.name, c.dir)
		if b != c.base || e != c.ext {
			t.Errorf("splitExt(%q) = %q,%q", c.name, b, e)
		}
	}
}
