// Tests for the exclusion list: plain folders exclude their subtree only on whole path components, and wildcard
// patterns are tried against the folder and its parents.
package fswatch

import (
	"path/filepath"
	"testing"
)

func TestMatchExclude(t *testing.T) {
	j := filepath.Join
	root := string(filepath.Separator)
	base := j(root, "data", "shared")
	cases := []struct {
		path, pattern string
		want          bool
	}{
		{base, base, true},
		{j(base, "a", "b"), base, true},
		{j(base, "a"), base + string(filepath.Separator), true},
		{base + "2", base, false},
		{j(root, "data"), base, false},
		{j(base, "x"), "", false},
		{j(base, "x"), "   ", false},
		{j(root, "Volumes", "USB", "photos"), j(root, "Volumes", "*"), true},
		{j(root, "Volumes"), j(root, "Volumes", "*"), false},
		{j(root, "home", "me", "tmp", "build"), j(root, "home", "*", "tmp"), true},
		{j(root, "home", "me", "src"), j(root, "home", "*", "tmp"), false},
	}
	for _, c := range cases {
		if got := matchExclude(c.path, c.pattern); got != c.want {
			t.Errorf("matchExclude(%q, %q) = %v, want %v", c.path, c.pattern, got, c.want)
		}
	}
	if !excluded(j(base, "a"), []string{"/nope", base}) || excluded(j(base, "a"), nil) {
		t.Error("excluded list handling")
	}
}
