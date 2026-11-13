// Tests for the Total Commander filter syntax used by the Synchronize
// dialog: wildcards, exclusions after "|", folder patterns ending in a
// slash or backslash, quoted patterns and path patterns.
package syncdirs

import "testing"

func TestFilterFiles(t *testing.T) {
	cases := []struct {
		filter, rel string
		want        bool
	}{
		{"", "a.txt", true},
		{"*.*", "README", true},
		{"*.txt", "a.TXT", true},
		{"*.txt", "a.doc", false},
		{"*.txt *.doc", "a.doc", true},
		{"*.txt;*.doc", "b/a.doc", true},
		{"| *.bak", "a.bak", false},
		{"| *.bak", "a.txt", true},
		{"*.* | *.bak *.tmp", "x.tmp", false},
		{`"my file*.txt"`, "my file 1.txt", true},
		{`"my file*.txt"`, "file.txt", false},
		{"src/*.go", "src/a.go", true},
		{`src\*.go`, "src/a.go", true},
		{"src/*.go", "lib/a.go", false},
		{`* | build\`, "a.go", true}, // folder pattern does not hide files
	}
	for _, c := range cases {
		f := ParseFilter(c.filter)
		if got := f.File(c.rel, baseName(c.rel), false); got != c.want {
			t.Errorf("filter %q file %q = %v, want %v", c.filter, c.rel, got, c.want)
		}
	}
}

func TestFilterDirs(t *testing.T) {
	cases := []struct {
		filter, rel string
		parentIncl  bool
		enter, incl bool
	}{
		{"", "x", false, true, true},
		{"*.txt", "x", false, true, true},
		{`| .git\`, ".git", false, false, false},
		{`| .git\`, "sub/.git", false, false, false},
		{`| node_modules/`, "node_modules", false, false, false},
		{`| node_modules/`, "src", false, true, true},
		{`src\`, "src", false, true, true},
		{`src\`, "lib", false, false, false},
		{`src\`, "src/deep", true, true, true},
		{`src\ | src\tmp\`, "src/tmp", true, false, false},
	}
	for _, c := range cases {
		f := ParseFilter(c.filter)
		enter, incl := f.Dir(c.rel, baseName(c.rel), c.parentIncl)
		if enter != c.enter || incl != c.incl {
			t.Errorf("filter %q dir %q = (%v,%v), want (%v,%v)", c.filter, c.rel, enter, incl, c.enter, c.incl)
		}
	}
}
