// Tests for single-mask matching: wildcards, sets, the DOS ".*" and "." rules, case folding and NFC normalisation.

package masks

import (
	"strings"
	"testing"
)

func TestMaskMatch(t *testing.T) {
	tests := []struct {
		mask, name string
		cs         bool
		want       bool
	}{
		{"*", "anything.txt", true, true},
		{"*", "", true, true},
		{"*.txt", "a.txt", true, true},
		{"*.txt", "a.TXT", true, false},
		{"*.txt", "a.TXT", false, true},
		{"*.txt", "a.txt.bak", true, false},
		{"a?c", "abc", true, true},
		{"a?c", "ac", true, false},
		{"a?c", "abbc", true, false},
		{"??", "é1", true, true},
		{"*.*", "noext", true, true},
		{"*.*", "a.b", true, true},
		{"readme.*", "readme", true, true},
		{"readme.*", "readme.md", true, true},
		{"readme.*", "readmes", true, false},
		{"*.", "noext", true, true},
		{"*.", "a.txt", true, false},
		{"*.", ".bashrc", true, true},
		{"abc.", "abc", true, true},
		{"abc.", "abc.", true, true},
		{"[abc]*", "beta", true, true},
		{"[abc]*", "delta", true, false},
		{"[a-c]x", "bx", true, true},
		{"[c-a]x", "bx", true, true},
		{"[!a-c]x", "bx", true, false},
		{"[^a-c]x", "dx", true, true},
		{"[a-c]x", "Bx", false, true},
		{"[a-c]x", "Bx", true, false},
		{"[]]x", "]x", true, true},
		{"[draft", "[draft", true, true},
		{"[[]draft].txt", "[draft].txt", true, true},
		{"*a*a*a*a*a*b", strings.Repeat("a", 200), true, false},
		{"**x", "abx", true, true},
		{"report", "REPORT", false, true},
		{"report", "REPORT", true, false},
		{"straße*", "STRASSE", false, false}, // simple folding only, as file systems do
		{"ÄBC", "äbc", false, true},
		{"café*", "cafe\u0301 menu", true, true}, // NFD name, NFC mask
		{"cafe\u0301", "café", true, true},
		{"a b", "a b", true, true},
	}
	for _, tt := range tests {
		if got := CompileMask(tt.mask, tt.cs).Match(tt.name); got != tt.want {
			t.Errorf("CompileMask(%q, cs=%v).Match(%q) = %v, want %v", tt.mask, tt.cs, tt.name, got, tt.want)
		}
	}
}

func TestMaskDirOnly(t *testing.T) {
	for _, s := range []string{`.git\`, ".git/"} {
		m := CompileMask(s, true)
		if !m.DirOnly() || !m.Match(".git") || m.String() != ".git/" {
			t.Errorf("%q: DirOnly=%v Match=%v String=%q", s, m.DirOnly(), m.Match(".git"), m.String())
		}
	}
	if CompileMask("/", true).DirOnly() {
		t.Error(`a lone "/" must not become an empty folder mask`)
	}
}

func TestPathMask(t *testing.T) {
	tests := []struct {
		mask, path string
		want       bool
	}{
		{`C:\Windows\*.dll`, "C:/Windows/a.dll", true},
		{`C:\Windows\*.dll`, "C:/Windows/sub/a.dll", false},
		{"/tmp/*", "/tmp/x", true},
		{"/tmp/*", "/tmp/x/y", false},
		{"/t?p/x", "/tmp/x", true},
		{"/home/*/cache", "/home/me/cache", true},
	}
	for _, tt := range tests {
		if got := compilePath(tt.mask, true).Match(tt.path); got != tt.want {
			t.Errorf("compilePath(%q).Match(%q) = %v, want %v", tt.mask, tt.path, got, tt.want)
		}
	}
}

func TestSplitExt(t *testing.T) {
	tests := []struct{ name, base, ext string }{
		{"a.txt", "a", "txt"},
		{"a.tar.gz", "a.tar", "gz"},
		{".bashrc", ".bashrc", ""},
		{"noext", "noext", ""},
		{"trail.", "trail", ""},
		{".a.b", ".a", "b"},
	}
	for _, tt := range tests {
		b, e := SplitExt(tt.name)
		if b != tt.base || e != tt.ext {
			t.Errorf("SplitExt(%q) = %q, %q; want %q, %q", tt.name, b, e, tt.base, tt.ext)
		}
	}
}

func TestHasWildcards(t *testing.T) {
	for s, want := range map[string]bool{"a.txt": false, "*.txt": true, "a?": true, "[ab]": true, "[ab": false} {
		if got := CompileMask(s, true).HasWildcards(); got != want {
			t.Errorf("HasWildcards(%q) = %v, want %v", s, got, want)
		}
	}
}
