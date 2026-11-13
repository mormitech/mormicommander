// Tests for the OS-aware path helpers used by "cd" and the placeholders.
package launch

import "testing"

func TestJoinPath(t *testing.T) {
	tests := []struct{ os, base, rel, want string }{
		{"linux", "/home/u", "docs", "/home/u/docs"},
		{"linux", "/home/u", "..", "/home"},
		{"linux", "/home/u", "/etc", "/etc"},
		{"linux", "/", "..", "/"},
		{"linux", "/a", "", "/a"},
		{"windows", `C:\Users\u`, "docs", `C:\Users\u\docs`},
		{"windows", `C:\Users\u`, `..\..`, `C:\`},
		{"windows", `C:\Users\u`, `..\..\..`, `C:\`},
		{"windows", `C:\Users\u`, `\Windows`, `C:\Windows`},
		{"windows", `C:\Users\u`, `D:`, `D:\`},
		{"windows", `C:\Users\u`, `D:\x/y`, `D:\x\y`},
		{"windows", `C:\Users\u`, `\\srv\share\dir`, `\\srv\share\dir`},
		{"windows", `\\srv\share\a`, `..\..`, `\\srv\share`},
		{"windows", `C:\`, "x", `C:\x`},
	}
	for _, tt := range tests {
		if got := joinPath(tt.os, tt.base, tt.rel); got != tt.want {
			t.Errorf("joinPath(%s, %q, %q) = %q, want %q", tt.os, tt.base, tt.rel, got, tt.want)
		}
	}
}

func TestPathBits(t *testing.T) {
	if got := withTrailingSep("windows", `C:\x`); got != `C:\x\` {
		t.Errorf("withTrailingSep = %q", got)
	}
	if got := withTrailingSep("linux", "/"); got != "/" {
		t.Errorf("withTrailingSep root = %q", got)
	}
	if got := baseName("windows", `C:\a\b.txt`); got != "b.txt" {
		t.Errorf("baseName = %q", got)
	}
	for _, tt := range []struct{ name, stem, ext string }{
		{"a.tar.gz", "a.tar", "gz"}, {".profile", ".profile", ""}, {"noext", "noext", ""}, {"x.", "x", ""},
	} {
		if s, e := splitExt(tt.name); s != tt.stem || e != tt.ext {
			t.Errorf("splitExt(%q) = %q, %q", tt.name, s, e)
		}
	}
	if !isAbsPath("windows", `C:\x`) || isAbsPath("windows", `C:x`) || isAbsPath("windows", `\x`) || !isAbsPath("windows", `\\s\h`) {
		t.Error("isAbsPath windows rules are wrong")
	}
}
