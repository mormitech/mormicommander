package dirsize

import "testing"

func TestIsAliasDir(t *testing.T) {
	for _, c := range []struct {
		p, root string
		want    bool
	}{
		{"/System/Volumes/Data", "/System", true},
		{"/System/Volumes/Data", "/", true},
		{"/System/Volumes/Data", "/System/Volumes/Data", false},
		{"/System/Volumes/Data", "/System/Volumes/Data/Users", false},
		{"/System/Volumes/Preboot", "/System", false},
		{"/Users", "/", false},
	} {
		if got := isAliasDir(c.p, c.root); got != c.want {
			t.Errorf("isAliasDir(%q, %q) = %v, want %v", c.p, c.root, got, c.want)
		}
	}
}
