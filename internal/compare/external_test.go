// Tests for external diff tool command expansion and same-name pairing.
package compare

import (
	"reflect"
	"testing"
)

func TestExternalCommand(t *testing.T) {
	tests := []struct {
		tmpl string
		want []string
		ok   bool
	}{
		{"meld %1 %2", []string{"meld", "/a b/x", "/c/y"}, true},
		{`"C:\Program Files\WinMerge\WinMergeU.exe" /e "%1" "%2"`, []string{`C:\Program Files\WinMerge\WinMergeU.exe`, "/e", "/a b/x", "/c/y"}, true},
		{"opendiff", []string{"opendiff", "/a b/x", "/c/y"}, true},
		{"tool --left=%1 --right=%2 100%%", []string{"tool", "--left=/a b/x", "--right=/c/y", "100%"}, true},
		{"tool %2 %1", []string{"tool", "/c/y", "/a b/x"}, true},
		{`tool ""`, []string{"tool", "", "/a b/x", "/c/y"}, true},
		{"tool %x", []string{"tool", "%x", "/a b/x", "/c/y"}, true},
		{"   ", nil, false},
		{`"unterminated %1`, nil, false},
	}
	for _, tc := range tests {
		got, err := ExternalCommand(tc.tmpl, "/a b/x", "/c/y")
		if (err == nil) != tc.ok || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("ExternalCommand(%q) = %q, %v; want %q", tc.tmpl, got, err, tc.want)
		}
	}
}

func TestPairByName(t *testing.T) {
	tests := []struct {
		name         string
		left, right  []string
		ci           bool
		pairs        []Pair
		onlyL, onlyR []string
	}{
		{"exact", []string{"b.txt", "a.txt"}, []string{"a.txt", "b.txt"}, false,
			[]Pair{{"a.txt", "a.txt"}, {"b.txt", "b.txt"}}, nil, nil},
		{"case sensitive", []string{"A.txt"}, []string{"a.txt"}, false, nil, []string{"A.txt"}, []string{"a.txt"}},
		{"case insensitive", []string{"A.txt"}, []string{"a.txt"}, true, []Pair{{"A.txt", "a.txt"}}, nil, nil},
		{"exact preferred", []string{"A.txt", "a.txt"}, []string{"a.txt", "A.TXT"}, true,
			[]Pair{{"A.txt", "A.TXT"}, {"a.txt", "a.txt"}}, nil, nil},
		{"leftovers", []string{"x", "y"}, []string{"y", "z"}, true, []Pair{{"y", "y"}}, []string{"x"}, []string{"z"}},
		{"relative paths", []string{"sub/a.go"}, []string{"sub/a.go", "a.go"}, false, []Pair{{"sub/a.go", "sub/a.go"}}, nil, []string{"a.go"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, l, r := PairByName(tc.left, tc.right, tc.ci)
			if !reflect.DeepEqual(p, tc.pairs) || !reflect.DeepEqual(l, tc.onlyL) || !reflect.DeepEqual(r, tc.onlyR) {
				t.Fatalf("got %v %v %v", p, l, r)
			}
		})
	}
}
