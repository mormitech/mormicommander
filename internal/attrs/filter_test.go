// Tests for TC-style masks used to restrict recursive attribute changes.
package attrs

import (
	"reflect"
	"testing"
)

func TestParseMask(t *testing.T) {
	f := ParseMask(`*.txt;*.doc "my file.*" | *.bak .git\ node_modules/`)
	if !reflect.DeepEqual(f.Include, []string{"*.txt", "*.doc", "my file.*"}) {
		t.Errorf("Include = %q", f.Include)
	}
	if !reflect.DeepEqual(f.Exclude, []string{"*.bak", `.git\`, "node_modules/"}) {
		t.Errorf("Exclude = %q", f.Exclude)
	}
	if s := f.String(); s != `*.txt *.doc "my file.*" | *.bak .git\ node_modules/` {
		t.Errorf("String = %q", s)
	}
	if (*Filter)(nil).String() != "" {
		t.Error("nil String")
	}
}

func TestFilterMatches(t *testing.T) {
	tests := []struct {
		mask  string
		name  string
		isDir bool
		want  bool
	}{
		{"", "anything", false, true},
		{"", "folder", true, true},
		{"*.txt", "a.txt", false, true},
		{"*.txt", "A.TXT", false, true},
		{"*.txt", "a.doc", false, false},
		{"*.txt", "dir.txt", true, false}, // file patterns do not pick folders
		{"*.*", "noext", false, true},
		{"*.*", "folder", true, true},
		{"*", "folder", true, true},
		{"src/", "src", true, true},
		{"src/", "src", false, false},
		{`*.txt | secret*`, "secret.txt", false, false},
		{`| *.bak`, "x.bak", false, false},
		{`| *.bak`, "x.go", false, true},
		{`| .git\`, ".git", true, false},
		{`| .git\`, ".git", false, true},
		{"a?c", "abc", false, true},
		{"[ab]*", "beta", false, true},
		{"[", "[", false, true}, // invalid pattern falls back to literal
	}
	for _, tt := range tests {
		f := ParseMask(tt.mask)
		kind := KindFile
		if tt.isDir {
			kind = KindDir
		}
		if got := f.Matches(Info{Name: tt.name, Kind: kind}); got != tt.want {
			t.Errorf("mask %q name %q dir=%v: got %v want %v", tt.mask, tt.name, tt.isDir, got, tt.want)
		}
	}
	var nilF *Filter
	if !nilF.Matches(Info{Name: "x"}) || !nilF.Descend("x") {
		t.Error("nil filter must match all")
	}
}

func TestFilterDescendAndHook(t *testing.T) {
	f := ParseMask(`*.go | .git\ vendor/ *.tmp`)
	if f.Descend(".git") || f.Descend("vendor") {
		t.Error("excluded folders must not be entered")
	}
	if !f.Descend("src") || !f.Descend("x.tmp") {
		t.Error("only folder exclusions stop descent")
	}
	f.Match = func(path string, info Info) bool { return info.Size > 10 }
	if f.Matches(Info{Name: "a.go", Size: 5}) || !f.Matches(Info{Name: "a.go", Size: 50}) {
		t.Error("Match hook not applied")
	}
}
