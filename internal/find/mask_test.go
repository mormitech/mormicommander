// Tests for the "Search for" mask syntax: wildcards, TC's special cases ("*.*", "*.",
// no wildcard means *text*), quoting, ";" and space separators, "|" exclusions and
// folder exclusions ending in a slash.

package find

import "testing"

func TestGlob(t *testing.T) {
	cases := []struct {
		pat, s string
		want   bool
	}{
		{"*", "", true},
		{"*", "abc", true},
		{"a*", "abc", true},
		{"*c", "abc", true},
		{"a?c", "abc", true},
		{"a?c", "ac", false},
		{"*.txt", "notes.txt", true},
		{"*.txt", "notes.txt.bak", false},
		{"*a*b*c*", "xxaxxbxxcxx", true},
		{"*a*b*c*", "xxaxxcxxbxx", false},
		{"[a].txt", "[a].txt", true},
		{"?", "é", true},
		{"??", "é", false},
		{"é*", "état", true},
		{"a*/b", "a/x/b", true},
		{"", "", true},
		{"", "a", false},
		{"**", "abc", true},
	}
	for _, c := range cases {
		if got := Glob(c.pat, c.s); got != c.want {
			t.Errorf("Glob(%q, %q) = %v, want %v", c.pat, c.s, got, c.want)
		}
	}
}

func TestMaskMatchName(t *testing.T) {
	cases := []struct {
		mask string
		name string
		rel  string
		want bool
	}{
		{"*.txt|old*", "notes.txt", "", true},
		{"*.txt|old*", "old_notes.txt", "", false},
		{"*.txt|old*", "notes.doc", "", false},
		{"*.txt;*.doc", "a.DOC", "", true},
		{"*.txt *.doc", "a.doc", "", true},
		{"*.TXT", "readme.txt", "", true},
		{"*.*", "Makefile", "", true},
		{"*", "Makefile", "", true},
		{"", "anything", "", true},
		{"*.", "Makefile", "", true},
		{"*.", "main.go", "", false},
		{"*.", ".bashrc", "", true},
		{"report", "Q3 report final.pdf", "", true},
		{"report", "summary.pdf", "", false},
		{`"my file*"`, "my file.txt", "", true},
		{`"my file*"`, "my.txt", "", false},
		{`"a;b.txt"`, "a;b.txt", "", true},
		{"|*.bak", "x.bak", "", false},
		{"|*.bak", "x.txt", "", true},
		{"src/*.go", "main.go", "src/main.go", true},
		{"src/*.go", "main.go", "lib/main.go", false},
		{`src\*.go`, "main.go", "src/main.go", true},
		{"*.go|*_test.go", "a_test.go", "", false},
		{"*.go|*_test.go", "a.go", "", true},
		{"a?c.txt", "abc.txt", "", true},
		{"*.txt|vendor/", "x.txt", "", true}, // folder exclusions do not affect names
	}
	for _, c := range cases {
		m, err := ParseMask(c.mask)
		if err != nil {
			t.Fatalf("ParseMask(%q): %v", c.mask, err)
		}
		rel := c.rel
		if rel == "" {
			rel = c.name
		}
		if got := m.MatchName(c.name, rel); got != c.want {
			t.Errorf("mask %q name %q rel %q = %v, want %v", c.mask, c.name, rel, got, c.want)
		}
	}
}

func TestMaskSkipDir(t *testing.T) {
	cases := []struct {
		mask, name, rel string
		want            bool
	}{
		{"*.go|vendor/", "vendor", "vendor", true},
		{"*.go|vendor/", "vendor", "a/vendor", true},
		{"*.go|vendor/", "vendors", "vendors", false},
		{`*|.git\`, ".git", ".git", true},
		{"*|bin/", "cabinet", "cabinet", false},
		{"*|build*/", "build-out", "build-out", true},
		{"*|a/b/", "b", "a/b", true},
		{"*|a/b/", "b", "c/b", false},
		{"*|node_modules/ .cache/", ".cache", ".cache", true},
		{"*.go", "vendor", "vendor", false},
	}
	for _, c := range cases {
		m := MustParseMask(c.mask)
		if got := m.SkipDir(c.name, c.rel); got != c.want {
			t.Errorf("mask %q SkipDir(%q, %q) = %v, want %v", c.mask, c.name, c.rel, got, c.want)
		}
	}
}

func TestParseMaskErrors(t *testing.T) {
	for _, s := range []string{"*.a|*.b|*.c", `"unclosed`} {
		if _, err := ParseMask(s); err == nil {
			t.Errorf("ParseMask(%q): want error", s)
		}
	}
	// A "|" inside quotes is part of the name, not a separator.
	m := MustParseMask(`"a|b"`)
	if len(m.Exclude) != 0 || !m.MatchName("xa|by", "xa|by") {
		t.Errorf("quoted | handled wrong: %+v", m)
	}
}

func TestParseSearchIn(t *testing.T) {
	got := ParseSearchIn(` /a ; "/b;c" ;; /d `)
	want := []string{"/a", "/b;c", "/d"}
	if len(got) != len(want) {
		t.Fatalf("got %q want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %q want %q", got, want)
		}
	}
}

func TestNilMask(t *testing.T) {
	var m *Mask
	if !m.MatchName("x", "x") || m.SkipDir("x", "x") {
		t.Error("nil mask should match everything and skip nothing")
	}
}
