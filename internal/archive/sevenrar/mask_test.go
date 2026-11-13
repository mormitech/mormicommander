// These tests check Total Commander style file masks: lists, exclusions,
// case-insensitivity, "*.*" matching names without a dot, "?" and "[" as an
// ordinary character, and that hostile patterns do not blow up.

package sevenrar

import (
	"strings"
	"testing"
)

func TestMask(t *testing.T) {
	cases := []struct {
		mask string
		name string
		want bool
	}{
		{"", "anything", true},
		{"*.*", "noext", true},
		{"*", "a.b", true},
		{"*.txt", "README.TXT", true},
		{"*.txt", "a.txt.bak", false},
		{"*.txt *.doc", "x.doc", true},
		{"*.txt;*.doc", "x.doc", true},
		{"*.txt;*.doc", "x.pdf", false},
		{"*.* | *.bak", "x.bak", false},
		{"*.* | *.bak", "x.txt", true},
		{"| *.bak", "x.txt", true},
		{"| *.bak *.tmp", "x.tmp", false},
		{"a?c.txt", "abc.txt", true},
		{"a?c.txt", "ac.txt", false},
		{"[x].txt", "[x].txt", true},
		{"[x].txt", "x.txt", false},
		{"readme.*", "readme", true},
		{"readme.*", "readme.md", true},
		{"readme.*", "readmes", false},
		{"*a*b*c*", "xxaxxbxxcxx", true},
		{"*a*b*c*", "xxaxxcxxbxx", false},
		{"ä*.TXT", "Ärvíz.txt", true},
		{"\"my file.txt\"", "my", false},
		{"\"my file.txt\"", "My File.txt", true},
		{"*.doc \"my file.txt\";*.pdf", "x.pdf", true},
		{"*.tar.gz", "x.tar.gz", true},
	}
	for _, c := range cases {
		if got := ParseMask(c.mask).Match(c.name); got != c.want {
			t.Errorf("mask %q on %q = %v, want %v", c.mask, c.name, got, c.want)
		}
	}
	var zero Mask
	if !zero.Match("x") {
		t.Error("zero mask does not match")
	}
}

func TestMaskPathological(t *testing.T) {
	m := ParseMask(strings.Repeat("*a", 50) + "b")
	if m.Match(strings.Repeat("a", 5000)) {
		t.Error("should not match")
	}
}
