package dirsize

import (
	"path/filepath"
	"sort"
	"testing"
)

func TestCache(t *testing.T) {
	base := filepath.FromSlash("/data")
	p := func(s string) string { return filepath.Join(base, filepath.FromSlash(s)) }
	all := []string{"", "a", "a/b", "a/b/c", "a/bc", "x", "x/y"}

	tests := []struct {
		invalidate string
		keep       []string
	}{
		{"a/b", []string{"a/bc", "x", "x/y"}},              // self, ancestors, descendants go; the sibling "a/bc" stays
		{"x/y/new", []string{"a", "a/b", "a/b/c", "a/bc"}}, // a new file in x/y: x/y, x and /data go
		{"a", []string{"x", "x/y"}},
		{"elsewhere", nil}, // a path outside /data: nothing is removed (checked separately below)
	}
	for _, tc := range tests {
		var c Cache
		for _, s := range all {
			c.Put(Result{Path: p(s), Complete: true, Bytes: 1})
		}
		if tc.invalidate == "elsewhere" {
			c.Invalidate(filepath.FromSlash("/other/thing"))
			if c.Len() != len(all) {
				t.Errorf("unrelated invalidate removed entries: %d left", c.Len())
			}
			continue
		}
		c.Invalidate(p(tc.invalidate))
		var left []string
		for _, s := range all {
			if _, ok := c.Get(p(s)); ok {
				left = append(left, s)
			}
		}
		sort.Strings(left)
		if !equal(left, tc.keep) {
			t.Errorf("Invalidate(%q) left %v, want %v", tc.invalidate, left, tc.keep)
		}
	}

	var c Cache
	c.Put(Result{Path: p("partial"), Complete: false})
	if _, ok := c.Get(p("partial")); ok {
		t.Error("incomplete result was cached")
	}
	c.Put(Result{Path: p("dir") + string(filepath.Separator), Complete: true, Bytes: 7})
	if r, ok := c.Get(p("dir")); !ok || r.Bytes != 7 {
		t.Error("lookup does not clean paths")
	}
	c.Clear()
	if c.Len() != 0 {
		t.Error("Clear left entries")
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
